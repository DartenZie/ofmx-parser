package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/app"
)

func TestAppRunConfigReplacesAirspaceAllowlist(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	outputPath := filepath.Join(tmpDir, "output.xml")
	configPath := filepath.Join(tmpDir, "parser.yaml")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ase>
    <AseUid><codeType>CTR</codeType><codeId>LKCTR1</codeId></AseUid>
    <txtName>CTR Zone</txtName>
    <codeDistVerLower>SFC</codeDistVerLower>
    <valDistVerLower>0</valDistVerLower>
    <codeDistVerUpper>MSL</codeDistVerUpper>
    <valDistVerUpper>1000</valDistVerUpper>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>CTR</codeType><codeId>LKCTR1</codeId></AseUid></AbdUid>
    <Avx><codeType>GRC</codeType><geoLat>49.000000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>49.100000N</geoLat><geoLong>014.200000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>48.900000N</geoLat><geoLong>014.300000E</geoLong><codeDatum>WGE</codeDatum></Avx>
  </Abd>
  <Ase>
    <AseUid><codeType>TMA</codeType><codeId>LKTMA1</codeId></AseUid>
    <txtName>TMA Zone</txtName>
    <codeDistVerLower>SFC</codeDistVerLower>
    <valDistVerLower>0</valDistVerLower>
    <codeDistVerUpper>MSL</codeDistVerUpper>
    <valDistVerUpper>2000</valDistVerUpper>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>TMA</codeType><codeId>LKTMA1</codeId></AseUid></AbdUid>
    <Avx><codeType>GRC</codeType><geoLat>48.000000N</geoLat><geoLong>013.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>48.100000N</geoLat><geoLong>013.200000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>47.900000N</geoLat><geoLong>013.300000E</geoLong><codeDatum>WGE</codeDatum></Avx>
  </Abd>
</OFMX-Snapshot>`

	configBody := `transform:
  airspace:
    allowed_types:
      - TMA
`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input fixture: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	err := app.Run(context.Background(), []string{
		"--input", inputPath,
		"--output", outputPath,
		"--config", configPath,
	})
	if err != nil {
		t.Fatalf("app run failed: %v", err)
	}

	b, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated xml: %v", err)
	}

	xml := string(b)
	if strings.Contains(xml, `<Airspace id="LKCTR1"`) {
		t.Fatalf("expected CTR airspace to be filtered by config, got %q", xml)
	}
	if !strings.Contains(xml, `<Airspace id="LKTMA1"`) {
		t.Fatalf("expected TMA airspace to remain after configured filtering, got %q", xml)
	}
}

func TestAppRunConfigFiltersAirspacesByMaxAltitudeFL(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	outputPath := filepath.Join(tmpDir, "output.xml")
	configPath := filepath.Join(tmpDir, "parser.yaml")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ase>
    <AseUid><codeType>CTR</codeType><codeId>LKSFC</codeId></AseUid>
    <txtName>Surface Zone</txtName>
    <codeDistVerLower>SFC</codeDistVerLower>
    <valDistVerLower>0</valDistVerLower>
    <codeDistVerUpper>MSL</codeDistVerUpper>
    <valDistVerUpper>1000</valDistVerUpper>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>CTR</codeType><codeId>LKSFC</codeId></AseUid></AbdUid>
    <Avx><codeType>GRC</codeType><geoLat>49.000000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>49.100000N</geoLat><geoLong>014.200000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>48.900000N</geoLat><geoLong>014.300000E</geoLong><codeDatum>WGE</codeDatum></Avx>
  </Abd>
  <Ase>
    <AseUid><codeType>TMA</codeType><codeId>LKFL100</codeId></AseUid>
    <txtName>FL100 Zone</txtName>
    <valDistVerLower>100</valDistVerLower>
    <uomDistVerLower>FL</uomDistVerLower>
    <codeDistVerUpper>MSL</codeDistVerUpper>
    <valDistVerUpper>2000</valDistVerUpper>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>TMA</codeType><codeId>LKFL100</codeId></AseUid></AbdUid>
    <Avx><codeType>GRC</codeType><geoLat>48.000000N</geoLat><geoLong>013.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>48.100000N</geoLat><geoLong>013.200000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>47.900000N</geoLat><geoLong>013.300000E</geoLong><codeDatum>WGE</codeDatum></Avx>
  </Abd>
</OFMX-Snapshot>`

	configBody := `transform:
  airspace:
    max_altitude_fl: 95
`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input fixture: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	err := app.Run(context.Background(), []string{
		"--input", inputPath,
		"--output", outputPath,
		"--config", configPath,
	})
	if err != nil {
		t.Fatalf("app run failed: %v", err)
	}

	b, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated xml: %v", err)
	}

	xml := string(b)
	if !strings.Contains(xml, `<Airspace id="LKSFC"`) {
		t.Fatalf("expected SFC airspace below FL95 threshold, got %q", xml)
	}
	if strings.Contains(xml, `<Airspace id="LKFL100"`) {
		t.Fatalf("expected FL100 airspace to be filtered out, got %q", xml)
	}
}

func TestAppRunConfigOnlyProvidesRequiredPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	outputPath := filepath.Join(tmpDir, "output.xml")
	configPath := filepath.Join(tmpDir, "parser.yaml")

	if err := os.WriteFile(inputPath, []byte(sampleConfigDrivenOFMXInput()), 0o600); err != nil {
		t.Fatalf("write input fixture: %v", err)
	}

	configBody := "ofmx:\n" +
		"  input: " + inputPath + "\n" +
		"xml:\n" +
		"  output: " + outputPath + "\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	if err := app.Run(context.Background(), []string{"--config", configPath}); err != nil {
		t.Fatalf("app run failed: %v", err)
	}

	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected output from config-only run: %v", err)
	}
}

func TestAppRunCLIOverridesConfigOutputPath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	configOutputPath := filepath.Join(tmpDir, "config-output.xml")
	overrideOutputPath := filepath.Join(tmpDir, "override-output.xml")
	configPath := filepath.Join(tmpDir, "parser.yaml")

	if err := os.WriteFile(inputPath, []byte(sampleConfigDrivenOFMXInput()), 0o600); err != nil {
		t.Fatalf("write input fixture: %v", err)
	}

	configBody := "ofmx:\n" +
		"  input: " + inputPath + "\n" +
		"xml:\n" +
		"  output: " + configOutputPath + "\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	if err := app.Run(context.Background(), []string{"--config", configPath, "--output", overrideOutputPath}); err != nil {
		t.Fatalf("app run failed: %v", err)
	}

	if _, err := os.Stat(overrideOutputPath); err != nil {
		t.Fatalf("expected overridden output path to be written: %v", err)
	}
	if _, err := os.Stat(configOutputPath); err == nil {
		t.Fatalf("expected config output path to be ignored when --output is set")
	}
}

func sampleConfigDrivenOFMXInput() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ase>
    <AseUid><codeType>CTR</codeType><codeId>LKCTR1</codeId></AseUid>
    <txtName>CTR Zone</txtName>
    <codeDistVerLower>SFC</codeDistVerLower>
    <valDistVerLower>0</valDistVerLower>
    <codeDistVerUpper>MSL</codeDistVerUpper>
    <valDistVerUpper>1000</valDistVerUpper>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>CTR</codeType><codeId>LKCTR1</codeId></AseUid></AbdUid>
    <Avx><codeType>GRC</codeType><geoLat>49.000000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>49.100000N</geoLat><geoLong>014.200000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>48.900000N</geoLat><geoLong>014.300000E</geoLong><codeDatum>WGE</codeDatum></Avx>
  </Abd>
</OFMX-Snapshot>`
}
