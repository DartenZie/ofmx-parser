// Package transform maps canonical OFMX models to custom XML output models.
//
// Author: Miroslav Pašek
package transform

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

var defaultAllowedAirspaceTypes = map[string]struct{}{
	"ATZ":    {},
	"CTR":    {},
	"TMA":    {},
	"D":      {},
	"P":      {},
	"PR":     {},
	"R":      {},
	"TRA":    {},
	"TRA_GA": {},
	"TSA":    {},
}

type Mapper interface {
	Map(ctx context.Context, input domain.OFMXDocument) (domain.OutputDocument, error)
}

// DefaultMapper implements the default mapping ruleset.
type DefaultMapper struct {
	AllowedAirspaceTypes []string
	MaxAirspaceLowerFL   int
}

// Map transforms ingested OFMX data into the target output document.
func (m DefaultMapper) Map(_ context.Context, input domain.OFMXDocument) (domain.OutputDocument, error) {
	airports, err := mapAirports(input)
	if err != nil {
		return domain.OutputDocument{}, err
	}

	navaids := mapNavaids(input)
	airspaces := mapAirspaces(
		input,
		effectiveAllowedAirspaceTypeSet(m.AllowedAirspaceTypes),
		effectiveMaxAirspaceLowerFL(m.MaxAirspaceLowerFL),
	)
	obstacles := mapObstacles(input)

	region := firstRegion(input.SnapshotMeta.Regions)
	if region == "" {
		region = "GLOBAL"
	}

	cycle := normalizeCycle(input.SnapshotMeta.Effective)
	if cycle == "" {
		cycle = "UNKNOWN"
	}

	out := domain.OutputDocument{
		Cycle:       cycle,
		Region:      region,
		GeneratedAt: input.SnapshotMeta.Created,
		Schema:      "output.xsd",
		Source:      input.SnapshotMeta.Origin,
	}

	if len(airports) > 0 {
		out.Airports = &domain.OutputAirports{Airports: airports}
	}

	if len(navaids) > 0 {
		out.Navaids = &domain.OutputNavaids{Navaids: navaids}
	}

	if len(airspaces) > 0 {
		out.Airspaces = &domain.OutputAirspaces{Airspaces: airspaces}
	}

	if len(obstacles) > 0 {
		out.Obstacles = &domain.OutputObstacles{Obstacles: obstacles}
	}

	return out, nil
}

func mapAirports(input domain.OFMXDocument) ([]domain.OutputAirport, error) {
	rwyByAirport := make(map[string][]domain.OFMXRunway)
	for _, rwy := range input.Runways {
		rwyByAirport[rwy.AirportID] = append(rwyByAirport[rwy.AirportID], rwy)
	}

	rdnByRunway := make(map[string][]domain.OFMXRunwayDirection)
	for _, rdn := range input.RunwayDirections {
		key := runwayKey(rdn.AirportID, rdn.RunwayDesign)
		rdnByRunway[key] = append(rdnByRunway[key], rdn)
	}

	out := make([]domain.OutputAirport, 0, len(input.Airports))
	for _, ap := range input.Airports {
		if strings.TrimSpace(ap.ID) == "" {
			return nil, domain.NewError(domain.ErrTransform, "airport missing ID", nil)
		}
		if strings.TrimSpace(ap.Name) == "" {
			return nil, domain.NewError(domain.ErrTransform, fmt.Sprintf("airport %q missing name", ap.ID), nil)
		}

		mapped := domain.OutputAirport{
			ID:    ap.ID,
			D:     firstNonEmpty(ap.Type, "UNKNOWN"),
			N:     ap.Name,
			Lat:   ap.Lat,
			Lon:   ap.Lon,
			ElevM: ap.ElevM,
		}

		runways := rwyByAirport[ap.ID]
		if len(runways) > 0 {
			mapped.Runways = &domain.OutputRunways{Runways: mapRunways(ap.ID, runways, rdnByRunway)}
		}

		out = append(out, mapped)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out, nil
}

func mapRunways(airportID string, runways []domain.OFMXRunway, dirs map[string][]domain.OFMXRunwayDirection) []domain.OutputRunway {
	out := make([]domain.OutputRunway, 0, len(runways))

	for _, rwy := range runways {
		key := runwayKey(airportID, rwy.Designation)
		rdn := dirs[key]
		mappedDirs := mapDirections(rwy.Designation, rdn)

		out = append(out, domain.OutputRunway{
			N:    firstNonEmpty(rwy.Designation, "RWY"),
			LenM: rwy.LengthM,
			WidM: rwy.WidthM,
			Comp: rwy.Composition,
			Prep: rwy.Preparation,
			Dirs: mappedDirs,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].N < out[j].N
	})

	return out
}

func mapDirections(runwayDesignation string, dirs []domain.OFMXRunwayDirection) []domain.OutputRunwayDirection {
	if len(dirs) == 0 {
		brg := inferBearing(runwayDesignation)
		return []domain.OutputRunwayDirection{{
			Bearing: brg,
			Code:    bearingToCardinal(brg),
		}}
	}

	out := make([]domain.OutputRunwayDirection, 0, len(dirs))
	for _, dir := range dirs {
		brg := dir.TrueBearing
		if brg == 0 {
			if dir.MagBearing != 0 {
				brg = dir.MagBearing
			} else {
				brg = inferBearing(dir.Designator)
			}
		}

		out = append(out, domain.OutputRunwayDirection{
			Bearing: brg,
			Code:    bearingToCardinal(brg),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Bearing < out[j].Bearing
	})

	return out
}

func mapNavaids(input domain.OFMXDocument) []domain.OutputNavaid {
	out := make([]domain.OutputNavaid, 0, len(input.VORs)+len(input.NDBs)+len(input.DMEs)+len(input.TACANs)+len(input.Markers)+len(input.DesignatedPoints))

	for _, v := range input.VORs {
		out = append(out, domain.OutputNavaid{ID: v.ID, T: "VOR", D: firstNonEmpty(v.Type, "VOR"), N: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.NDBs {
		out = append(out, domain.OutputNavaid{ID: v.ID, T: "NDB", D: firstNonEmpty(v.Class, "NDB"), N: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.DMEs {
		out = append(out, domain.OutputNavaid{ID: v.ID, T: "DME", D: firstNonEmpty(v.Type, "DME"), N: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.TACANs {
		out = append(out, domain.OutputNavaid{ID: v.ID, T: "TACAN", D: "TACAN", N: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.Markers {
		out = append(out, domain.OutputNavaid{ID: v.ID, T: "MARKER", D: firstNonEmpty(v.Class, "MARKER"), N: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.DesignatedPoints {
		out = append(out, domain.OutputNavaid{ID: v.ID, T: "DESIGNATED", D: firstNonEmpty(v.Type, "DESIGNATED"), N: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == out[j].ID {
			return out[i].T < out[j].T
		}
		return out[i].ID < out[j].ID
	})

	return out
}

func mapAirspaces(input domain.OFMXDocument, allowedTypes map[string]struct{}, maxLowerFL int) []domain.OutputAirspace {
	borderByAirspace := make(map[string][]domain.OFMXGeoPoint)
	for _, border := range input.AirspaceBorders {
		if strings.TrimSpace(border.AirspaceID) == "" || len(border.Points) == 0 {
			continue
		}
		borderByAirspace[border.AirspaceID] = append(borderByAirspace[border.AirspaceID], border.Points...)
	}

	out := make([]domain.OutputAirspace, 0, len(input.Airspaces))
	for _, as := range input.Airspaces {
		if !passesAirspaceFilters(as, allowedTypes, maxLowerFL) {
			continue
		}

		pts := dedupePoints(borderByAirspace[as.ID])
		if len(pts) < 3 {
			continue
		}

		poly := make([]domain.OutputPoint, 0, len(pts))
		minLat, minLon := pts[0].Lat, pts[0].Lon
		maxLat, maxLon := pts[0].Lat, pts[0].Lon

		for _, p := range pts {
			poly = append(poly, domain.OutputPoint{Lat: p.Lat, Lon: p.Lon})
			if p.Lat < minLat {
				minLat = p.Lat
			}
			if p.Lat > maxLat {
				maxLat = p.Lat
			}
			if p.Lon < minLon {
				minLon = p.Lon
			}
			if p.Lon > maxLon {
				maxLon = p.Lon
			}
		}

		lowRef := mapHeightRef(as.LowerRef)
		upRef := mapHeightRef(as.UpperRef)
		upValue := as.UpperValueM
		if upValue == 0 {
			upValue = as.LowerValueM
		}

		out = append(out, domain.OutputAirspace{
			ID:     firstNonEmpty(as.ID),
			D:      firstNonEmpty(as.Class, as.Activity, as.Type, "UNKNOWN"),
			N:      firstNonEmpty(as.Name, as.ID),
			T:      firstNonEmpty(as.Type, "UNKNOWN"),
			LowM:   as.LowerValueM,
			LowRef: lowRef,
			UpM:    upValue,
			UpRef:  upRef,
			Rmk:    as.Remark,
			Poly:   domain.OutputPolygon{Points: poly},
			BBox: domain.OutputBBox{
				MinLat: minLat,
				MinLon: minLon,
				MaxLat: maxLat,
				MaxLon: maxLon,
			},
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

func isAllowedAirspaceType(raw string, allowedTypes map[string]struct{}) bool {
	_, ok := allowedTypes[strings.ToUpper(strings.TrimSpace(raw))]
	return ok
}

func effectiveAllowedAirspaceTypeSet(custom []string) map[string]struct{} {
	if len(custom) == 0 {
		return defaultAllowedAirspaceTypes
	}

	out := make(map[string]struct{}, len(custom))
	for _, raw := range custom {
		v := strings.ToUpper(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		out[v] = struct{}{}
	}
	if len(out) == 0 {
		return defaultAllowedAirspaceTypes
	}

	return out
}

func mapObstacles(input domain.OFMXDocument) []domain.OutputObstacle {
	out := make([]domain.OutputObstacle, 0, len(input.Obstacles))
	for i, obs := range input.Obstacles {
		id := strings.TrimSpace(obs.ID)
		if id == "" {
			id = fmt.Sprintf("OBS_%d", i+1)
		}

		out = append(out, domain.OutputObstacle{
			ID:    id,
			T:     firstNonEmpty(obs.Type, "UNKNOWN"),
			N:     firstNonEmpty(obs.Name, id),
			Lat:   obs.Lat,
			Lon:   obs.Lon,
			HM:    obs.HeightM,
			ElevM: obs.ElevationM,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

func dedupePoints(points []domain.OFMXGeoPoint) []domain.OFMXGeoPoint {
	seen := make(map[string]struct{}, len(points))
	out := make([]domain.OFMXGeoPoint, 0, len(points))
	for _, p := range points {
		key := fmt.Sprintf("%.7f|%.7f", p.Lat, p.Lon)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

func mapHeightRef(ofmxCode string) string {
	v := strings.ToUpper(strings.TrimSpace(ofmxCode))
	switch {
	case strings.Contains(v, "UNL"):
		return "UNL"
	case strings.Contains(v, "FL"):
		return "FL"
	case strings.Contains(v, "SFC"), strings.Contains(v, "AGL"), strings.Contains(v, "HEI"):
		return "AGL"
	case strings.Contains(v, "MSL"), strings.Contains(v, "AMSL"):
		return "MSL"
	case v == "":
		return "STD"
	default:
		return "STD"
	}
}

func runwayKey(airportID, runwayDesignation string) string {
	return airportID + "|" + runwayDesignation
}

func firstRegion(regions string) string {
	parts := strings.Fields(strings.TrimSpace(regions))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func normalizeCycle(effective string) string {
	if len(effective) >= 10 {
		date := effective[:10]
		return strings.ReplaceAll(date, "-", "")
	}
	return strings.TrimSpace(effective)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func inferBearing(designator string) float64 {
	d := strings.TrimSpace(designator)
	if d == "" {
		return 0
	}

	num := ""
	for i := 0; i < len(d); i++ {
		if d[i] >= '0' && d[i] <= '9' {
			num += string(d[i])
		} else {
			break
		}
	}

	if num == "" {
		return 0
	}

	v, err := strconv.Atoi(num)
	if err != nil {
		return 0
	}

	brg := float64(v * 10)
	if brg > 360 {
		brg = math.Mod(brg, 360)
	}
	return brg
}

func bearingToCardinal(bearing float64) string {
	cardinals := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	n := len(cardinals)
	index := int(math.Mod(math.Round(bearing/22.5), float64(n)))
	if index < 0 {
		index += n
	}
	return cardinals[index]
}
