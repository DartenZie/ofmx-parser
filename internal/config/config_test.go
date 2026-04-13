package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseArgsAcceptsMapOnlyMode(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{
		"--input", "input.ofmx",
		"--pbf-input", "base.osm.pbf",
		"--pmtiles-output", "map.pmtiles",
	})
	if err != nil {
		t.Fatalf("expected valid map-only args, got: %v", err)
	}
}

func TestParseArgsAcceptsTerrainOnlyModeWithoutInput(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{
		"--terrain-source-dir", "copdem",
		"--terrain-aoi-bbox", "12.0,48.0,13.0,49.0",
		"--terrain-version", "COPDEM-30-2026-04",
		"--terrain-pmtiles-output", "terrain.pmtiles",
	})
	if err != nil {
		t.Fatalf("expected valid terrain-only args, got: %v", err)
	}
}

func TestParseArgsRejectsTerrainModeWithoutAOI(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{
		"--terrain-source-dir", "copdem",
		"--terrain-version", "COPDEM-30-2026-04",
		"--terrain-pmtiles-output", "terrain.pmtiles",
	})
	if err == nil {
		t.Fatal("expected error when terrain aoi bbox missing")
	}
}

func TestParseArgsRejectsInvalidTerrainTileSize(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{
		"--terrain-source-dir", "copdem",
		"--terrain-aoi-bbox", "12.0,48.0,13.0,49.0",
		"--terrain-version", "COPDEM-30-2026-04",
		"--terrain-pmtiles-output", "terrain.pmtiles",
		"--terrain-tile-size", "300",
	})
	if err == nil {
		t.Fatal("expected error for invalid terrain tile size")
	}
}

func TestParseBoundingBoxParsesValidInput(t *testing.T) {
	t.Parallel()

	bbox, err := ParseBoundingBox("12.0,48.0,13.0,49.0")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if bbox.MinLon != 12.0 || bbox.MinLat != 48.0 || bbox.MaxLon != 13.0 || bbox.MaxLat != 49.0 {
		t.Fatalf("unexpected bbox parsed: %+v", bbox)
	}
}

func TestParseArgsRejectsMapModeWithoutPBF(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{
		"--input", "input.ofmx",
		"--pmtiles-output", "map.pmtiles",
	})
	if err == nil {
		t.Fatal("expected error when --pbf-input missing in map mode")
	}
}

func TestParseArgsRejectsGeoJSONDebugWithoutPBF(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{
		"--input", "input.ofmx",
		"--pmtiles-output", "map.pmtiles",
		"--geojson-output-dir", "debug",
	})
	if err == nil {
		t.Fatal("expected error when --pbf-input missing with --geojson-output-dir")
	}
}

func TestParseArgsRejectsWhenNoOutputRequested(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{"--input", "input.ofmx"})
	if err == nil {
		t.Fatal("expected error when no --output or --pmtiles-output provided")
	}
}

func TestParseArgsRejectsNonPositiveArcChord(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{
		"--input", "input.ofmx",
		"--output", "out.xml",
		"--arc-max-chord-m", "0",
	})
	if err == nil {
		t.Fatal("expected error for non-positive --arc-max-chord-m")
	}
}

func TestLoadFileParsesAirspaceAllowedTypes(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "parser.yaml")

	content := []byte(`transform:
  airspace:
    allowed_types:
      - ctr
      - tma
      - CTR
      - "  r "
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	want := []string{"CTR", "TMA", "R"}
	if len(got.Transform.Airspace.AllowedTypes) != len(want) {
		t.Fatalf("expected %d allowed types, got %+v", len(want), got.Transform.Airspace.AllowedTypes)
	}
	for i := range want {
		if got.Transform.Airspace.AllowedTypes[i] != want[i] {
			t.Fatalf("unexpected allowed types order/content: got %+v want %+v", got.Transform.Airspace.AllowedTypes, want)
		}
	}

	if got.EffectiveAirspaceMaxAltitudeFL() != DefaultAirspaceMaxAltitudeFL {
		t.Fatalf("expected default max altitude FL %d, got %d", DefaultAirspaceMaxAltitudeFL, got.EffectiveAirspaceMaxAltitudeFL())
	}
}

func TestLoadFileRejectsInvalidYAML(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	if err := os.WriteFile(configPath, []byte("transform: ["), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadFile(configPath); err == nil {
		t.Fatal("expected parse error for invalid yaml")
	}
}

func TestLoadFileParsesAirspaceMaxAltitudeFL(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "parser.yaml")

	content := []byte(`transform:
  airspace:
    max_altitude_fl: 120
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if got.EffectiveAirspaceMaxAltitudeFL() != 120 {
		t.Fatalf("expected max altitude FL 120, got %d", got.EffectiveAirspaceMaxAltitudeFL())
	}
}

func TestLoadFileRejectsAirspaceMaxAltitudeBelowMinimum(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "parser.yaml")

	content := []byte(`transform:
  airspace:
    max_altitude_fl: 90
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadFile(configPath); err == nil {
		t.Fatal("expected config validation error for max_altitude_fl below minimum")
	}
}
