// Package ingest reads OFMX input and builds canonical domain models.
//
// Author: Miroslav Pašek
package ingest

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

type OFMXReader interface {
	Read(ctx context.Context, path string) (domain.OFMXDocument, error)
}

// FileReader reads OFMX XML snapshots from filesystem paths.
type FileReader struct {
	FrontierSnapToleranceMeters float64
	CoordinateEpsilon           float64
	ArcMaxChordLengthMeters     float64
	Warningf                    func(format string, args ...any)
}

type frontierExpansionOptions struct {
	SnapToleranceMeters float64
	CoordinateEpsilon   float64
	ArcMaxChordLengthM  float64
	Warningf            func(format string, args ...any)
}

const (
	defaultFrontierSnapToleranceMeters = 1000.0
	defaultCoordinateEpsilon           = 1e-7
	defaultArcMaxChordLengthMeters     = 750.0
)

func (r FileReader) frontierOptions() frontierExpansionOptions {
	opts := frontierExpansionOptions{
		SnapToleranceMeters: r.FrontierSnapToleranceMeters,
		CoordinateEpsilon:   r.CoordinateEpsilon,
		ArcMaxChordLengthM:  r.ArcMaxChordLengthMeters,
		Warningf:            r.Warningf,
	}
	if opts.SnapToleranceMeters <= 0 {
		opts.SnapToleranceMeters = defaultFrontierSnapToleranceMeters
	}
	if opts.CoordinateEpsilon <= 0 {
		opts.CoordinateEpsilon = defaultCoordinateEpsilon
	}
	if opts.ArcMaxChordLengthM <= 0 {
		opts.ArcMaxChordLengthM = defaultArcMaxChordLengthMeters
	}
	if opts.Warningf == nil {
		opts.Warningf = log.Printf
	}
	return opts
}

// Read loads and parses an OFMX snapshot file.
func (r FileReader) Read(_ context.Context, path string) (domain.OFMXDocument, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return domain.OFMXDocument{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to read input file %q", path), err)
	}

	doc, err := parseSnapshotWithOptions(b, r.frontierOptions())
	if err != nil {
		return domain.OFMXDocument{}, err
	}

	doc.SourcePath = path
	doc.RawXML = b

	return doc, nil
}

func parseSnapshot(raw []byte) (domain.OFMXDocument, error) {
	return parseSnapshotWithOptions(raw, FileReader{}.frontierOptions())
}

func parseSnapshotWithOptions(raw []byte, opts frontierExpansionOptions) (domain.OFMXDocument, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))

	for {
		tok, err := dec.Token()
		if err != nil {
			return domain.OFMXDocument{}, domain.NewError(domain.ErrIngest, "failed to parse OFMX XML", err)
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if start.Name.Local != "OFMX-Snapshot" {
			return domain.OFMXDocument{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("unexpected root element %q", start.Name.Local), nil)
		}

		meta := extractSnapshotMeta(start)
		if err := validateSnapshotMeta(meta); err != nil {
			return domain.OFMXDocument{}, err
		}

		doc, err := parseSnapshotContent(dec, opts)
		if err != nil {
			return domain.OFMXDocument{}, err
		}
		doc.SnapshotMeta = meta

		return doc, nil
	}
}

func extractSnapshotMeta(start xml.StartElement) domain.OFMXSnapshotMetadata {
	meta := domain.OFMXSnapshotMetadata{}
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "version":
			meta.Version = attr.Value
		case "origin":
			meta.Origin = attr.Value
		case "namespace":
			meta.Namespace = attr.Value
		case "regions":
			meta.Regions = attr.Value
		case "created":
			meta.Created = attr.Value
		case "effective":
			meta.Effective = attr.Value
		case "expiration":
			meta.Expiration = attr.Value
		}
	}

	return meta
}

func validateSnapshotMeta(meta domain.OFMXSnapshotMetadata) error {
	if meta.Version == "" {
		return domain.NewError(domain.ErrIngest, "missing required OFMX-Snapshot attribute: version", nil)
	}
	if meta.Origin == "" {
		return domain.NewError(domain.ErrIngest, "missing required OFMX-Snapshot attribute: origin", nil)
	}
	if meta.Namespace == "" {
		return domain.NewError(domain.ErrIngest, "missing required OFMX-Snapshot attribute: namespace", nil)
	}
	if meta.Created == "" {
		return domain.NewError(domain.ErrIngest, "missing required OFMX-Snapshot attribute: created", nil)
	}
	if meta.Effective == "" {
		return domain.NewError(domain.ErrIngest, "missing required OFMX-Snapshot attribute: effective", nil)
	}

	return nil
}

type featureParseState struct {
	abds []abdXML
	gbrs []gbrXML
}

func parseSnapshotContent(dec *xml.Decoder, opts frontierExpansionOptions) (domain.OFMXDocument, error) {
	doc := domain.OFMXDocument{
		FeatureCounts: make(map[string]int),
	}

	counts := make(map[string]int)
	state := featureParseState{}
	depth := 1

	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return domain.OFMXDocument{}, domain.NewError(domain.ErrIngest, "failed to read OFMX-Snapshot content", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 1 {
				counts[t.Name.Local]++
				if err := parseTopLevelFeature(dec, t, &doc, &state); err != nil {
					return domain.OFMXDocument{}, err
				}
				continue
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}

	index, err := buildGeographicalBorderIndex(state.gbrs, opts)
	if err != nil {
		return domain.OFMXDocument{}, err
	}

	airspaceNames := make(map[string]string, len(doc.Airspaces))
	for _, as := range doc.Airspaces {
		if strings.TrimSpace(as.ID) == "" {
			continue
		}
		airspaceNames[as.ID] = strings.TrimSpace(as.Name)
	}

	doc.AirspaceBorders = make([]domain.OFMXAirspaceBorder, 0, len(state.abds))
	for _, abd := range state.abds {
		border, mapErr := mapAirspaceBorder(abd, index, airspaceNames, opts)
		if mapErr != nil {
			return domain.OFMXDocument{}, mapErr
		}
		doc.AirspaceBorders = append(doc.AirspaceBorders, border)
	}

	doc.FeatureCounts = counts
	return doc, nil
}

func parseTopLevelFeature(dec *xml.Decoder, start xml.StartElement, doc *domain.OFMXDocument, state *featureParseState) error {
	switch start.Name.Local {
	case "Ahp":
		var in ahpXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Ahp", err)
		}
		airport, err := mapAirport(in)
		if err != nil {
			return err
		}
		doc.Airports = append(doc.Airports, airport)
	case "Rwy":
		var in rwyXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Rwy", err)
		}
		doc.Runways = append(doc.Runways, mapRunway(in))
	case "Rdn":
		var in rdnXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Rdn", err)
		}
		doc.RunwayDirections = append(doc.RunwayDirections, mapRunwayDirection(in))
	case "Vor":
		var in vorXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Vor", err)
		}
		nv, err := mapVOR(in)
		if err != nil {
			return err
		}
		doc.VORs = append(doc.VORs, nv)
	case "Ndb":
		var in ndbXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Ndb", err)
		}
		nv, err := mapNDB(in)
		if err != nil {
			return err
		}
		doc.NDBs = append(doc.NDBs, nv)
	case "Dme":
		var in dmeXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Dme", err)
		}
		nv, err := mapDME(in)
		if err != nil {
			return err
		}
		doc.DMEs = append(doc.DMEs, nv)
	case "Tcn":
		var in tcnXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Tcn", err)
		}
		nv, err := mapTACAN(in)
		if err != nil {
			return err
		}
		doc.TACANs = append(doc.TACANs, nv)
	case "Mkr":
		var in mkrXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Mkr", err)
		}
		nv, err := mapMarker(in)
		if err != nil {
			return err
		}
		doc.Markers = append(doc.Markers, nv)
	case "Dpn":
		var in dpnXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Dpn", err)
		}
		nv, err := mapDesignatedPoint(in)
		if err != nil {
			return err
		}
		doc.DesignatedPoints = append(doc.DesignatedPoints, nv)
	case "Ase":
		var in aseXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Ase", err)
		}
		doc.Airspaces = append(doc.Airspaces, mapAirspace(in))
	case "Abd":
		var in abdXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Abd", err)
		}
		state.abds = append(state.abds, in)
	case "Gbr":
		var in gbrXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Gbr", err)
		}
		state.gbrs = append(state.gbrs, in)
	case "Obs":
		var in obsXML
		if err := dec.DecodeElement(&in, &start); err != nil {
			return domain.NewError(domain.ErrIngest, "failed to decode Obs", err)
		}
		obs, err := mapObstacle(in)
		if err != nil {
			return err
		}
		doc.Obstacles = append(doc.Obstacles, obs)
	default:
		if err := dec.Skip(); err != nil {
			return domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to skip feature %q", start.Name.Local), err)
		}
	}

	return nil
}

type ahpXML struct {
	AhpUid struct {
		CodeID string `xml:"codeId"`
	} `xml:"AhpUid"`
	TxtName  string `xml:"txtName"`
	CodeType string `xml:"codeType"`
	GeoLat   string `xml:"geoLat"`
	GeoLong  string `xml:"geoLong"`
	ValElev  string `xml:"valElev"`
}

type rwyXML struct {
	RwyUid struct {
		AhpUid struct {
			CodeID string `xml:"codeId"`
		} `xml:"AhpUid"`
		TxtDesig string `xml:"txtDesig"`
	} `xml:"RwyUid"`
	ValLen          string `xml:"valLen"`
	ValWid          string `xml:"valWid"`
	CodeComposition string `xml:"codeComposition"`
	CodePreparation string `xml:"codePreparation"`
}

type rdnXML struct {
	RdnUid struct {
		RwyUid struct {
			AhpUid struct {
				CodeID string `xml:"codeId"`
			} `xml:"AhpUid"`
			TxtDesig string `xml:"txtDesig"`
		} `xml:"RwyUid"`
		TxtDesig string `xml:"txtDesig"`
	} `xml:"RdnUid"`
	ValTrueBrg string `xml:"valTrueBrg"`
	ValMagBrg  string `xml:"valMagBrg"`
}

type vorXML struct {
	VorUid struct {
		CodeID  string `xml:"codeId"`
		GeoLat  string `xml:"geoLat"`
		GeoLong string `xml:"geoLong"`
	} `xml:"VorUid"`
	TxtName  string `xml:"txtName"`
	CodeType string `xml:"codeType"`
}

type ndbXML struct {
	NdbUid struct {
		CodeID  string `xml:"codeId"`
		GeoLat  string `xml:"geoLat"`
		GeoLong string `xml:"geoLong"`
	} `xml:"NdbUid"`
	TxtName   string `xml:"txtName"`
	CodeClass string `xml:"codeClass"`
}

type dmeXML struct {
	DmeUid struct {
		CodeID  string `xml:"codeId"`
		GeoLat  string `xml:"geoLat"`
		GeoLong string `xml:"geoLong"`
	} `xml:"DmeUid"`
	TxtName  string `xml:"txtName"`
	CodeType string `xml:"codeType"`
}

type tcnXML struct {
	TcnUid struct {
		CodeID  string `xml:"codeId"`
		GeoLat  string `xml:"geoLat"`
		GeoLong string `xml:"geoLong"`
	} `xml:"TcnUid"`
	TxtName string `xml:"txtName"`
}

type mkrXML struct {
	MkrUid struct {
		CodeID  string `xml:"codeId"`
		GeoLat  string `xml:"geoLat"`
		GeoLong string `xml:"geoLong"`
	} `xml:"MkrUid"`
	TxtName   string `xml:"txtName"`
	CodeClass string `xml:"codeClass"`
}

type dpnXML struct {
	DpnUid struct {
		CodeID  string `xml:"codeId"`
		GeoLat  string `xml:"geoLat"`
		GeoLong string `xml:"geoLong"`
	} `xml:"DpnUid"`
	TxtName  string `xml:"txtName"`
	CodeType string `xml:"codeType"`
}

type aseXML struct {
	AseUid struct {
		CodeType string `xml:"codeType"`
		CodeID   string `xml:"codeId"`
	} `xml:"AseUid"`
	TxtName          string `xml:"txtName"`
	CodeClass        string `xml:"codeClass"`
	CodeActivity     string `xml:"codeActivity"`
	CodeDistVerLower string `xml:"codeDistVerLower"`
	ValDistVerLower  string `xml:"valDistVerLower"`
	CodeDistVerUpper string `xml:"codeDistVerUpper"`
	ValDistVerUpper  string `xml:"valDistVerUpper"`
	TxtRmk           string `xml:"txtRmk"`
}

type abdXML struct {
	AbdUid struct {
		AseUid struct {
			CodeType string `xml:"codeType"`
			CodeID   string `xml:"codeId"`
		} `xml:"AseUid"`
	} `xml:"AbdUid"`
	Vertices []abdVertexXML `xml:"Avx"`
	Circle   *abdCircleXML  `xml:"Circle"`
}

type abdVertexXML struct {
	GbrUID struct {
		MID     string `xml:"mid,attr"`
		TxtName string `xml:"txtName"`
	} `xml:"GbrUid"`
	CodeType     string `xml:"codeType"`
	GeoLat       string `xml:"geoLat"`
	GeoLong      string `xml:"geoLong"`
	CodeDatum    string `xml:"codeDatum"`
	GeoLatArc    string `xml:"geoLatArc"`
	GeoLongArc   string `xml:"geoLongArc"`
	ValRadiusArc string `xml:"valRadiusArc"`
	UOMRadiusArc string `xml:"uomRadiusArc"`
}

type abdCircleXML struct {
	GeoLatCen  string `xml:"geoLatCen"`
	GeoLongCen string `xml:"geoLongCen"`
	CodeDatum  string `xml:"codeDatum"`
	ValRadius  string `xml:"valRadius"`
	UOMRadius  string `xml:"uomRadius"`
}

type gbrXML struct {
	GbrUID struct {
		MID     string `xml:"mid,attr"`
		TxtName string `xml:"txtName"`
	} `xml:"GbrUid"`
	Vertices []gbrVertexXML `xml:"Gbv"`
}

type gbrVertexXML struct {
	CodeType  string `xml:"codeType"`
	GeoLat    string `xml:"geoLat"`
	GeoLong   string `xml:"geoLong"`
	CodeDatum string `xml:"codeDatum"`
}

type obsXML struct {
	ObsUid struct {
		Mid    string `xml:"mid,attr"`
		OgrUid struct {
			TxtName string `xml:"txtName"`
		} `xml:"OgrUid"`
		GeoLat  string `xml:"geoLat"`
		GeoLong string `xml:"geoLong"`
	} `xml:"ObsUid"`
	TxtName  string `xml:"txtName"`
	CodeType string `xml:"codeType"`
	ValElev  string `xml:"valElev"`
	ValHgt   string `xml:"valHgt"`
}

func mapAirport(in ahpXML) (domain.OFMXAirport, error) {
	lat, err := parseCoordinate(in.GeoLat, true)
	if err != nil {
		return domain.OFMXAirport{}, domain.NewError(domain.ErrIngest, "failed to parse Ahp latitude", err)
	}

	lon, err := parseCoordinate(in.GeoLong, false)
	if err != nil {
		return domain.OFMXAirport{}, domain.NewError(domain.ErrIngest, "failed to parse Ahp longitude", err)
	}

	return domain.OFMXAirport{
		ID:    strings.TrimSpace(in.AhpUid.CodeID),
		Name:  strings.TrimSpace(in.TxtName),
		Type:  strings.TrimSpace(in.CodeType),
		Lat:   lat,
		Lon:   lon,
		ElevM: parseFloatOrDefault(in.ValElev, 0),
	}, nil
}

func mapRunway(in rwyXML) domain.OFMXRunway {
	return domain.OFMXRunway{
		AirportID:   strings.TrimSpace(in.RwyUid.AhpUid.CodeID),
		Designation: strings.TrimSpace(in.RwyUid.TxtDesig),
		LengthM:     parseFloatOrDefault(in.ValLen, 0),
		WidthM:      parseFloatOrDefault(in.ValWid, 0),
		Composition: strings.TrimSpace(in.CodeComposition),
		Preparation: strings.TrimSpace(in.CodePreparation),
	}
}

func mapRunwayDirection(in rdnXML) domain.OFMXRunwayDirection {
	return domain.OFMXRunwayDirection{
		AirportID:    strings.TrimSpace(in.RdnUid.RwyUid.AhpUid.CodeID),
		RunwayDesign: strings.TrimSpace(in.RdnUid.RwyUid.TxtDesig),
		Designator:   strings.TrimSpace(in.RdnUid.TxtDesig),
		TrueBearing:  parseFloatOrDefault(in.ValTrueBrg, 0),
		MagBearing:   parseFloatOrDefault(in.ValMagBrg, 0),
	}
}

func mapVOR(in vorXML) (domain.OFMXVOR, error) {
	lat, err := parseCoordinate(in.VorUid.GeoLat, true)
	if err != nil {
		return domain.OFMXVOR{}, domain.NewError(domain.ErrIngest, "failed to parse Vor latitude", err)
	}
	lon, err := parseCoordinate(in.VorUid.GeoLong, false)
	if err != nil {
		return domain.OFMXVOR{}, domain.NewError(domain.ErrIngest, "failed to parse Vor longitude", err)
	}

	return domain.OFMXVOR{
		ID:   strings.TrimSpace(in.VorUid.CodeID),
		Name: strings.TrimSpace(in.TxtName),
		Type: strings.TrimSpace(in.CodeType),
		Lat:  lat,
		Lon:  lon,
	}, nil
}

func mapNDB(in ndbXML) (domain.OFMXNDB, error) {
	lat, err := parseCoordinate(in.NdbUid.GeoLat, true)
	if err != nil {
		return domain.OFMXNDB{}, domain.NewError(domain.ErrIngest, "failed to parse Ndb latitude", err)
	}
	lon, err := parseCoordinate(in.NdbUid.GeoLong, false)
	if err != nil {
		return domain.OFMXNDB{}, domain.NewError(domain.ErrIngest, "failed to parse Ndb longitude", err)
	}

	return domain.OFMXNDB{
		ID:    strings.TrimSpace(in.NdbUid.CodeID),
		Name:  strings.TrimSpace(in.TxtName),
		Class: strings.TrimSpace(in.CodeClass),
		Lat:   lat,
		Lon:   lon,
	}, nil
}

func mapDME(in dmeXML) (domain.OFMXDME, error) {
	lat, err := parseCoordinate(in.DmeUid.GeoLat, true)
	if err != nil {
		return domain.OFMXDME{}, domain.NewError(domain.ErrIngest, "failed to parse Dme latitude", err)
	}
	lon, err := parseCoordinate(in.DmeUid.GeoLong, false)
	if err != nil {
		return domain.OFMXDME{}, domain.NewError(domain.ErrIngest, "failed to parse Dme longitude", err)
	}

	return domain.OFMXDME{
		ID:   strings.TrimSpace(in.DmeUid.CodeID),
		Name: strings.TrimSpace(in.TxtName),
		Type: strings.TrimSpace(in.CodeType),
		Lat:  lat,
		Lon:  lon,
	}, nil
}

func mapTACAN(in tcnXML) (domain.OFMXTACAN, error) {
	lat, err := parseCoordinate(in.TcnUid.GeoLat, true)
	if err != nil {
		return domain.OFMXTACAN{}, domain.NewError(domain.ErrIngest, "failed to parse Tcn latitude", err)
	}
	lon, err := parseCoordinate(in.TcnUid.GeoLong, false)
	if err != nil {
		return domain.OFMXTACAN{}, domain.NewError(domain.ErrIngest, "failed to parse Tcn longitude", err)
	}

	return domain.OFMXTACAN{
		ID:   strings.TrimSpace(in.TcnUid.CodeID),
		Name: strings.TrimSpace(in.TxtName),
		Lat:  lat,
		Lon:  lon,
	}, nil
}

func mapMarker(in mkrXML) (domain.OFMXMarker, error) {
	lat, err := parseCoordinate(in.MkrUid.GeoLat, true)
	if err != nil {
		return domain.OFMXMarker{}, domain.NewError(domain.ErrIngest, "failed to parse Mkr latitude", err)
	}
	lon, err := parseCoordinate(in.MkrUid.GeoLong, false)
	if err != nil {
		return domain.OFMXMarker{}, domain.NewError(domain.ErrIngest, "failed to parse Mkr longitude", err)
	}

	return domain.OFMXMarker{
		ID:    strings.TrimSpace(in.MkrUid.CodeID),
		Name:  strings.TrimSpace(in.TxtName),
		Class: strings.TrimSpace(in.CodeClass),
		Lat:   lat,
		Lon:   lon,
	}, nil
}

func mapDesignatedPoint(in dpnXML) (domain.OFMXDesignatedPoint, error) {
	lat, err := parseCoordinate(in.DpnUid.GeoLat, true)
	if err != nil {
		return domain.OFMXDesignatedPoint{}, domain.NewError(domain.ErrIngest, "failed to parse Dpn latitude", err)
	}
	lon, err := parseCoordinate(in.DpnUid.GeoLong, false)
	if err != nil {
		return domain.OFMXDesignatedPoint{}, domain.NewError(domain.ErrIngest, "failed to parse Dpn longitude", err)
	}

	return domain.OFMXDesignatedPoint{
		ID:   strings.TrimSpace(in.DpnUid.CodeID),
		Name: strings.TrimSpace(in.TxtName),
		Type: strings.TrimSpace(in.CodeType),
		Lat:  lat,
		Lon:  lon,
	}, nil
}

func mapAirspace(in aseXML) domain.OFMXAirspace {
	return domain.OFMXAirspace{
		ID:          strings.TrimSpace(in.AseUid.CodeID),
		Type:        strings.TrimSpace(in.AseUid.CodeType),
		Name:        strings.TrimSpace(in.TxtName),
		Class:       strings.TrimSpace(in.CodeClass),
		Activity:    strings.TrimSpace(in.CodeActivity),
		LowerValueM: parseFloatOrDefault(in.ValDistVerLower, 0),
		LowerRef:    strings.TrimSpace(in.CodeDistVerLower),
		UpperValueM: parseFloatOrDefault(in.ValDistVerUpper, 0),
		UpperRef:    strings.TrimSpace(in.CodeDistVerUpper),
		Remark:      strings.TrimSpace(in.TxtRmk),
	}
}

func mapAirspaceBorder(in abdXML, borderIndex geographicalBorderIndex, airspaceNames map[string]string, opts frontierExpansionOptions) (domain.OFMXAirspaceBorder, error) {
	out := domain.OFMXAirspaceBorder{
		AirspaceID: strings.TrimSpace(in.AbdUid.AseUid.CodeID),
		Points:     make([]domain.OFMXGeoPoint, 0, len(in.Vertices)),
	}
	if in.Circle != nil {
		circlePoints, err := expandCircularBorder(*in.Circle, opts)
		if err != nil {
			return domain.OFMXAirspaceBorder{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to map circular Abd border for airspace %q", out.AirspaceID), err)
		}
		out.Points = append(out.Points, circlePoints...)
		return out, nil
	}
	if len(in.Vertices) == 0 {
		return out, nil
	}

	parsedVertices := make([]airspaceVertex, 0, len(in.Vertices))
	for _, v := range in.Vertices {
		lat, err := parseCoordinate(v.GeoLat, true)
		if err != nil {
			return domain.OFMXAirspaceBorder{}, domain.NewError(domain.ErrIngest, "failed to parse Abd vertex latitude", err)
		}
		lon, err := parseCoordinate(v.GeoLong, false)
		if err != nil {
			return domain.OFMXAirspaceBorder{}, domain.NewError(domain.ErrIngest, "failed to parse Abd vertex longitude", err)
		}
		parsedVertices = append(parsedVertices, airspaceVertex{
			Point:      domain.OFMXGeoPoint{Lat: lat, Lon: lon},
			CodeType:   strings.ToUpper(strings.TrimSpace(v.CodeType)),
			CodeDatum:  strings.ToUpper(strings.TrimSpace(v.CodeDatum)),
			GbrMID:     strings.TrimSpace(v.GbrUID.MID),
			GbrTxtName: strings.TrimSpace(v.GbrUID.TxtName),
		})

		if strings.TrimSpace(v.GeoLatArc) != "" && strings.TrimSpace(v.GeoLongArc) != "" {
			arcLat, err := parseCoordinate(v.GeoLatArc, true)
			if err != nil {
				return domain.OFMXAirspaceBorder{}, domain.NewError(domain.ErrIngest, "failed to parse Abd arc center latitude", err)
			}
			arcLon, err := parseCoordinate(v.GeoLongArc, false)
			if err != nil {
				return domain.OFMXAirspaceBorder{}, domain.NewError(domain.ErrIngest, "failed to parse Abd arc center longitude", err)
			}

			parsedVertices[len(parsedVertices)-1].ArcCenter = &domain.OFMXGeoPoint{Lat: arcLat, Lon: arcLon}
		}

		if radiusM, ok := parseHorizontalDistanceMeters(v.ValRadiusArc, v.UOMRadiusArc); ok {
			parsedVertices[len(parsedVertices)-1].ArcRadiusM = radiusM
		}
	}

	for i, v := range parsedVertices {
		if v.CodeType != "FNT" {
			if (v.CodeType == "CWA" || v.CodeType == "CCA") && v.ArcCenter != nil {
				next := parsedVertices[(i+1)%len(parsedVertices)].Point
				expandedArc := expandArcSegment(v.Point, next, *v.ArcCenter, v.CodeType, v.ArcRadiusM, opts)
				if len(expandedArc) > 0 {
					for _, p := range expandedArc {
						out.Points = appendPointUnique(out.Points, p, opts.CoordinateEpsilon)
					}
					continue
				}
			}

			out.Points = appendPointUnique(out.Points, v.Point, opts.CoordinateEpsilon)
			continue
		}

		gbrPoints, ok := borderIndex.find(v.GbrMID, v.GbrTxtName)
		if !ok {
			opts.Warningf("OFMX frontier warning airspace_id=%q airspace_name=%q missing_border_uid=%q missing_border_name=%q", out.AirspaceID, strings.TrimSpace(airspaceNames[out.AirspaceID]), v.GbrMID, v.GbrTxtName)
			out.Points = appendPointUnique(out.Points, v.Point, opts.CoordinateEpsilon)
			continue
		}

		prev := parsedVertices[(i-1+len(parsedVertices))%len(parsedVertices)].Point
		next := parsedVertices[(i+1)%len(parsedVertices)].Point
		expanded := expandFrontierSegment(gbrPoints, prev, next, opts)
		if len(expanded) == 0 {
			out.Points = appendPointUnique(out.Points, v.Point, opts.CoordinateEpsilon)
			continue
		}
		for _, p := range expanded {
			out.Points = appendPointUnique(out.Points, p, opts.CoordinateEpsilon)
		}
	}

	return out, nil
}

func mapObstacle(in obsXML) (domain.OFMXObstacle, error) {
	lat, err := parseCoordinate(in.ObsUid.GeoLat, true)
	if err != nil {
		return domain.OFMXObstacle{}, domain.NewError(domain.ErrIngest, "failed to parse Obs latitude", err)
	}
	lon, err := parseCoordinate(in.ObsUid.GeoLong, false)
	if err != nil {
		return domain.OFMXObstacle{}, domain.NewError(domain.ErrIngest, "failed to parse Obs longitude", err)
	}

	id := strings.TrimSpace(in.ObsUid.Mid)
	if id == "" {
		id = strings.TrimSpace(in.ObsUid.OgrUid.TxtName)
	}
	if id == "" {
		id = strings.TrimSpace(in.TxtName)
	}

	return domain.OFMXObstacle{
		ID:         id,
		Type:       strings.TrimSpace(in.CodeType),
		Name:       strings.TrimSpace(in.TxtName),
		Lat:        lat,
		Lon:        lon,
		HeightM:    parseFloatOrDefault(in.ValHgt, 0),
		ElevationM: parseFloatOrDefault(in.ValElev, 0),
	}, nil
}

func parseFloatOrDefault(s string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return fallback
	}

	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}

	return v
}

func parseCoordinate(raw string, isLat bool) (float64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("empty coordinate")
	}

	hemi := value[len(value)-1]
	body := strings.TrimSpace(value[:len(value)-1])
	if body == "" {
		return 0, fmt.Errorf("missing coordinate body")
	}

	maxAbs := 180.0
	if isLat {
		maxAbs = 90.0
	}

	if decimalDeg, err := strconv.ParseFloat(body, 64); err == nil {
		signed, err := applyHemisphere(decimalDeg, hemi, isLat)
		if err == nil && math.Abs(signed) <= maxAbs {
			return signed, nil
		}
	}

	dmsDeg, err := parseDMSBody(body, isLat)
	if err != nil {
		return 0, err
	}

	signed, err := applyHemisphere(dmsDeg, hemi, isLat)
	if err != nil {
		return 0, err
	}

	if math.Abs(signed) > maxAbs {
		return 0, fmt.Errorf("coordinate %q out of range", raw)
	}

	return signed, nil
}

func parseDMSBody(body string, isLat bool) (float64, error) {
	degDigits := 3
	if isLat {
		degDigits = 2
	}

	if len(body) < degDigits {
		return 0, fmt.Errorf("invalid coordinate body %q", body)
	}

	degPart := body[:degDigits]
	rest := body[degDigits:]

	deg, err := strconv.ParseFloat(degPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid degrees %q", degPart)
	}

	min := 0.0
	sec := 0.0

	switch {
	case len(rest) == 0:
		min, sec = 0, 0
	case len(rest) <= 2:
		min, err = strconv.ParseFloat(rest, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid minutes %q", rest)
		}
	default:
		min, err = strconv.ParseFloat(rest[:2], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid minutes %q", rest[:2])
		}
		sec, err = strconv.ParseFloat(rest[2:], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid seconds %q", rest[2:])
		}
	}

	return deg + (min / 60.0) + (sec / 3600.0), nil
}

func applyHemisphere(value float64, hemi byte, isLat bool) (float64, error) {
	switch hemi {
	case 'N':
		if !isLat {
			return 0, fmt.Errorf("invalid longitude hemisphere %q", string(hemi))
		}
		return value, nil
	case 'S':
		if !isLat {
			return 0, fmt.Errorf("invalid longitude hemisphere %q", string(hemi))
		}
		return -value, nil
	case 'E':
		if isLat {
			return 0, fmt.Errorf("invalid latitude hemisphere %q", string(hemi))
		}
		return value, nil
	case 'W':
		if isLat {
			return 0, fmt.Errorf("invalid latitude hemisphere %q", string(hemi))
		}
		return -value, nil
	default:
		return 0, fmt.Errorf("invalid hemisphere %q", string(hemi))
	}
}
