package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func parseAndValidate(args []string) (ParsedArgs, error) {
	parsed, err := ParseArgs(args)
	if err != nil {
		return ParsedArgs{}, err
	}
	if err := parsed.Config.Validate(); err != nil {
		return ParsedArgs{}, err
	}
	return parsed, nil
}

func strPtr(v string) *string     { return &v }
func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestParseArgsAcceptsMapOnlyMode(t *testing.T) {
	t.Parallel()

	_, err := parseAndValidate([]string{
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

	_, err := parseAndValidate([]string{
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

	_, err := parseAndValidate([]string{
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

	_, err := parseAndValidate([]string{
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

	_, err := parseAndValidate([]string{
		"--input", "input.ofmx",
		"--pmtiles-output", "map.pmtiles",
	})
	if err == nil {
		t.Fatal("expected error when --pbf-input missing in map mode")
	}
}

func TestParseArgsRejectsGeoJSONDebugWithoutPBF(t *testing.T) {
	t.Parallel()

	_, err := parseAndValidate([]string{
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

	_, err := parseAndValidate([]string{"--input", "input.ofmx"})
	if err == nil {
		t.Fatal("expected error when no --output or --pmtiles-output provided")
	}
}

func TestParseArgsRejectsNonPositiveArcChord(t *testing.T) {
	t.Parallel()

	_, err := parseAndValidate([]string{
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

// ---- Elevation quantization and clip polygon flags --------------------------

func baseTerrainArgs() []string {
	return []string{
		"--terrain-source-dir", "copdem",
		"--terrain-aoi-bbox", "12.0,48.0,13.0,49.0",
		"--terrain-version", "COPDEM-30-2026-04",
		"--terrain-pmtiles-output", "terrain.pmtiles",
	}
}

func TestParseArgsAcceptsElevationQuantization(t *testing.T) {
	t.Parallel()

	args := append(baseTerrainArgs(), "--terrain-elevation-quantization-m", "1.0")
	cfg, err := parseAndValidate(args)
	if err != nil {
		t.Fatalf("expected valid args, got: %v", err)
	}
	if cfg.Config.TerrainElevationQuantizationM != 1.0 {
		t.Errorf("got TerrainElevationQuantizationM=%.1f, want 1.0", cfg.Config.TerrainElevationQuantizationM)
	}
}

func TestParseArgsRejectsNegativeElevationQuantization(t *testing.T) {
	t.Parallel()

	args := append(baseTerrainArgs(), "--terrain-elevation-quantization-m", "-0.5")
	if _, err := parseAndValidate(args); err == nil {
		t.Fatal("expected error for negative --terrain-elevation-quantization-m")
	}
}

func TestParseArgsAcceptsZeroElevationQuantization(t *testing.T) {
	t.Parallel()

	// Zero means disabled – should be accepted.
	args := append(baseTerrainArgs(), "--terrain-elevation-quantization-m", "0")
	cfg, err := parseAndValidate(args)
	if err != nil {
		t.Fatalf("expected valid args with quantization=0, got: %v", err)
	}
	if cfg.Config.TerrainElevationQuantizationM != 0 {
		t.Errorf("got TerrainElevationQuantizationM=%.1f, want 0", cfg.Config.TerrainElevationQuantizationM)
	}
}

func TestParseArgsAcceptsClipPolygon(t *testing.T) {
	t.Parallel()

	args := append(baseTerrainArgs(), "--terrain-clip-polygon", "/data/cz_border.geojson")
	cfg, err := parseAndValidate(args)
	if err != nil {
		t.Fatalf("expected valid args with clip polygon, got: %v", err)
	}
	if cfg.Config.TerrainClipPolygonPath != "/data/cz_border.geojson" {
		t.Errorf("got TerrainClipPolygonPath=%q, want /data/cz_border.geojson", cfg.Config.TerrainClipPolygonPath)
	}
}

func TestParseArgsDefaultsElevationQuantizationToZero(t *testing.T) {
	t.Parallel()

	cfg, err := parseAndValidate(baseTerrainArgs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Config.TerrainElevationQuantizationM != 0 {
		t.Errorf("expected default quantization=0, got %.2f", cfg.Config.TerrainElevationQuantizationM)
	}
}

func TestParseArgsDefaultsClipPolygonToEmpty(t *testing.T) {
	t.Parallel()

	cfg, err := parseAndValidate(baseTerrainArgs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Config.TerrainClipPolygonPath != "" {
		t.Errorf("expected default clip polygon empty, got %q", cfg.Config.TerrainClipPolygonPath)
	}
}

func TestParseArgsAcceptsClipCountryName(t *testing.T) {
	t.Parallel()

	args := append(baseTerrainArgs(), "--terrain-clip-country-name", "CZECHREPUBLIC")
	cfg, err := parseAndValidate(args)
	if err != nil {
		t.Fatalf("expected valid args with clip country name, got: %v", err)
	}
	if cfg.Config.TerrainClipPolygonCountryName != "CZECHREPUBLIC" {
		t.Errorf("got TerrainClipPolygonCountryName=%q, want CZECHREPUBLIC", cfg.Config.TerrainClipPolygonCountryName)
	}
}

func TestParseArgsDefaultsClipCountryNameToEmpty(t *testing.T) {
	t.Parallel()

	cfg, err := parseAndValidate(baseTerrainArgs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Config.TerrainClipPolygonCountryName != "" {
		t.Errorf("expected default clip country name empty, got %q", cfg.Config.TerrainClipPolygonCountryName)
	}
}

func TestLoadFileParsesGroupedRuntimeConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "parser.yaml")

	content := []byte(`ofmx:
  input: input.ofmx
  arc_max_chord_m: 600
xml:
  output: output.xml
  report: report.json
bundle:
  output: output.ofpkg
map:
  pbf_input: base.osm.pbf
  pmtiles_output: map.pmtiles
  geojson_output_dir: geojson
  temp_dir: map-runtime
  tilemaker:
    bin: /opt/bin/tilemaker
    config: custom.config.json
    process: custom.process.lua
terrain:
  source_dir: copdem
  source_checksums: checksums.txt
  aoi_bbox: 12.0,48.0,13.0,49.0
  version: COPDEM-30-2026-04
  pmtiles_output: terrain.pmtiles
  manifest_output: terrain.manifest.json
  build_report_output: build-report.json
  build_dir: terrain-build
  min_zoom: 6
  max_zoom: 9
  tile_size: 512
  encoding: terrarium
  vertical_datum: EGM96
  schema_version: 1.2.0
  nodata_fill_distance: 50
  nodata_fill_smoothing: 1
  seam_threshold: 4
  rmse_threshold_m: 10.5
  control_points: control.csv
  build_timestamp: 2026-01-01T00:00:00Z
  gdal2tiles_processes: 4
  elevation_quantization_m: 1.0
  clip_polygon: cz-border.geojson
  clip_country_name: CZECHREPUBLIC
  toolchain:
    gdalbuildvrt_bin: /opt/bin/gdalbuildvrt
    gdal_fillnodata_bin: /opt/bin/gdal_fillnodata.py
    gdalwarp_bin: /opt/bin/gdalwarp
    gdal_translate_bin: /opt/bin/gdal_translate
    gdaladdo_bin: /opt/bin/gdaladdo
    gdal_calc_bin: /opt/bin/gdal_calc.py
    gdal_merge_bin: /opt/bin/gdal_merge.py
    gdal2tiles_bin: /opt/bin/gdal2tiles.py
    gdaldem_bin: /opt/bin/gdaldem
    gdalinfo_bin: /opt/bin/gdalinfo
    gdallocationinfo_bin: /opt/bin/gdallocationinfo
    pmtiles_bin: /opt/bin/pmtiles
transform:
  airspace:
    max_altitude_fl: 120
    allowed_types:
      - ctr
      - tma
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if got.OFMX.InputPath == nil || *got.OFMX.InputPath != "input.ofmx" {
		t.Fatalf("unexpected ofmx.input: %+v", got.OFMX.InputPath)
	}
	if got.Map.Tilemaker.Bin == nil || *got.Map.Tilemaker.Bin != "/opt/bin/tilemaker" {
		t.Fatalf("unexpected map.tilemaker.bin: %+v", got.Map.Tilemaker.Bin)
	}
	if got.Bundle.OutputPath == nil || *got.Bundle.OutputPath != "output.ofpkg" {
		t.Fatalf("unexpected bundle.output: %+v", got.Bundle.OutputPath)
	}
	if got.Terrain.Toolchain.GDAL2TilesBin == nil || *got.Terrain.Toolchain.GDAL2TilesBin != "/opt/bin/gdal2tiles.py" {
		t.Fatalf("unexpected terrain.toolchain.gdal2tiles_bin: %+v", got.Terrain.Toolchain.GDAL2TilesBin)
	}
	if got.Terrain.ClipPolygonCountryName == nil || *got.Terrain.ClipPolygonCountryName != "CZECHREPUBLIC" {
		t.Fatalf("unexpected terrain.clip_country_name: %+v", got.Terrain.ClipPolygonCountryName)
	}
	if got.EffectiveAirspaceMaxAltitudeFL() != 120 {
		t.Fatalf("expected max altitude FL 120, got %d", got.EffectiveAirspaceMaxAltitudeFL())
	}
	wantAllowed := []string{"CTR", "TMA"}
	if !reflect.DeepEqual(got.Transform.Airspace.AllowedTypes, wantAllowed) {
		t.Fatalf("unexpected normalized allowed types: got %+v want %+v", got.Transform.Airspace.AllowedTypes, wantAllowed)
	}
}

func TestFileConfigApplyToRespectsExplicitFlags(t *testing.T) {
	t.Parallel()

	parsed, err := ParseArgs([]string{
		"--output", "cli.xml",
		"--bundle-output", "cli.ofpkg",
		"--terrain-gdal2tiles-processes", "8",
	})
	if err != nil {
		t.Fatalf("parse args failed: %v", err)
	}

	cfg := parsed.Config
	fileCfg := FileConfig{
		OFMX:   OFMXFileConfig{InputPath: strPtr("config.ofmx")},
		XML:    XMLFileConfig{OutputPath: strPtr("config.xml")},
		Bundle: BundleFileConfig{OutputPath: strPtr("config.ofpkg")},
		Terrain: TerrainFileConfig{
			Encoding:            strPtr("terrain-rgb"),
			GDAL2TilesProcesses: intPtr(4),
		},
	}

	fileCfg.ApplyTo(&cfg, parsed.ExplicitFlags)

	if cfg.InputPath != "config.ofmx" {
		t.Fatalf("expected file config to set input path, got %q", cfg.InputPath)
	}
	if cfg.OutputPath != "cli.xml" {
		t.Fatalf("expected explicit CLI output to win, got %q", cfg.OutputPath)
	}
	if cfg.TerrainEncoding != "terrain-rgb" {
		t.Fatalf("expected file config to set terrain encoding, got %q", cfg.TerrainEncoding)
	}
	if cfg.BundleOutputPath != "cli.ofpkg" {
		t.Fatalf("expected explicit CLI bundle output to win, got %q", cfg.BundleOutputPath)
	}
	if cfg.TerrainGDAL2TilesProcesses != 8 {
		t.Fatalf("expected explicit CLI gdal2tiles processes to win, got %d", cfg.TerrainGDAL2TilesProcesses)
	}
}

func TestExampleConfigMatchesCLIParsedDefaults(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	examplePath := filepath.Join(filepath.Dir(thisFile), "..", "..", "configs", "parser.example.yaml")

	fileCfg, err := LoadFile(examplePath)
	if err != nil {
		t.Fatalf("load example config failed: %v", err)
	}

	parsed, err := ParseArgs(nil)
	if err != nil {
		t.Fatalf("parse default args failed: %v", err)
	}

	got := parsed.Config
	fileCfg.ApplyTo(&got, parsed.ExplicitFlags)

	if !reflect.DeepEqual(got, parsed.Config) {
		t.Fatalf("example config drifted from CLI defaults:\n got: %+v\nwant: %+v", got, parsed.Config)
	}

	wantAllowed := []string{"ATZ", "CTR", "TMA", "D", "P", "PR", "R", "TRA", "TRA_GA", "TSA"}
	if !reflect.DeepEqual(fileCfg.Transform.Airspace.AllowedTypes, wantAllowed) {
		t.Fatalf("unexpected example allowed types: got %+v want %+v", fileCfg.Transform.Airspace.AllowedTypes, wantAllowed)
	}
	if fileCfg.EffectiveAirspaceMaxAltitudeFL() != DefaultAirspaceMaxAltitudeFL {
		t.Fatalf("expected example max altitude default %d, got %d", DefaultAirspaceMaxAltitudeFL, fileCfg.EffectiveAirspaceMaxAltitudeFL())
	}
}
