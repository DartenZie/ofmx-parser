package ingest

import (
	"fmt"
	"math"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

type airspaceVertex struct {
	Point      domain.OFMXGeoPoint
	CodeType   string
	CodeDatum  string
	GbrMID     string
	GbrTxtName string
	ArcCenter  *domain.OFMXGeoPoint
	ArcRadiusM float64
}

type geographicalBorderIndex struct {
	byMID  map[string][]domain.OFMXGeoPoint
	byName map[string][]domain.OFMXGeoPoint
}

func (i geographicalBorderIndex) find(mid, name string) ([]domain.OFMXGeoPoint, bool) {
	mid = strings.TrimSpace(mid)
	if mid != "" {
		if pts, ok := i.byMID[mid]; ok {
			return pts, true
		}
	}
	name = strings.ToUpper(strings.TrimSpace(name))
	if name != "" {
		if pts, ok := i.byName[name]; ok {
			return pts, true
		}
	}
	return nil, false
}

func buildGeographicalBorderIndex(gbrs []gbrXML, opts frontierExpansionOptions) (geographicalBorderIndex, error) {
	index := geographicalBorderIndex{
		byMID:  make(map[string][]domain.OFMXGeoPoint),
		byName: make(map[string][]domain.OFMXGeoPoint),
	}

	for _, g := range gbrs {
		mid := strings.TrimSpace(g.GbrUID.MID)
		name := strings.TrimSpace(g.GbrUID.TxtName)
		if mid == "" && name == "" {
			continue
		}

		points := make([]domain.OFMXGeoPoint, 0, len(g.Vertices))
		for _, v := range g.Vertices {
			datum := strings.ToUpper(strings.TrimSpace(v.CodeDatum))
			if datum != "" && datum != "WGE" {
				opts.Warningf("OFMX Gbr warning skipped_non_wge_vertex border_uid=%q border_name=%q datum=%q", mid, name, datum)
				continue
			}

			lat, err := parseCoordinate(v.GeoLat, true)
			if err != nil {
				return geographicalBorderIndex{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to parse Gbr vertex latitude for %q", firstNonEmpty(mid, name)), err)
			}
			lon, err := parseCoordinate(v.GeoLong, false)
			if err != nil {
				return geographicalBorderIndex{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to parse Gbr vertex longitude for %q", firstNonEmpty(mid, name)), err)
			}
			points = appendPointUnique(points, domain.OFMXGeoPoint{Lat: lat, Lon: lon}, opts.CoordinateEpsilon)
		}

		if len(points) < 2 {
			continue
		}

		if mid != "" {
			index.byMID[mid] = append([]domain.OFMXGeoPoint(nil), points...)
		}
		if name != "" {
			index.byName[strings.ToUpper(name)] = append([]domain.OFMXGeoPoint(nil), points...)
		}
	}

	return index, nil
}

func expandFrontierSegment(border []domain.OFMXGeoPoint, startAnchor, stopAnchor domain.OFMXGeoPoint, opts frontierExpansionOptions) ([]domain.OFMXGeoPoint, projectedPoint, projectedPoint, bool) {
	if len(border) < 2 {
		return nil, projectedPoint{}, projectedPoint{}, false
	}

	start := nearestPointOnPolyline(border, startAnchor)
	end := nearestPointOnPolyline(border, stopAnchor)

	if start.DistanceM > opts.SnapToleranceMeters || end.DistanceM > opts.SnapToleranceMeters {
		return nil, start, end, false
	}

	return subPathBetween(border, start, end, opts.CoordinateEpsilon), start, end, true
}

func appendPointUnique(points []domain.OFMXGeoPoint, p domain.OFMXGeoPoint, epsilon float64) []domain.OFMXGeoPoint {
	if len(points) == 0 {
		return append(points, p)
	}
	last := points[len(points)-1]
	if pointsEqual(last, p, epsilon) {
		return points
	}
	return append(points, p)
}

func dedupeConsecutive(points []domain.OFMXGeoPoint, epsilon float64) []domain.OFMXGeoPoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]domain.OFMXGeoPoint, 0, len(points))
	for _, p := range points {
		out = appendPointUnique(out, p, epsilon)
	}
	return out
}

func pointsEqual(a, b domain.OFMXGeoPoint, epsilon float64) bool {
	return math.Abs(a.Lat-b.Lat) <= epsilon && math.Abs(a.Lon-b.Lon) <= epsilon
}

type projectedPoint struct {
	Segment   int
	T         float64
	Point     domain.OFMXGeoPoint
	DistanceM float64
}

func nearestPointOnPolyline(line []domain.OFMXGeoPoint, target domain.OFMXGeoPoint) projectedPoint {
	best := projectedPoint{DistanceM: math.MaxFloat64}
	for i := 0; i < len(line)-1; i++ {
		candidate := nearestPointOnSegment(line[i], line[i+1], target)
		candidate.Segment = i
		if candidate.DistanceM < best.DistanceM {
			best = candidate
		}
	}
	if best.DistanceM == math.MaxFloat64 {
		return projectedPoint{Point: line[0]}
	}
	return best
}

func nearestPointOnSegment(a, b, target domain.OFMXGeoPoint) projectedPoint {
	meanLatRad := ((a.Lat + b.Lat + target.Lat) / 3.0) * math.Pi / 180.0
	latScale := 111320.0
	lonScale := 111320.0 * math.Cos(meanLatRad)

	ax, ay := a.Lon*lonScale, a.Lat*latScale
	bx, by := b.Lon*lonScale, b.Lat*latScale
	tx, ty := target.Lon*lonScale, target.Lat*latScale

	dx := bx - ax
	dy := by - ay
	den := dx*dx + dy*dy
	t := 0.0
	if den > 0 {
		t = ((tx-ax)*dx + (ty-ay)*dy) / den
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	px := ax + t*dx
	py := ay + t*dy
	p := domain.OFMXGeoPoint{
		Lat: py / latScale,
		Lon: px / lonScale,
	}

	return projectedPoint{
		T:         t,
		Point:     p,
		DistanceM: distanceMeters(p, target),
	}
}

func subPathBetween(line []domain.OFMXGeoPoint, start, end projectedPoint, epsilon float64) []domain.OFMXGeoPoint {
	if len(line) < 2 {
		return nil
	}

	forward := start.Segment < end.Segment || (start.Segment == end.Segment && start.T <= end.T)
	if forward {
		points := []domain.OFMXGeoPoint{start.Point}
		for i := start.Segment + 1; i <= end.Segment; i++ {
			points = append(points, line[i])
		}
		points = append(points, end.Point)
		return dedupeConsecutive(points, epsilon)
	}

	points := []domain.OFMXGeoPoint{end.Point}
	for i := end.Segment + 1; i <= start.Segment; i++ {
		points = append(points, line[i])
	}
	points = append(points, start.Point)
	points = reversePolyline(points)
	return dedupeConsecutive(points, epsilon)
}

func reversePolyline(points []domain.OFMXGeoPoint) []domain.OFMXGeoPoint {
	out := append([]domain.OFMXGeoPoint(nil), points...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func distanceMeters(a, b domain.OFMXGeoPoint) float64 {
	const earthRadiusM = 6371000.0
	lat1 := a.Lat * math.Pi / 180.0
	lat2 := b.Lat * math.Pi / 180.0
	dlat := (b.Lat - a.Lat) * math.Pi / 180.0
	dlon := (b.Lon - a.Lon) * math.Pi / 180.0

	sinDLat := math.Sin(dlat / 2)
	sinDLon := math.Sin(dlon / 2)
	h := sinDLat*sinDLat + math.Cos(lat1)*math.Cos(lat2)*sinDLon*sinDLon
	c := 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
	return earthRadiusM * c
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
