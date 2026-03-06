package ingest

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

const (
	earthRadiusMeters      = 6371000.0
	minimumCircleSegments  = 8
	minimumArcChordMeters  = 1.0
	fullCircleRadians      = 2 * math.Pi
	bearingToleranceMeters = 1.0
)

func expandCircularBorder(circle abdCircleXML, opts frontierExpansionOptions) ([]domain.OFMXGeoPoint, error) {
	centerLat, err := parseCoordinate(circle.GeoLatCen, true)
	if err != nil {
		return nil, fmt.Errorf("invalid circle center latitude: %w", err)
	}
	centerLon, err := parseCoordinate(circle.GeoLongCen, false)
	if err != nil {
		return nil, fmt.Errorf("invalid circle center longitude: %w", err)
	}

	radiusM, ok := parseHorizontalDistanceMeters(circle.ValRadius, circle.UOMRadius)
	if !ok {
		return nil, fmt.Errorf("invalid circle radius value=%q uom=%q", strings.TrimSpace(circle.ValRadius), strings.TrimSpace(circle.UOMRadius))
	}

	maxChord := opts.ArcMaxChordLengthM
	if maxChord < minimumArcChordMeters {
		maxChord = minimumArcChordMeters
	}

	segments := segmentsForArc(radiusM, fullCircleRadians, maxChord)
	if segments < minimumCircleSegments {
		segments = minimumCircleSegments
	}

	center := domain.OFMXGeoPoint{Lat: centerLat, Lon: centerLon}
	points := make([]domain.OFMXGeoPoint, 0, segments+1)
	for i := 0; i < segments; i++ {
		bearing := float64(i) * fullCircleRadians / float64(segments)
		points = append(points, destinationPoint(center, bearing, radiusM))
	}
	points = append(points, points[0])
	return dedupeConsecutive(points, opts.CoordinateEpsilon), nil
}

func expandArcSegment(start, end, center domain.OFMXGeoPoint, arcType string, providedRadiusM float64, opts frontierExpansionOptions) []domain.OFMXGeoPoint {
	arcType = strings.ToUpper(strings.TrimSpace(arcType))
	if arcType != "CWA" && arcType != "CCA" {
		return nil
	}

	startRadius := distanceMeters(center, start)
	if startRadius <= 0 {
		return nil
	}

	radiusM := startRadius
	if providedRadiusM > 0 {
		radiusM = providedRadiusM
	}

	startBearing := initialBearing(center, start)
	endBearing := initialBearing(center, end)

	isFullCircle := distanceMeters(start, end) <= bearingToleranceMeters
	var sweep float64
	switch arcType {
	case "CWA":
		sweep = clockwiseSweep(startBearing, endBearing)
	case "CCA":
		sweep = -clockwiseSweep(endBearing, startBearing)
	}
	if isFullCircle {
		if arcType == "CWA" {
			sweep = fullCircleRadians
		} else {
			sweep = -fullCircleRadians
		}
	}
	if sweep == 0 {
		return nil
	}

	maxChord := opts.ArcMaxChordLengthM
	if maxChord < minimumArcChordMeters {
		maxChord = minimumArcChordMeters
	}

	segments := segmentsForArc(radiusM, math.Abs(sweep), maxChord)
	points := make([]domain.OFMXGeoPoint, 0, segments+1)
	points = append(points, start)
	for i := 1; i < segments; i++ {
		frac := float64(i) / float64(segments)
		bearing := normalizeBearing(startBearing + sweep*frac)
		points = append(points, destinationPoint(center, bearing, radiusM))
	}
	points = append(points, end)

	return dedupeConsecutive(points, opts.CoordinateEpsilon)
}

func segmentsForArc(radiusM, sweepRad, maxChordM float64) int {
	if radiusM <= 0 || sweepRad <= 0 {
		return 1
	}

	maxStep := fullCircleRadians
	if maxChordM > 0 && maxChordM < 2*radiusM {
		maxStep = 2 * math.Asin(maxChordM/(2*radiusM))
	}
	if maxStep <= 0 {
		return 1
	}

	segments := int(math.Ceil(sweepRad / maxStep))
	if segments < 1 {
		segments = 1
	}
	return segments
}

func parseHorizontalDistanceMeters(rawValue, rawUOM string) (float64, bool) {
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return 0, false
	}

	switch strings.ToUpper(strings.TrimSpace(rawUOM)) {
	case "M":
		return v, true
	case "KM":
		return v * 1000, true
	case "FT":
		return v * 0.3048, true
	case "NM":
		return v * 1852, true
	default:
		return 0, false
	}
}

func initialBearing(from, to domain.OFMXGeoPoint) float64 {
	lat1 := from.Lat * math.Pi / 180.0
	lat2 := to.Lat * math.Pi / 180.0
	dlon := (to.Lon - from.Lon) * math.Pi / 180.0

	y := math.Sin(dlon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dlon)
	return normalizeBearing(math.Atan2(y, x))
}

func destinationPoint(center domain.OFMXGeoPoint, bearingRad, distanceM float64) domain.OFMXGeoPoint {
	angDist := distanceM / earthRadiusMeters
	lat1 := center.Lat * math.Pi / 180.0
	lon1 := center.Lon * math.Pi / 180.0

	sinLat1 := math.Sin(lat1)
	cosLat1 := math.Cos(lat1)
	sinAng := math.Sin(angDist)
	cosAng := math.Cos(angDist)

	lat2 := math.Asin(sinLat1*cosAng + cosLat1*sinAng*math.Cos(bearingRad))
	lon2 := lon1 + math.Atan2(
		math.Sin(bearingRad)*sinAng*cosLat1,
		cosAng-sinLat1*math.Sin(lat2),
	)

	return domain.OFMXGeoPoint{
		Lat: lat2 * 180.0 / math.Pi,
		Lon: normalizeLongitude(lon2 * 180.0 / math.Pi),
	}
}

func clockwiseSweep(startBearing, endBearing float64) float64 {
	delta := normalizeBearing(endBearing - startBearing)
	if delta < 0 {
		delta += fullCircleRadians
	}
	return delta
}

func normalizeBearing(rad float64) float64 {
	v := math.Mod(rad, fullCircleRadians)
	if v < 0 {
		v += fullCircleRadians
	}
	return v
}

func normalizeLongitude(lon float64) float64 {
	v := math.Mod(lon+180.0, 360.0)
	if v < 0 {
		v += 360.0
	}
	return v - 180.0
}
