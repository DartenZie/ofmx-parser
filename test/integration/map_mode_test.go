package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/app"
)

func TestAppRunMapModeStrictFailsWhenTilemakerMissing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	pbfPath := filepath.Join(tmpDir, "base.osm.pbf")
	pmtilesPath := filepath.Join(tmpDir, "out.pmtiles")

	if err := os.WriteFile(inputPath, []byte(fixtureInput(t, "map_mode_basic.ofmx")), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if err := os.WriteFile(pbfPath, []byte("pbf"), 0o600); err != nil {
		t.Fatalf("write pbf: %v", err)
	}

	err := app.Run(context.Background(), []string{
		"--input", inputPath,
		"--pbf-input", pbfPath,
		"--pmtiles-output", pmtilesPath,
		"--tilemaker-bin", "definitely-missing-tilemaker-binary",
	})
	if err == nil {
		t.Fatal("expected strict-fail error when tilemaker is missing")
	}

	if !strings.Contains(err.Error(), "strict-fail") {
		t.Fatalf("expected strict-fail context in error, got %v", err)
	}
}

func TestAppRunMapModeGeneratesPMTilesAndRuntimeFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	pbfPath := filepath.Join(tmpDir, "base.osm.pbf")
	pmtilesPath := filepath.Join(tmpDir, "out.pmtiles")
	mapTempDir := filepath.Join(tmpDir, "map-runtime")
	argsLogPath := filepath.Join(tmpDir, "tilemaker.args.log")
	fakeTilemakerPath := filepath.Join(tmpDir, "tilemaker-fake")

	if err := os.WriteFile(inputPath, []byte(fixtureInput(t, "map_mode_basic.ofmx")), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if err := os.WriteFile(pbfPath, []byte("pbf"), 0o600); err != nil {
		t.Fatalf("write pbf: %v", err)
	}

	if err := os.WriteFile(fakeTilemakerPath, []byte(fakeTilemakerScript(argsLogPath)), 0o755); err != nil {
		t.Fatalf("write fake tilemaker: %v", err)
	}

	err := app.Run(context.Background(), []string{
		"--input", inputPath,
		"--pbf-input", pbfPath,
		"--pmtiles-output", pmtilesPath,
		"--tilemaker-bin", fakeTilemakerPath,
		"--map-temp-dir", mapTempDir,
	})
	if err != nil {
		t.Fatalf("map-mode run failed: %v", err)
	}

	for _, path := range []string{
		pmtilesPath,
		filepath.Join(mapTempDir, "aviation_airports.geojson"),
		filepath.Join(mapTempDir, "aviation_zones.geojson"),
		filepath.Join(mapTempDir, "aviation_poi.geojson"),
		filepath.Join(mapTempDir, "aviation_airspace_borders.geojson"),
		filepath.Join(mapTempDir, "tilemaker.generated.config.json"),
		filepath.Join(mapTempDir, "tilemaker.generated.process.lua"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %q to exist: %v", path, err)
		}
	}

	logBytes, err := os.ReadFile(argsLogPath)
	if err != nil {
		t.Fatalf("read tilemaker args log: %v", err)
	}
	log := string(logBytes)
	for _, token := range []string{"--input", pbfPath, "--output", pmtilesPath, "--config", "--process"} {
		if !strings.Contains(log, token) {
			t.Fatalf("expected token %q in tilemaker args log, got: %s", token, log)
		}
	}
}

func TestAppRunDualModeGeneratesXMLAndPMTiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	pbfPath := filepath.Join(tmpDir, "base.osm.pbf")
	xmlPath := filepath.Join(tmpDir, "out.xml")
	pmtilesPath := filepath.Join(tmpDir, "out.pmtiles")
	argsLogPath := filepath.Join(tmpDir, "tilemaker.args.log")
	fakeTilemakerPath := filepath.Join(tmpDir, "tilemaker-fake")

	if err := os.WriteFile(inputPath, []byte(fixtureInput(t, "map_mode_basic.ofmx")), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if err := os.WriteFile(pbfPath, []byte("pbf"), 0o600); err != nil {
		t.Fatalf("write pbf: %v", err)
	}
	if err := os.WriteFile(fakeTilemakerPath, []byte(fakeTilemakerScript(argsLogPath)), 0o755); err != nil {
		t.Fatalf("write fake tilemaker: %v", err)
	}

	err := app.Run(context.Background(), []string{
		"--input", inputPath,
		"--output", xmlPath,
		"--pbf-input", pbfPath,
		"--pmtiles-output", pmtilesPath,
		"--tilemaker-bin", fakeTilemakerPath,
	})
	if err != nil {
		t.Fatalf("dual-mode run failed: %v", err)
	}

	for _, path := range []string{xmlPath, pmtilesPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected output %q to exist: %v", path, err)
		}
	}
}

func TestAppRunMapModeRespectsCustomTilemakerConfigAndProcess(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	pbfPath := filepath.Join(tmpDir, "base.osm.pbf")
	pmtilesPath := filepath.Join(tmpDir, "out.pmtiles")
	argsLogPath := filepath.Join(tmpDir, "tilemaker.args.log")
	fakeTilemakerPath := filepath.Join(tmpDir, "tilemaker-fake")
	customConfigPath := filepath.Join(tmpDir, "custom.config.json")
	customProcessPath := filepath.Join(tmpDir, "custom.process.lua")

	if err := os.WriteFile(inputPath, []byte(fixtureInput(t, "map_mode_basic.ofmx")), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if err := os.WriteFile(pbfPath, []byte("pbf"), 0o600); err != nil {
		t.Fatalf("write pbf: %v", err)
	}
	if err := os.WriteFile(fakeTilemakerPath, []byte(fakeTilemakerScript(argsLogPath)), 0o755); err != nil {
		t.Fatalf("write fake tilemaker: %v", err)
	}
	if err := os.WriteFile(customConfigPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write custom config: %v", err)
	}
	if err := os.WriteFile(customProcessPath, []byte("node_keys = {}\nway_keys = {}\n"), 0o644); err != nil {
		t.Fatalf("write custom process: %v", err)
	}

	err := app.Run(context.Background(), []string{
		"--input", inputPath,
		"--pbf-input", pbfPath,
		"--pmtiles-output", pmtilesPath,
		"--tilemaker-bin", fakeTilemakerPath,
		"--tilemaker-config", customConfigPath,
		"--tilemaker-process", customProcessPath,
	})
	if err != nil {
		t.Fatalf("custom-config map-mode run failed: %v", err)
	}

	logBytes, err := os.ReadFile(argsLogPath)
	if err != nil {
		t.Fatalf("read tilemaker args log: %v", err)
	}
	log := string(logBytes)

	if !strings.Contains(log, customConfigPath) {
		t.Fatalf("expected custom config path in tilemaker args: %s", log)
	}
	if !strings.Contains(log, customProcessPath) {
		t.Fatalf("expected custom process path in tilemaker args: %s", log)
	}
}

func TestAppRunMapModeAggregatesAllAbdBordersPerAirspace(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	pbfPath := filepath.Join(tmpDir, "base.osm.pbf")
	pmtilesPath := filepath.Join(tmpDir, "out.pmtiles")
	mapTempDir := filepath.Join(tmpDir, "map-runtime")
	argsLogPath := filepath.Join(tmpDir, "tilemaker.args.log")
	fakeTilemakerPath := filepath.Join(tmpDir, "tilemaker-fake")

	if err := os.WriteFile(inputPath, []byte(fixtureInput(t, "map_multi_abd.ofmx")), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if err := os.WriteFile(pbfPath, []byte("pbf"), 0o600); err != nil {
		t.Fatalf("write pbf: %v", err)
	}
	if err := os.WriteFile(fakeTilemakerPath, []byte(fakeTilemakerScript(argsLogPath)), 0o755); err != nil {
		t.Fatalf("write fake tilemaker: %v", err)
	}

	err := app.Run(context.Background(), []string{
		"--input", inputPath,
		"--pbf-input", pbfPath,
		"--pmtiles-output", pmtilesPath,
		"--tilemaker-bin", fakeTilemakerPath,
		"--map-temp-dir", mapTempDir,
	})
	if err != nil {
		t.Fatalf("map-mode run failed: %v", err)
	}

	zonesPath := filepath.Join(mapTempDir, "aviation_zones.geojson")
	b, err := os.ReadFile(zonesPath)
	if err != nil {
		t.Fatalf("read zones geojson: %v", err)
	}

	var fc map[string]any
	if err := json.Unmarshal(b, &fc); err != nil {
		t.Fatalf("unmarshal zones geojson: %v", err)
	}

	features := fc["features"].([]any)
	if len(features) != 1 {
		t.Fatalf("expected one zone feature, got %d", len(features))
	}

	geometry := features[0].(map[string]any)["geometry"].(map[string]any)
	coordinates := geometry["coordinates"].([]any)
	ring := coordinates[0].([]any)
	if len(ring) != 4 {
		t.Fatalf("expected single-ring polygon with 3 points plus closure, got %d", len(ring))
	}
}

func fakeTilemakerScript(argsLogPath string) string {
	return "#!/usr/bin/env sh\n" +
		"set -eu\n" +
		"log=\"" + argsLogPath + "\"\n" +
		": > \"$log\"\n" +
		"for arg in \"$@\"; do\n" +
		"  printf '%s\\n' \"$arg\" >> \"$log\"\n" +
		"done\n" +
		"out=\"\"\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--output\" ]; then\n" +
		"    out=\"$2\"\n" +
		"    shift 2\n" +
		"    continue\n" +
		"  fi\n" +
		"  shift\n" +
		"done\n" +
		"touch \"$out\"\n"
}
