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
		CountryBorders:   []domain.MapCountryBoundary{{UID: "BORDER-1", Name: "CZ-DE", Line: []domain.OFMXGeoPoint{{Lat: 49, Lon: 14}, {Lat: 49.1, Lon: 14.2}}}},
	}

	dir := t.TempDir()
	artifacts, err := (GeoJSONFileWriter{}).Write(context.Background(), dataset, dir)
	if err != nil {
		t.Fatalf("write geojson failed: %v", err)
	}

	paths := []string{artifacts.AirportsPath, artifacts.ZonesPath, artifacts.PointsOfInterestPath, artifacts.AirspaceBordersPath, artifacts.CountriesBoundaryPath}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %q to exist: %v", path, err)
		}
	}
}

func TestGeoJSONFileWriterWriteSortsCountryBoundariesByUID(t *testing.T) {
	t.Parallel()

	dataset := domain.MapDataset{
		CountryBorders: []domain.MapCountryBoundary{
			{UID: "ZZ", Name: "Z", Line: []domain.OFMXGeoPoint{{Lat: 1, Lon: 1}, {Lat: 1.1, Lon: 1.1}}},
			{UID: "AA", Name: "A", Line: []domain.OFMXGeoPoint{{Lat: 2, Lon: 2}, {Lat: 2.1, Lon: 2.1}}},
		},
	}

	artifacts, err := (GeoJSONFileWriter{}).Write(context.Background(), dataset, t.TempDir())
	if err != nil {
		t.Fatalf("write geojson failed: %v", err)
	}

	b, err := os.ReadFile(artifacts.CountriesBoundaryPath)
	if err != nil {
		t.Fatalf("read countries boundary file failed: %v", err)
	}

	var fc map[string]any
	if err := json.Unmarshal(b, &fc); err != nil {
		t.Fatalf("unmarshal countries boundary geojson failed: %v", err)
	}

	features := fc["features"].([]any)
	firstUID := features[0].(map[string]any)["properties"].(map[string]any)["uid"].(string)
	if firstUID != "AA" {
		t.Fatalf("expected first country boundary uid AA after sorting, got %q", firstUID)
	}
}

func TestGeoJSONFileWriterWriteIncludesZoneTypeOnAirspaceBorders(t *testing.T) {
	t.Parallel()

	dataset := domain.MapDataset{
		AirspaceBorders: []domain.MapBorderLine{{
			EdgeID:   "E_1",
			ZoneA:    "A",
			ZoneType: "ATZ",
			Line:     []domain.OFMXGeoPoint{{Lat: 49, Lon: 14}, {Lat: 49.1, Lon: 14.1}},
		}},
	}

	artifacts, err := (GeoJSONFileWriter{}).Write(context.Background(), dataset, t.TempDir())
	if err != nil {
		t.Fatalf("write geojson failed: %v", err)
	}

	b, err := os.ReadFile(artifacts.AirspaceBordersPath)
	if err != nil {
		t.Fatalf("read borders file failed: %v", err)
	}

	var fc map[string]any
	if err := json.Unmarshal(b, &fc); err != nil {
		t.Fatalf("unmarshal borders geojson failed: %v", err)
	}

	features := fc["features"].([]any)
	zoneType := features[0].(map[string]any)["properties"].(map[string]any)["zone_type"].(string)
	if zoneType != "ATZ" {
		t.Fatalf("expected border zone_type ATZ, got %q", zoneType)
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

func TestGeoJSONFileWriterWriteIncludesVocalicTypeOnPOI(t *testing.T) {
	t.Parallel()

	dataset := domain.MapDataset{
		PointsOfInterest: []domain.MapPOI{{ID: "AB", Kind: "VOR", Name: "Alpha Bravo", Type: "vocalic", Lat: 50.2, Lon: 14.4}},
	}

	artifacts, err := (GeoJSONFileWriter{}).Write(context.Background(), dataset, t.TempDir())
	if err != nil {
		t.Fatalf("write geojson failed: %v", err)
	}

	b, err := os.ReadFile(artifacts.PointsOfInterestPath)
	if err != nil {
		t.Fatalf("read poi file failed: %v", err)
	}

	var fc map[string]any
	if err := json.Unmarshal(b, &fc); err != nil {
		t.Fatalf("unmarshal poi geojson failed: %v", err)
	}

	features := fc["features"].([]any)
	typ := features[0].(map[string]any)["properties"].(map[string]any)["type"].(string)
	if typ != "vocalic" {
		t.Fatalf("expected poi type vocalic, got %q", typ)
	}
}
