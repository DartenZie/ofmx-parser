package output

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

func TestWriteGeoJSONDebugBundleCopiesArtifacts(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	dbgDir := filepath.Join(t.TempDir(), "geojson-debug")

	artifacts := domain.MapGeoJSONArtifacts{
		AirportsPath:          filepath.Join(srcDir, "aviation_airports.geojson"),
		ZonesPath:             filepath.Join(srcDir, "aviation_zones.geojson"),
		PointsOfInterestPath:  filepath.Join(srcDir, "aviation_poi.geojson"),
		AirspaceBordersPath:   filepath.Join(srcDir, "aviation_airspace_borders.geojson"),
		CountriesBoundaryPath: filepath.Join(srcDir, "countries_boundary.geojson"),
	}

	for _, path := range []string{
		artifacts.AirportsPath,
		artifacts.ZonesPath,
		artifacts.PointsOfInterestPath,
		artifacts.AirspaceBordersPath,
		artifacts.CountriesBoundaryPath,
	} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("write source artifact %q: %v", path, err)
		}
	}

	if err := WriteGeoJSONDebugBundle(context.Background(), artifacts, dbgDir); err != nil {
		t.Fatalf("write GeoJSON debug bundle: %v", err)
	}

	for _, name := range []string{
		"aviation_airports.geojson",
		"aviation_zones.geojson",
		"aviation_poi.geojson",
		"aviation_airspace_borders.geojson",
		"countries_boundary.geojson",
	} {
		if _, err := os.Stat(filepath.Join(dbgDir, name)); err != nil {
			t.Fatalf("expected copied debug artifact %q: %v", name, err)
		}
	}
}

func TestWriteGeoJSONDebugBundleNoopWhenDirEmpty(t *testing.T) {
	t.Parallel()

	if err := WriteGeoJSONDebugBundle(context.Background(), domain.MapGeoJSONArtifacts{}, ""); err != nil {
		t.Fatalf("expected no error for empty debug dir, got: %v", err)
	}
}
