// Package transform maps canonical OFMX models to custom XML output models.
//
// Author: Miroslav Pašek
package transform

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

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
		CountryBorders:   make([]domain.MapCountryBoundary, 0, len(input.CountryBorders)),
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

	polygonPartsByAirspace := make(map[string][][]domain.OFMXGeoPoint)
	for _, border := range input.AirspaceBorders {
		polygon := normalizePolygonPoints(border.Points)
		if len(polygon) >= 3 {
			polygonPartsByAirspace[border.AirspaceID] = append(polygonPartsByAirspace[border.AirspaceID], polygon)
		}
	}

	allowedAirspaceIDs := make(map[string]struct{}, len(input.Airspaces))
	airspaceTypeByID := make(map[string]string, len(input.Airspaces))
	for _, as := range input.Airspaces {
		if !passesAirspaceFilters(as, allowedTypes, maxLowerFL) {
			continue
		}

		polygon := resolveZonePolygon(polygonPartsByAirspace[as.ID])
		if len(polygon) < 3 {
			continue
		}

		allowedAirspaceIDs[as.ID] = struct{}{}
		airspaceTypeByID[as.ID] = as.Type
		upValue := as.UpperValueM
		if upValue == 0 {
			upValue = as.LowerValueM
		}
		dataset.Zones = append(dataset.Zones, domain.MapZonePolygon{
			ID:      as.ID,
			Name:    as.Name,
			Type:    as.Type,
			Class:   as.Class,
			LowM:    as.LowerValueM,
			LowRef:  mapHeightRef(as.LowerRef),
			UpM:     upValue,
			UpRef:   mapHeightRef(as.UpperRef),
			Rmk:     as.Remark,
			Polygon: polygon,
		})
	}

	sort.Slice(dataset.Zones, func(i, j int) bool {
		return dataset.Zones[i].ID < dataset.Zones[j].ID
	})

	dataset.AirspaceBorders = annotateAirspaceBorders(
		dedupeAirspaceBorders(filterAirspaceBordersByID(input.AirspaceBorders, allowedAirspaceIDs)),
		airspaceTypeByID,
	)

	for _, border := range input.CountryBorders {
		if len(border.Points) < 2 {
			continue
		}

		dataset.CountryBorders = append(dataset.CountryBorders, domain.MapCountryBoundary{
			UID:  border.UID,
			Name: border.Name,
			Line: append([]domain.OFMXGeoPoint(nil), border.Points...),
		})
	}

	sort.Slice(dataset.CountryBorders, func(i, j int) bool {
		if dataset.CountryBorders[i].UID == dataset.CountryBorders[j].UID {
			return dataset.CountryBorders[i].Name < dataset.CountryBorders[j].Name
		}
		return dataset.CountryBorders[i].UID < dataset.CountryBorders[j].UID
	})

	for _, v := range input.NDBs {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, mapNavaidPOI(v.ID, "NDB", firstNonEmpty(v.Name, v.ID), v.Lat, v.Lon))
	}
	for _, v := range input.TACANs {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, mapNavaidPOI(v.ID, "TACAN", firstNonEmpty(v.Name, v.ID), v.Lat, v.Lon))
	}
	for _, v := range input.Markers {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, mapNavaidPOI(v.ID, "MARKER", firstNonEmpty(v.Name, v.ID), v.Lat, v.Lon))
	}
	for _, v := range input.DesignatedPoints {
		dataset.PointsOfInterest = append(dataset.PointsOfInterest, mapNavaidPOI(v.ID, "DESIGNATED", firstNonEmpty(v.Name, v.ID), v.Lat, v.Lon))
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

func resolveZonePolygon(parts [][]domain.OFMXGeoPoint) []domain.OFMXGeoPoint {
	if len(parts) == 0 {
		return nil
	}

	ordered := append([][]domain.OFMXGeoPoint(nil), parts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return polygonAreaAbs(ordered[i]) > polygonAreaAbs(ordered[j])
	})

	out := append([]domain.OFMXGeoPoint(nil), ordered[0]...)
	used := make([]bool, len(ordered))
	used[0] = true

	for {
		merged := false
		for i, part := range ordered {
			if used[i] {
				continue
			}

			next, ok := stitchPolygonChains(out, part)
			if !ok {
				continue
			}

			out = next
			used[i] = true
			merged = true
		}

		if !merged {
			break
		}
	}

	return normalizePolygonPoints(out)
}

func stitchPolygonChains(a, b []domain.OFMXGeoPoint) ([]domain.OFMXGeoPoint, bool) {
	if len(a) < 2 || len(b) < 2 {
		return nil, false
	}

	aStart := a[0]
	aEnd := a[len(a)-1]
	bStart := b[0]
	bEnd := b[len(b)-1]

	switch {
	case pointsEqualWithEpsilon(aEnd, bStart, polygonCoordinateEpsilon):
		out := append(append([]domain.OFMXGeoPoint(nil), a...), b[1:]...)
		return out, true
	case pointsEqualWithEpsilon(aEnd, bEnd, polygonCoordinateEpsilon):
		rb := reversedPoints(b)
		out := append(append([]domain.OFMXGeoPoint(nil), a...), rb[1:]...)
		return out, true
	case pointsEqualWithEpsilon(aStart, bEnd, polygonCoordinateEpsilon):
		out := append(append([]domain.OFMXGeoPoint(nil), b[:len(b)-1]...), a...)
		return out, true
	case pointsEqualWithEpsilon(aStart, bStart, polygonCoordinateEpsilon):
		rb := reversedPoints(b)
		out := append(append([]domain.OFMXGeoPoint(nil), rb[:len(rb)-1]...), a...)
		return out, true
	default:
		return nil, false
	}
}

func reversedPoints(points []domain.OFMXGeoPoint) []domain.OFMXGeoPoint {
	out := append([]domain.OFMXGeoPoint(nil), points...)
	for i := 0; i < len(out)/2; i++ {
		j := len(out) - 1 - i
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func polygonAreaAbs(points []domain.OFMXGeoPoint) float64 {
	if len(points) < 3 {
		return 0
	}

	area2 := 0.0
	for i := 0; i < len(points); i++ {
		next := (i + 1) % len(points)
		area2 += points[i].Lon*points[next].Lat - points[next].Lon*points[i].Lat
	}

	if area2 < 0 {
		area2 = -area2
	}

	return area2 / 2.0
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

type borderSegment struct {
	a quantizedPoint
	b quantizedPoint
}

type graphEdge struct {
	a    quantizedPoint
	b    quantizedPoint
	used bool
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

	type groupedSegments struct {
		zones    []string
		segments []borderSegment
	}
	groups := make(map[string]*groupedSegments)
	groupKeys := make([]string, 0)

	for _, key := range keys {
		agg := edges[key]
		zones := sortedZoneIDs(agg.zones)
		groupKey := strings.Join(zones, "|")

		bucket, ok := groups[groupKey]
		if !ok {
			bucket = &groupedSegments{zones: zones, segments: make([]borderSegment, 0)}
			groups[groupKey] = bucket
			groupKeys = append(groupKeys, groupKey)
		}
		bucket.segments = append(bucket.segments, borderSegment{a: agg.a, b: agg.b})
	}

	sort.Strings(groupKeys)
	out := make([]domain.MapBorderLine, 0, len(keys))
	for _, groupKey := range groupKeys {
		bucket := groups[groupKey]
		lines := stitchSegments(bucket.segments)
		for _, linePoints := range lines {
			line := domain.MapBorderLine{
				EdgeID: edgeLineKey(linePoints),
				Shared: len(bucket.zones) > 1,
				Line:   dequantizedLine(linePoints),
			}

			if len(bucket.zones) > 0 {
				line.ZoneA = bucket.zones[0]
			}
			if len(bucket.zones) > 1 {
				line.ZoneB = bucket.zones[1]
			}

			out = append(out, line)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].EdgeID == out[j].EdgeID {
			if out[i].ZoneA == out[j].ZoneA {
				return out[i].ZoneB < out[j].ZoneB
			}
			return out[i].ZoneA < out[j].ZoneA
		}
		return out[i].EdgeID < out[j].EdgeID
	})

	return out
}

func annotateAirspaceBorders(borders []domain.MapBorderLine, airspaceTypeByID map[string]string) []domain.MapBorderLine {
	if len(borders) == 0 {
		return nil
	}

	out := make([]domain.MapBorderLine, 0, len(borders)*2)
	for _, border := range borders {
		zoneAType := airspaceTypeByID[border.ZoneA]
		zoneBType := airspaceTypeByID[border.ZoneB]

		if border.Shared && border.ZoneA != "" && border.ZoneB != "" && zoneAType != "" && zoneBType != "" && zoneAType != zoneBType {
			first := border
			first.ZoneType = zoneAType

			second := border
			second.ZoneA = border.ZoneB
			second.ZoneB = border.ZoneA
			second.ZoneType = zoneBType

			out = append(out, first, second)
			continue
		}

		border.ZoneType = zoneAType
		out = append(out, border)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].EdgeID != out[j].EdgeID {
			return out[i].EdgeID < out[j].EdgeID
		}
		if out[i].ZoneA != out[j].ZoneA {
			return out[i].ZoneA < out[j].ZoneA
		}
		if out[i].ZoneB != out[j].ZoneB {
			return out[i].ZoneB < out[j].ZoneB
		}
		if out[i].ZoneType != out[j].ZoneType {
			return out[i].ZoneType < out[j].ZoneType
		}
		if out[i].Shared != out[j].Shared {
			return !out[i].Shared && out[j].Shared
		}
		return false
	})

	return out
}

func stitchSegments(segments []borderSegment) [][]quantizedPoint {
	if len(segments) == 0 {
		return nil
	}

	graph := make([]graphEdge, 0, len(segments))
	adj := make(map[quantizedPoint][]int)
	for _, seg := range segments {
		idx := len(graph)
		graph = append(graph, graphEdge{a: seg.a, b: seg.b})
		adj[seg.a] = append(adj[seg.a], idx)
		adj[seg.b] = append(adj[seg.b], idx)
	}

	vertices := make([]quantizedPoint, 0, len(adj))
	for p := range adj {
		vertices = append(vertices, p)
	}
	sort.Slice(vertices, func(i, j int) bool { return compareQuantizedPoints(vertices[i], vertices[j]) < 0 })

	lines := make([][]quantizedPoint, 0, len(segments))

	for _, start := range vertices {
		if len(adj[start]) == 2 {
			continue
		}

		for {
			edgeIdx, ok := firstUnusedIncidentEdge(start, adj[start], graph)
			if !ok {
				break
			}
			lines = append(lines, walkLine(start, edgeIdx, adj, graph))
		}
	}

	for {
		edgeIdx, ok := firstUnusedEdge(graph)
		if !ok {
			break
		}

		start := graph[edgeIdx].a
		if compareQuantizedPoints(graph[edgeIdx].b, start) < 0 {
			start = graph[edgeIdx].b
		}

		lines = append(lines, walkLine(start, edgeIdx, adj, graph))
	}

	sort.Slice(lines, func(i, j int) bool { return edgeLineKey(lines[i]) < edgeLineKey(lines[j]) })
	return lines
}

func firstUnusedIncidentEdge(at quantizedPoint, edgeIndexes []int, graph []graphEdge) (int, bool) {
	bestIdx := -1
	var bestOther quantizedPoint

	for _, idx := range edgeIndexes {
		if graph[idx].used {
			continue
		}
		other := graph[idx].a
		if other == at {
			other = graph[idx].b
		}

		if bestIdx == -1 || compareQuantizedPoints(other, bestOther) < 0 || (other == bestOther && idx < bestIdx) {
			bestIdx = idx
			bestOther = other
		}
	}

	if bestIdx == -1 {
		return 0, false
	}
	return bestIdx, true
}

func firstUnusedEdge(graph []graphEdge) (int, bool) {
	bestIdx := -1
	var bestA, bestB quantizedPoint

	for idx := range graph {
		if graph[idx].used {
			continue
		}

		a := graph[idx].a
		b := graph[idx].b
		if compareQuantizedPoints(b, a) < 0 {
			a, b = b, a
		}

		if bestIdx == -1 || compareQuantizedPoints(a, bestA) < 0 || (a == bestA && (compareQuantizedPoints(b, bestB) < 0 || (b == bestB && idx < bestIdx))) {
			bestIdx = idx
			bestA = a
			bestB = b
		}
	}

	if bestIdx == -1 {
		return 0, false
	}
	return bestIdx, true
}

func walkLine(start quantizedPoint, startEdge int, adj map[quantizedPoint][]int, graph []graphEdge) []quantizedPoint {
	line := []quantizedPoint{start}
	current := start
	edgeIdx := startEdge

	for {
		if graph[edgeIdx].used {
			break
		}

		next := graph[edgeIdx].a
		if next == current {
			next = graph[edgeIdx].b
		}

		graph[edgeIdx].used = true
		line = append(line, next)

		if len(adj[next]) != 2 {
			break
		}

		nextEdge, ok := firstUnusedIncidentEdge(next, adj[next], graph)
		if !ok {
			break
		}

		current = next
		edgeIdx = nextEdge
	}

	return line
}

func dequantizedLine(points []quantizedPoint) []domain.OFMXGeoPoint {
	line := make([]domain.OFMXGeoPoint, 0, len(points))
	for _, p := range points {
		line = append(line, dequantizePoint(p))
	}
	return line
}

func edgeLineKey(points []quantizedPoint) string {
	forward := encodeQuantizedPath(points)
	reverse := encodeQuantizedPath(reversedQuantizedPoints(points))
	if reverse < forward {
		return "L_" + reverse
	}
	return "L_" + forward
}

func encodeQuantizedPath(points []quantizedPoint) string {
	b := strings.Builder{}
	for i, p := range points {
		if i > 0 {
			b.WriteString("_")
		}
		b.WriteString(fmt.Sprintf("%d_%d", p.lat, p.lon))
	}
	return b.String()
}

func reversedQuantizedPoints(points []quantizedPoint) []quantizedPoint {
	out := append([]quantizedPoint(nil), points...)
	for i := 0; i < len(out)/2; i++ {
		j := len(out) - 1 - i
		out[i], out[j] = out[j], out[i]
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

var navaidPhoneticAlphabet = map[rune]string{
	'A': "Alpha",
	'B': "Bravo",
	'C': "Charlie",
	'D': "Delta",
	'E': "Echo",
	'F': "Foxtrot",
	'G': "Golf",
	'H': "Hotel",
	'I': "India",
	'J': "Juliett",
	'K': "Kilo",
	'L': "Lima",
	'M': "Mike",
	'N': "November",
	'O': "Oscar",
	'P': "Papa",
	'Q': "Quebec",
	'R': "Romeo",
	'S': "Sierra",
	'T': "Tango",
	'U': "Uniform",
	'V': "Victor",
	'W': "Whiskey",
	'X': "X-ray",
	'Y': "Yankee",
	'Z': "Zulu",
}

func mapNavaidPOI(id, kind, defaultName string, lat, lon float64) domain.MapPOI {
	vocalicName, ok := vocalicNavaidName(id)
	if ok {
		return domain.MapPOI{ID: id, Kind: kind, Name: vocalicName, Type: "vocalic", Lat: lat, Lon: lon}
	}

	return domain.MapPOI{ID: id, Kind: kind, Name: defaultName, Lat: lat, Lon: lon}
}

func vocalicNavaidName(id string) (string, bool) {
	trimmed := strings.TrimSpace(id)
	if len([]rune(trimmed)) != 1 {
		return trimmed, false
	}

	r := []rune(strings.ToUpper(trimmed))[0]
	word, ok := navaidPhoneticAlphabet[r]
	if !ok {
		return trimmed, false
	}

	return word, true
}
