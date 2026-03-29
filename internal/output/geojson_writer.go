// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pašek
package output

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

const (
	airportsGeoJSONFileName         = "aviation_airports.geojson"
	zonesGeoJSONFileName            = "aviation_zones.geojson"
	pointsOfInterestGeoJSONFileName = "aviation_poi.geojson"
	airspaceBordersGeoJSONFileName  = "aviation_airspace_borders.geojson"
)

// MapGeoJSONWriter writes map dataset layers as GeoJSON source files.
type MapGeoJSONWriter interface {
	Write(ctx context.Context, dataset domain.MapDataset, dir string) (domain.MapGeoJSONArtifacts, error)
}

// GeoJSONFileWriter serializes map dataset layers into GeoJSON files.
type GeoJSONFileWriter struct{}

// Write writes four aviation GeoJSON files to dir.
func (w GeoJSONFileWriter) Write(ctx context.Context, dataset domain.MapDataset, dir string) (domain.MapGeoJSONArtifacts, error) {
	if err := ctx.Err(); err != nil {
		return domain.MapGeoJSONArtifacts{}, domain.NewError(domain.ErrOutput, "GeoJSON write cancelled", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return domain.MapGeoJSONArtifacts{}, domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create map temp dir %q", dir), err)
	}

	artifacts := domain.MapGeoJSONArtifacts{
		AirportsPath:         filepath.Join(dir, airportsGeoJSONFileName),
		ZonesPath:            filepath.Join(dir, zonesGeoJSONFileName),
		PointsOfInterestPath: filepath.Join(dir, pointsOfInterestGeoJSONFileName),
		AirspaceBordersPath:  filepath.Join(dir, airspaceBordersGeoJSONFileName),
	}

	if err := writeGeoJSON(ctx, artifacts.AirportsPath, airportsFeatureCollection(dataset.Airports)); err != nil {
		return domain.MapGeoJSONArtifacts{}, err
	}
	if err := writeGeoJSON(ctx, artifacts.ZonesPath, zonesFeatureCollection(dataset.Zones)); err != nil {
		return domain.MapGeoJSONArtifacts{}, err
	}
	if err := writeGeoJSON(ctx, artifacts.PointsOfInterestPath, poiFeatureCollection(dataset.PointsOfInterest)); err != nil {
		return domain.MapGeoJSONArtifacts{}, err
	}
	if err := writeGeoJSON(ctx, artifacts.AirspaceBordersPath, bordersFeatureCollection(dataset.AirspaceBorders)); err != nil {
		return domain.MapGeoJSONArtifacts{}, err
	}

	return artifacts, nil
}

type geoJSONFeatureCollection struct {
	Type     string           `json:"type"`
	Features []geoJSONFeature `json:"features"`
}

type geoJSONFeature struct {
	Type       string         `json:"type"`
	Geometry   any            `json:"geometry"`
	Properties map[string]any `json:"properties"`
}

type geoJSONPoint struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

type geoJSONLineString struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

type geoJSONPolygon struct {
	Type        string        `json:"type"`
	Coordinates [][][]float64 `json:"coordinates"`
}

func airportsFeatureCollection(airports []domain.MapAirportPoint) geoJSONFeatureCollection {
	sorted := append([]domain.MapAirportPoint(nil), airports...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	features := make([]geoJSONFeature, 0, len(sorted))
	for _, ap := range sorted {
		features = append(features, geoJSONFeature{
			Type: "Feature",
			Geometry: geoJSONPoint{
				Type:        "Point",
				Coordinates: []float64{ap.Lon, ap.Lat},
			},
			Properties: map[string]any{
				"id":     ap.ID,
				"name":   ap.Name,
				"type":   ap.Type,
				"elev_m": ap.ElevM,
			},
		})
	}

	return geoJSONFeatureCollection{Type: "FeatureCollection", Features: features}
}

func zonesFeatureCollection(zones []domain.MapZonePolygon) geoJSONFeatureCollection {
	sorted := append([]domain.MapZonePolygon(nil), zones...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	features := make([]geoJSONFeature, 0, len(sorted))
	for _, zone := range sorted {
		coords := polygonCoordinates(zone.Polygon)
		if len(coords) == 0 {
			continue
		}

		features = append(features, geoJSONFeature{
			Type: "Feature",
			Geometry: geoJSONPolygon{
				Type:        "Polygon",
				Coordinates: [][][]float64{coords},
			},
			Properties: map[string]any{
				"id":        zone.ID,
				"name":      zone.Name,
				"zone_type": zone.Type,
				"class":     zone.Class,
				"low_m":     zone.LowM,
				"low_ref":   zone.LowRef,
				"up_m":      zone.UpM,
				"up_ref":    zone.UpRef,
				"rmk":       zone.Rmk,
			},
		})
	}

	return geoJSONFeatureCollection{Type: "FeatureCollection", Features: features}
}

func poiFeatureCollection(pois []domain.MapPOI) geoJSONFeatureCollection {
	sorted := append([]domain.MapPOI(nil), pois...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Kind == sorted[j].Kind {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].Kind < sorted[j].Kind
	})

	features := make([]geoJSONFeature, 0, len(sorted))
	for _, poi := range sorted {
		features = append(features, geoJSONFeature{
			Type: "Feature",
			Geometry: geoJSONPoint{
				Type:        "Point",
				Coordinates: []float64{poi.Lon, poi.Lat},
			},
			Properties: map[string]any{
				"id":   poi.ID,
				"kind": poi.Kind,
				"name": poi.Name,
			},
		})
	}

	return geoJSONFeatureCollection{Type: "FeatureCollection", Features: features}
}

func bordersFeatureCollection(borders []domain.MapBorderLine) geoJSONFeatureCollection {
	sorted := append([]domain.MapBorderLine(nil), borders...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].EdgeID < sorted[j].EdgeID })

	features := make([]geoJSONFeature, 0, len(sorted))
	for _, border := range sorted {
		if len(border.Line) < 2 {
			continue
		}

		coords := make([][]float64, 0, len(border.Line))
		for _, p := range border.Line {
			coords = append(coords, []float64{p.Lon, p.Lat})
		}

		features = append(features, geoJSONFeature{
			Type: "Feature",
			Geometry: geoJSONLineString{
				Type:        "LineString",
				Coordinates: coords,
			},
			Properties: map[string]any{
				"edge_id": border.EdgeID,
				"zone_a":  border.ZoneA,
				"zone_b":  border.ZoneB,
				"shared":  border.Shared,
			},
		})
	}

	return geoJSONFeatureCollection{Type: "FeatureCollection", Features: features}
}

func polygonCoordinates(points []domain.OFMXGeoPoint) [][]float64 {
	if len(points) < 3 {
		return nil
	}

	ring := make([][]float64, 0, len(points)+1)
	for _, p := range points {
		ring = append(ring, []float64{p.Lon, p.Lat})
	}

	first := ring[0]
	last := ring[len(ring)-1]
	if first[0] != last[0] || first[1] != last[1] {
		ring = append(ring, []float64{first[0], first[1]})
	}

	return ring
}

func writeGeoJSON(ctx context.Context, path string, fc geoJSONFeatureCollection) error {
	if err := ctx.Err(); err != nil {
		return domain.NewError(domain.ErrOutput, "GeoJSON write cancelled", err)
	}

	b, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to marshal GeoJSON for %q", path), err)
	}

	if err := writeFileAtomic(ctx, path, append(b, '\n'), 0o644); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to write GeoJSON file %q", path), err)
	}

	return nil
}
