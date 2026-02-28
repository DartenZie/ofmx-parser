package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/app"
)

const mapModeOFMXInput = `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ahp>
    <AhpUid><codeId>LKPR</codeId></AhpUid>
    <OrgUid><txtName>Authority</txtName></OrgUid>
    <txtName>Prague Airport</txtName>
    <codeType>AD</codeType>
    <geoLat>50.100556N</geoLat>
    <geoLong>014.262222E</geoLong>
    <valElev>380</valElev>
  </Ahp>
  <Ase>
    <AseUid><codeType>R</codeType><codeId>LKR1</codeId></AseUid>
    <txtName>Restricted Area</txtName>
    <codeClass>C</codeClass>
    <codeDistVerLower>SFC</codeDistVerLower>
    <valDistVerLower>0</valDistVerLower>
    <codeDistVerUpper>MSL</codeDistVerUpper>
    <valDistVerUpper>2450</valDistVerUpper>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>R</codeType><codeId>LKR1</codeId></AseUid></AbdUid>
    <Avx><codeType>GRC</codeType><geoLat>49.000000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>49.100000N</geoLat><geoLong>014.200000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>48.900000N</geoLat><geoLong>014.300000E</geoLong><codeDatum>WGE</codeDatum></Avx>
  </Abd>
</OFMX-Snapshot>`

func TestAppRunMapModeStrictFailsWhenTilemakerMissing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	pbfPath := filepath.Join(tmpDir, "base.osm.pbf")
	pmtilesPath := filepath.Join(tmpDir, "out.pmtiles")

	if err := os.WriteFile(inputPath, []byte(mapModeOFMXInput), 0o600); err != nil {
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

	if err := os.WriteFile(inputPath, []byte(mapModeOFMXInput), 0o600); err != nil {
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

	if err := os.WriteFile(inputPath, []byte(mapModeOFMXInput), 0o600); err != nil {
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

	if err := os.WriteFile(inputPath, []byte(mapModeOFMXInput), 0o600); err != nil {
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
