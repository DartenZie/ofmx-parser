package output

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

func TestGeoJSONFileWriterWriteCreatesAllGeoJSONFiles(t *testing.T) {
	t.Parallel()

	dataset := domain.MapDataset{
		Airports: []domain.MapAirportPoint{{ID: "LKPR", Name: "Prague", Type: "AD", Lat: 50.1, Lon: 14.2, ElevM: 380}},
		Zones: []domain.MapZonePolygon{{
			ID: "LKR1", Name: "Zone", Type: "R", Class: "C", LowM: 0, LowRef: "AGL", UpM: 1000, UpRef: "MSL",
			Polygon: []domain.OFMXGeoPoint{{Lat: 49, Lon: 14}, {Lat: 49.1, Lon: 14.2}, {Lat: 48.9, Lon: 14.3}},
		}},
		PointsOfInterest: []domain.MapPOI{{ID: "VLM", Kind: "VOR", Name: "VLM", Lat: 50.2, Lon: 14.4}},
		AirspaceBorders:  []domain.MapBorderLine{{EdgeID: "E_1", ZoneA: "LKR1", Shared: false, Line: []domain.OFMXGeoPoint{{Lat: 49, Lon: 14}, {Lat: 49.1, Lon: 14.2}}}},
	}

	dir := t.TempDir()
	artifacts, err := (GeoJSONFileWriter{}).Write(context.Background(), dataset, dir)
	if err != nil {
		t.Fatalf("write geojson failed: %v", err)
	}

	paths := []string{artifacts.AirportsPath, artifacts.ZonesPath, artifacts.PointsOfInterestPath, artifacts.AirspaceBordersPath}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %q to exist: %v", path, err)
		}
	}
}

func TestGeoJSONFileWriterWriteClosesPolygonRing(t *testing.T) {
	t.Parallel()

	dataset := domain.MapDataset{
		Zones: []domain.MapZonePolygon{{
			ID: "LKR1", Name: "Zone", Type: "R", Class: "C", LowM: 0, LowRef: "AGL", UpM: 1000, UpRef: "MSL",
			Polygon: []domain.OFMXGeoPoint{{Lat: 49, Lon: 14}, {Lat: 49.1, Lon: 14.2}, {Lat: 48.9, Lon: 14.3}},
		}},
	}

	artifacts, err := (GeoJSONFileWriter{}).Write(context.Background(), dataset, t.TempDir())
	if err != nil {
		t.Fatalf("write geojson failed: %v", err)
	}

	b, err := os.ReadFile(artifacts.ZonesPath)
	if err != nil {
		t.Fatalf("read zones file failed: %v", err)
	}

	var fc map[string]any
	if err := json.Unmarshal(b, &fc); err != nil {
		t.Fatalf("unmarshal zones geojson failed: %v", err)
	}

	features := fc["features"].([]any)
	geometry := features[0].(map[string]any)["geometry"].(map[string]any)
	coords := geometry["coordinates"].([]any)
	ring := coords[0].([]any)

	first := ring[0].([]any)
	last := ring[len(ring)-1].([]any)
	if first[0].(float64) != last[0].(float64) || first[1].(float64) != last[1].(float64) {
		t.Fatalf("expected closed polygon ring, first=%v last=%v", first, last)
	}
}

func TestGeoJSONFileWriterWriteSortsAirportFeaturesByID(t *testing.T) {
	t.Parallel()

	dataset := domain.MapDataset{
		Airports: []domain.MapAirportPoint{
			{ID: "ZZZZ", Name: "Z", Type: "AD", Lat: 1, Lon: 1},
			{ID: "AAAA", Name: "A", Type: "AD", Lat: 2, Lon: 2},
		},
	}

	artifacts, err := (GeoJSONFileWriter{}).Write(context.Background(), dataset, t.TempDir())
	if err != nil {
		t.Fatalf("write geojson failed: %v", err)
	}

	b, err := os.ReadFile(artifacts.AirportsPath)
	if err != nil {
		t.Fatalf("read airports file failed: %v", err)
	}

	var fc map[string]any
	if err := json.Unmarshal(b, &fc); err != nil {
		t.Fatalf("unmarshal airports geojson failed: %v", err)
	}

	features := fc["features"].([]any)
	firstID := features[0].(map[string]any)["properties"].(map[string]any)["id"].(string)
	if firstID != "AAAA" {
		t.Fatalf("expected first airport id AAAA after sorting, got %q", firstID)
	}
}
