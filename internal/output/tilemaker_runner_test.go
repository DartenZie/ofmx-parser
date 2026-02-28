package output

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

func TestExecTilemakerRunnerRunFailsWhenBinaryMissing(t *testing.T) {
	t.Parallel()

	req := domain.MapExportRequest{
		PBFInputPath:      "input.osm.pbf",
		PMTilesOutputPath: filepath.Join(t.TempDir(), "out.pmtiles"),
		TilemakerBin:      "definitely-missing-tilemaker-binary",
	}

	err := (ExecTilemakerRunner{}).Run(context.Background(), req, domain.MapGeoJSONArtifacts{})
	if err == nil {
		t.Fatal("expected strict-fail error when tilemaker binary is missing")
	}

	if !strings.Contains(err.Error(), "strict-fail") {
		t.Fatalf("expected strict-fail context in error, got: %v", err)
	}
}

func TestExecTilemakerRunnerRunExecutesFakeBinaryAndProducesPMTiles(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	fakeBin := filepath.Join(tmp, "tilemaker")
	outputPath := filepath.Join(tmp, "out.pmtiles")
	pbfPath := filepath.Join(tmp, "input.osm.pbf")

	if err := os.WriteFile(pbfPath, []byte("pbf"), 0o644); err != nil {
		t.Fatalf("write pbf input: %v", err)
	}

	script := `#!/usr/bin/env sh
set -eu
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
if [ -z "$out" ]; then
  exit 2
fi
touch "$out"
`
	if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tilemaker: %v", err)
	}

	artifacts := domain.MapGeoJSONArtifacts{
		AirportsPath:         filepath.Join(tmp, "aviation_airports.geojson"),
		ZonesPath:            filepath.Join(tmp, "aviation_zones.geojson"),
		PointsOfInterestPath: filepath.Join(tmp, "aviation_poi.geojson"),
		AirspaceBordersPath:  filepath.Join(tmp, "aviation_airspace_borders.geojson"),
	}
	for _, path := range []string{artifacts.AirportsPath, artifacts.ZonesPath, artifacts.PointsOfInterestPath, artifacts.AirspaceBordersPath} {
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write artifact %q: %v", path, err)
		}
	}

	req := domain.MapExportRequest{
		PBFInputPath:      pbfPath,
		PMTilesOutputPath: outputPath,
		TilemakerBin:      fakeBin,
		TempDir:           tmp,
	}

	if err := (ExecTilemakerRunner{}).Run(context.Background(), req, artifacts); err != nil {
		t.Fatalf("runner failed: %v", err)
	}

	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected PMTiles output file: %v", err)
	}
}

func TestGenerateTilemakerRuntimeFilesContainsRequiredLayers(t *testing.T) {
	t.Parallel()

	artifacts := domain.MapGeoJSONArtifacts{
		AirportsPath:         "/tmp/aviation_airports.geojson",
		ZonesPath:            "/tmp/aviation_zones.geojson",
		PointsOfInterestPath: "/tmp/aviation_poi.geojson",
		AirspaceBordersPath:  "/tmp/aviation_airspace_borders.geojson",
	}

	configPath, processPath, err := generateTilemakerRuntimeFiles(t.TempDir(), artifacts)
	if err != nil {
		t.Fatalf("generate runtime files failed: %v", err)
	}

	cb, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	for _, token := range []string{"landuse-residential", "road_major_motorway", "place_label_other", "aviation_airspace_borders"} {
		if !strings.Contains(string(cb), token) {
			t.Fatalf("expected token %q in generated config", token)
		}
	}

	pb, err := os.ReadFile(processPath)
	if err != nil {
		t.Fatalf("read process lua: %v", err)
	}

	for _, token := range []string{"place_label_city", "place_label_other", "road_trunk_primary", "waterway-tunnel"} {
		if !strings.Contains(string(pb), token) {
			t.Fatalf("expected token %q in generated process lua", token)
		}
	}
}
