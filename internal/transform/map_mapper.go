// Package transform maps canonical OFMX models to custom XML output models.
//
// Author: Miroslav Pašek
package transform

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// MapMapper maps ingested OFMX data into map export intermediate model.
type MapMapper interface {
	MapToMapDataset(ctx context.Context, input domain.OFMXDocument) (domain.MapDataset, error)
}

// DefaultMapMapper provides the default OFMX to map-dataset mapping implementation.
type DefaultMapMapper struct {
	AllowedAirspaceTypes []string
	MaxAirspaceLowerFL   int
}

// MapToMapDataset maps OFMXDocument into map-oriented intermediate structures.
func (m DefaultMapMapper) MapToMapDataset(_ context.Context, input domain.OFMXDocument) (domain.MapDataset, error) {
	allowedTypes := effectiveAllowedAirspaceTypeSet(m.AllowedAirspaceTypes)
	maxLowerFL := effectiveMaxAirspaceLowerFL(m.MaxAirspaceLowerFL)

	dataset := domain.MapDataset{
		Airports:         make([]domain.MapAirportPoint, 0, len(input.Airports)),
		Zones:            make([]domain.MapZonePolygon, 0, len(input.Airspaces)),
		PointsOfInterest: make([]domain.MapPOI, 0, len(input.VORs)+len(input.NDBs)+len(input.DMEs)+len(input.TACANs)+len(input.Markers)+len(input.DesignatedPoints)+len(input.Obstacles)),
		AirspaceBorders:  make([]domain.MapBorderLine, 0),
	}

	for _, ap := range input.Airports {
		dataset.Airports = append(dataset.Airports, domain.MapAirportPoint{
			ID:    ap.ID,
			Name:  ap.Name,
			Type:  ap.Type,
			Lat:   ap.Lat,
			Lon:   ap.Lon,
			ElevM: ap.ElevM,
		})
	}

	sort.Slice(dataset.Airports, func(i, j int) bool {
		return dataset.Airports[i].ID < dataset.Airports[j].ID
	})

	polygonByAirspace := make(map[string][]domain.OFMXGeoPoint)
	for _, border := range input.AirspaceBorders {
		if _, exists := polygonByAirspace[border.AirspaceID]; exists {
			continue
		}
		if len(border.Points) >= 3 {
			polygonByAirspace[border.AirspaceID] = append([]domain.OFMXGeoPoint(nil), border.Points...)
		}
	}

	allowedAirspaceIDs := make(map[string]struct{}, len(input.Airspaces))
	for _, as := range input.Airspaces {
		if !passesAirspaceFilters(as, allowedTypes, maxLowerFL) {
			continue
		}

		allowedAirspaceIDs[as.ID] = struct{}{}
		dataset.Zones = append(dataset.Zones, domain.MapZonePolygon{
			ID:      as.ID,
			Name:    as.Name,
			Type:    as.Type,
			Class:   as.Class,
			LowM:    as.LowerValueM,
			LowRef:  mapHeightRef(as.LowerRef),
			UpM:     as.UpperValueM,
			UpRef:   mapHeightRef(as.UpperRef),
			Rmk:     as.Remark,
			Polygon: polygonByAirspace[as.ID],
		})
	}

	sort.Slice(dataset.Zones, func(i, j int) bool {
		return dataset.Zones[i].ID < dataset.Zones[j].ID
	})

	dataset.AirspaceBorders = dedupeAirspaceBorders(filterAirspaceBordersByID(input.AirspaceBorders, allowedAirspaceIDs))

	for _, v := range input.VORs {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, domain.MapPOI{ID: v.ID, Kind: "VOR", Name: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.NDBs {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, domain.MapPOI{ID: v.ID, Kind: "NDB", Name: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.DMEs {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, domain.MapPOI{ID: v.ID, Kind: "DME", Name: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.TACANs {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, domain.MapPOI{ID: v.ID, Kind: "TACAN", Name: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.Markers {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, domain.MapPOI{ID: v.ID, Kind: "MARKER", Name: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.DesignatedPoints {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, domain.MapPOI{ID: v.ID, Kind: "DESIGNATED", Name: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}
	for _, v := range input.Obstacles {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, domain.MapPOI{ID: v.ID, Kind: "OBSTACLE", Name: firstNonEmpty(v.Name, v.ID), Lat: v.Lat, Lon: v.Lon})
	}

	sort.Slice(dataset.PointsOfInterest, func(i, j int) bool {
		if dataset.PointsOfInterest[i].Kind == dataset.PointsOfInterest[j].Kind {
			return dataset.PointsOfInterest[i].ID < dataset.PointsOfInterest[j].ID
		}
		return dataset.PointsOfInterest[i].Kind < dataset.PointsOfInterest[j].Kind
	})

	return dataset, nil
}

func filterAirspaceBordersByID(borders []domain.OFMXAirspaceBorder, allowed map[string]struct{}) []domain.OFMXAirspaceBorder {
	if len(allowed) == 0 || len(borders) == 0 {
		return nil
	}

	out := make([]domain.OFMXAirspaceBorder, 0, len(borders))
	for _, border := range borders {
		if _, ok := allowed[border.AirspaceID]; !ok {
			continue
		}
		out = append(out, border)
	}

	return out
}

const borderQuantizationFactor = 1_000_000.0

type quantizedPoint struct {
	lat int64
	lon int64
}

type borderAggregate struct {
	a     quantizedPoint
	b     quantizedPoint
	zones map[string]struct{}
}

func dedupeAirspaceBorders(borders []domain.OFMXAirspaceBorder) []domain.MapBorderLine {
	edges := make(map[string]*borderAggregate)

	for _, border := range borders {
		segments := borderSegments(border.Points)
		for _, seg := range segments {
			a := quantizePoint(seg[0])
			b := quantizePoint(seg[1])
			if a == b {
				continue
			}

			key, ca, cb := canonicalEdge(a, b)
			agg, ok := edges[key]
			if !ok {
				agg = &borderAggregate{
					a:     ca,
					b:     cb,
					zones: make(map[string]struct{}),
				}
				edges[key] = agg
			}

			if border.AirspaceID != "" {
				agg.zones[border.AirspaceID] = struct{}{}
			}
		}
	}

	keys := make([]string, 0, len(edges))
	for key := range edges {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]domain.MapBorderLine, 0, len(keys))
	for _, key := range keys {
		agg := edges[key]
		zones := sortedZoneIDs(agg.zones)

		line := domain.MapBorderLine{
			EdgeID: key,
			Shared: len(zones) > 1,
			Line: []domain.OFMXGeoPoint{
				dequantizePoint(agg.a),
				dequantizePoint(agg.b),
			},
		}

		if len(zones) > 0 {
			line.ZoneA = zones[0]
		}
		if len(zones) > 1 {
			line.ZoneB = zones[1]
		}

		out = append(out, line)
	}

	return out
}

func borderSegments(points []domain.OFMXGeoPoint) [][2]domain.OFMXGeoPoint {
	if len(points) < 2 {
		return nil
	}

	ring := append([]domain.OFMXGeoPoint(nil), points...)
	qFirst := quantizePoint(ring[0])
	qLast := quantizePoint(ring[len(ring)-1])
	if qFirst != qLast {
		ring = append(ring, ring[0])
	}

	segments := make([][2]domain.OFMXGeoPoint, 0, len(ring)-1)
	for i := 0; i < len(ring)-1; i++ {
		segments = append(segments, [2]domain.OFMXGeoPoint{ring[i], ring[i+1]})
	}

	return segments
}

func quantizePoint(p domain.OFMXGeoPoint) quantizedPoint {
	return quantizedPoint{
		lat: int64(math.Round(p.Lat * borderQuantizationFactor)),
		lon: int64(math.Round(p.Lon * borderQuantizationFactor)),
	}
}

func dequantizePoint(p quantizedPoint) domain.OFMXGeoPoint {
	return domain.OFMXGeoPoint{
		Lat: float64(p.lat) / borderQuantizationFactor,
		Lon: float64(p.lon) / borderQuantizationFactor,
	}
}

func canonicalEdge(a, b quantizedPoint) (string, quantizedPoint, quantizedPoint) {
	if compareQuantizedPoints(a, b) <= 0 {
		return edgeKey(a, b), a, b
	}
	return edgeKey(b, a), b, a
}

func compareQuantizedPoints(a, b quantizedPoint) int {
	if a.lat < b.lat {
		return -1
	}
	if a.lat > b.lat {
		return 1
	}
	if a.lon < b.lon {
		return -1
	}
	if a.lon > b.lon {
		return 1
	}
	return 0
}

func edgeKey(a, b quantizedPoint) string {
	return fmt.Sprintf("E_%d_%d_%d_%d", a.lat, a.lon, b.lat, b.lon)
}

func sortedZoneIDs(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}

	out := make([]string, 0, len(m))
	for zoneID := range m {
		out = append(out, zoneID)
	}
	sort.Strings(out)
	return out
}
