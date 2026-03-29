// Package domain defines canonical OFMX input and output XML models.
//
// Author: Miroslav Pašek
package domain

import "encoding/xml"

type OFMXDocument struct {
	SourcePath    string
	SnapshotMeta  OFMXSnapshotMetadata
	FeatureCounts map[string]int

	Airports         []OFMXAirport
	Runways          []OFMXRunway
	RunwayDirections []OFMXRunwayDirection
	VORs             []OFMXVOR
	NDBs             []OFMXNDB
	DMEs             []OFMXDME
	TACANs           []OFMXTACAN
	Markers          []OFMXMarker
	DesignatedPoints []OFMXDesignatedPoint
	Airspaces        []OFMXAirspace
	AirspaceBorders  []OFMXAirspaceBorder
	CountryBorders   []OFMXGeographicalBorder
	Obstacles        []OFMXObstacle
}

// OFMXSnapshotMetadata carries root OFMX snapshot attributes.
type OFMXSnapshotMetadata struct {
	Version    string
	Origin     string
	Namespace  string
	Regions    string
	Created    string
	Effective  string
	Expiration string
}

// ParseReport contains ingest statistics used for diagnostics.
type ParseReport struct {
	SnapshotMeta  OFMXSnapshotMetadata `json:"snapshot_meta"`
	TotalFeatures int                  `json:"total_features"`
	FeatureCounts map[string]int       `json:"feature_counts"`
}

// OutputDocument is the root custom XML output model.
type OutputDocument struct {
	XMLName     xml.Name         `xml:"NavSnapshot"`
	Cycle       string           `xml:"cycle,attr"`
	Region      string           `xml:"region,attr"`
	GeneratedAt string           `xml:"generatedAt,attr"`
	Schema      string           `xml:"schema,attr"`
	Source      string           `xml:"source,attr,omitempty"`
	Airports    *OutputAirports  `xml:"Airports,omitempty"`
	Navaids     *OutputNavaids   `xml:"Navaids,omitempty"`
	Airspaces   *OutputAirspaces `xml:"Airspaces,omitempty"`
	Obstacles   *OutputObstacles `xml:"Obstacles,omitempty"`
}

// OutputAirports wraps airport output entries.
type OutputAirports struct {
	Airports []OutputAirport `xml:"Airport"`
}

// OutputAirport represents a mapped airport.
type OutputAirport struct {
	ID      string         `xml:"id,attr"`
	D       string         `xml:"d,attr"`
	N       string         `xml:"n,attr"`
	Lat     float64        `xml:"lat,attr"`
	Lon     float64        `xml:"lon,attr"`
	ElevM   float64        `xml:"elevM,attr"`
	Runways *OutputRunways `xml:"Runways,omitempty"`
}

// OutputRunways wraps runway output entries.
type OutputRunways struct {
	Runways []OutputRunway `xml:"Runway"`
}

// OutputRunway represents a mapped runway.
type OutputRunway struct {
	N    string                  `xml:"n,attr"`
	LenM float64                 `xml:"lenM,attr"`
	WidM float64                 `xml:"widM,attr"`
	Comp string                  `xml:"comp,attr,omitempty"`
	Prep string                  `xml:"prep,attr,omitempty"`
	Dirs []OutputRunwayDirection `xml:"Dir"`
}

// OutputRunwayDirection represents a mapped runway direction.
type OutputRunwayDirection struct {
	Bearing float64 `xml:"brg,attr"`
	Code    string  `xml:"code,attr"`
}

// OutputNavaids wraps navaid output entries.
type OutputNavaids struct {
	Navaids []OutputNavaid `xml:"Navaid"`
}

// OutputNavaid represents a mapped navaid.
type OutputNavaid struct {
	ID  string  `xml:"id,attr"`
	T   string  `xml:"t,attr"`
	D   string  `xml:"d,attr"`
	N   string  `xml:"n,attr"`
	Lat float64 `xml:"lat,attr"`
	Lon float64 `xml:"lon,attr"`
}

// OutputAirspaces wraps airspace output entries.
type OutputAirspaces struct {
	Airspaces []OutputAirspace `xml:"Airspace"`
}

// OutputAirspace represents a mapped airspace with geometry.
type OutputAirspace struct {
	ID     string        `xml:"id,attr"`
	D      string        `xml:"d,attr"`
	N      string        `xml:"n,attr"`
	T      string        `xml:"t,attr"`
	LowM   float64       `xml:"lowM,attr"`
	LowRef string        `xml:"lowRef,attr"`
	UpM    float64       `xml:"upM,attr"`
	UpRef  string        `xml:"upRef,attr"`
	Rmk    string        `xml:"rmk,attr,omitempty"`
	Poly   OutputPolygon `xml:"Poly"`
	BBox   OutputBBox    `xml:"BBox"`
}

// OutputPolygon represents an airspace polygon.
type OutputPolygon struct {
	Points []OutputPoint `xml:"P"`
}

// OutputPoint represents one geometry point.
type OutputPoint struct {
	Lat float64 `xml:"lat,attr"`
	Lon float64 `xml:"lon,attr"`
}

// OutputBBox is a bounding box for geometry.
type OutputBBox struct {
	MinLat float64 `xml:"minLat,attr"`
	MinLon float64 `xml:"minLon,attr"`
	MaxLat float64 `xml:"maxLat,attr"`
	MaxLon float64 `xml:"maxLon,attr"`
}

// OutputObstacles wraps obstacle output entries.
type OutputObstacles struct {
	Obstacles []OutputObstacle `xml:"Obstacle"`
}

// OutputObstacle represents a mapped obstacle.
type OutputObstacle struct {
	ID    string  `xml:"id,attr"`
	T     string  `xml:"t,attr"`
	N     string  `xml:"n,attr"`
	Lat   float64 `xml:"lat,attr"`
	Lon   float64 `xml:"lon,attr"`
	HM    float64 `xml:"hM,attr"`
	ElevM float64 `xml:"elevM,attr"`
}

// OFMXAirport is the canonical ingest model of an airport.
type OFMXAirport struct {
	ID    string
	Name  string
	Type  string
	Lat   float64
	Lon   float64
	ElevM float64
}

// OFMXRunway is the canonical ingest model of a runway.
type OFMXRunway struct {
	AirportID   string
	Designation string
	LengthM     float64
	WidthM      float64
	Composition string
	Preparation string
}

// OFMXRunwayDirection is the canonical ingest model of runway direction.
type OFMXRunwayDirection struct {
	AirportID    string
	RunwayDesign string
	Designator   string
	TrueBearing  float64
	MagBearing   float64
}

// OFMXVOR is the canonical ingest model of a VOR.
type OFMXVOR struct {
	ID   string
	Name string
	Type string
	Lat  float64
	Lon  float64
}

// OFMXNDB is the canonical ingest model of an NDB.
type OFMXNDB struct {
	ID    string
	Name  string
	Class string
	Lat   float64
	Lon   float64
}

// OFMXDME is the canonical ingest model of a DME.
type OFMXDME struct {
	ID   string
	Name string
	Type string
	Lat  float64
	Lon  float64
}

// OFMXTACAN is the canonical ingest model of a TACAN.
type OFMXTACAN struct {
	ID   string
	Name string
	Lat  float64
	Lon  float64
}

// OFMXMarker is the canonical ingest model of a marker beacon.
type OFMXMarker struct {
	ID    string
	Name  string
	Class string
	Lat   float64
	Lon   float64
}

// OFMXDesignatedPoint is the canonical ingest model of a designated point.
type OFMXDesignatedPoint struct {
	ID   string
	Name string
	Type string
	Lat  float64
	Lon  float64
}

// OFMXAirspace is the canonical ingest model of an airspace.
type OFMXAirspace struct {
	ID          string
	Type        string
	Name        string
	Class       string
	Activity    string
	LowerValueM float64
	LowerRef    string
	UpperValueM float64
	UpperRef    string
	Remark      string
}

// OFMXAirspaceBorder represents one airspace border definition.
type OFMXAirspaceBorder struct {
	AirspaceID string
	Points     []OFMXGeoPoint
}

// OFMXGeographicalBorder represents one parsed geographical border (Gbr).
type OFMXGeographicalBorder struct {
	UID    string
	Name   string
	Points []OFMXGeoPoint
}

// OFMXGeoPoint is a geographic point in decimal degrees.
type OFMXGeoPoint struct {
	Lat float64
	Lon float64
}

// OFMXObstacle is the canonical ingest model of an obstacle.
type OFMXObstacle struct {
	ID         string
	Type       string
	Name       string
	Lat        float64
	Lon        float64
	HeightM    float64
	ElevationM float64
}
