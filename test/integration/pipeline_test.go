package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/ingest"
	outputpkg "github.com/DartenZie/ofmx-parser/internal/output"
	"github.com/DartenZie/ofmx-parser/internal/pipeline"
	"github.com/DartenZie/ofmx-parser/internal/transform"
)

func TestPipelineExecuteWritesXML(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	outputPath := filepath.Join(tmpDir, "output.xml")

	input := `<?xml version="1.0" encoding="UTF-8"?>
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
  <Rwy>
    <RwyUid>
      <AhpUid><codeId>LKPR</codeId></AhpUid>
      <txtDesig>06/24</txtDesig>
    </RwyUid>
    <valLen>3715</valLen>
    <valWid>45</valWid>
  </Rwy>
  <Rdn>
    <RdnUid>
      <RwyUid>
        <AhpUid><codeId>LKPR</codeId></AhpUid>
        <txtDesig>06/24</txtDesig>
      </RwyUid>
      <txtDesig>06</txtDesig>
    </RdnUid>
    <valTrueBrg>58</valTrueBrg>
  </Rdn>
  <Vor>
    <VorUid>
      <codeId>VLM</codeId>
      <geoLat>50.116667N</geoLat>
      <geoLong>014.500000E</geoLong>
    </VorUid>
    <OrgUid><txtName>Authority</txtName></OrgUid>
    <txtName>VLM VOR</txtName>
    <codeType>VOR</codeType>
    <valFreq>113.6</valFreq>
    <uomFreq>MHZ</uomFreq>
    <codeTypeNorth>TRUE</codeTypeNorth>
    <codeDatum>WGE</codeDatum>
  </Vor>
  <Ase>
    <AseUid>
      <codeType>R</codeType>
      <codeId>LKR1</codeId>
    </AseUid>
    <txtName>Restricted Area</txtName>
    <codeClass>C</codeClass>
    <codeDistVerLower>SFC</codeDistVerLower>
    <valDistVerLower>0</valDistVerLower>
    <codeDistVerUpper>MSL</codeDistVerUpper>
    <valDistVerUpper>2450</valDistVerUpper>
  </Ase>
  <Abd>
    <AbdUid>
      <AseUid>
        <codeType>R</codeType>
        <codeId>LKR1</codeId>
      </AseUid>
    </AbdUid>
    <Avx><codeType>GRC</codeType><geoLat>49.000000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>49.100000N</geoLat><geoLong>014.200000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>48.900000N</geoLat><geoLong>014.300000E</geoLong><codeDatum>WGE</codeDatum></Avx>
  </Abd>
  <Obs>
    <ObsUid mid="OBS001">
      <OgrUid><txtName>GROUP-A</txtName></OgrUid>
      <geoLat>49.200000N</geoLat>
      <geoLong>014.300000E</geoLong>
    </ObsUid>
    <txtName>Mast 1</txtName>
    <codeType>TOWER</codeType>
    <codeDatum>WGE</codeDatum>
    <valElev>300</valElev>
    <valHgt>120</valHgt>
    <uomDistVer>M</uomDistVer>
  </Obs>
</OFMX-Snapshot>`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input fixture: %v", err)
	}

	runner := pipeline.New(ingest.FileReader{}, transform.DefaultMapper{}, outputpkg.XMLFileWriter{})

	if _, err := runner.Execute(context.Background(), inputPath, outputPath); err != nil {
		t.Fatalf("pipeline execute failed: %v", err)
	}

	b, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated xml: %v", err)
	}

	xml := string(b)
	if !strings.Contains(xml, "<NavSnapshot") {
		t.Fatalf("expected NavSnapshot root, got %q", xml)
	}

	if !strings.Contains(xml, "<Airport id=\"LKPR\"") {
		t.Fatalf("expected airport mapping entry, got %q", xml)
	}

	if !strings.Contains(xml, "<Navaid id=\"VLM\"") {
		t.Fatalf("expected navaid mapping entry, got %q", xml)
	}

	if !strings.Contains(xml, "<Airspace id=\"LKR1\"") {
		t.Fatalf("expected airspace mapping entry, got %q", xml)
	}

	if !strings.Contains(xml, "<Obstacle id=\"OBS001\"") {
		t.Fatalf("expected obstacle mapping entry, got %q", xml)
	}
}
