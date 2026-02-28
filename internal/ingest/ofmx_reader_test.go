package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileReaderReadParsesSnapshotMetaAndCounts(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "snapshot.ofmx")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ahp><AhpUid><codeId>A1</codeId></AhpUid><OrgUid><txtName>ORG</txtName></OrgUid><txtName>One</txtName><codeType>AD</codeType><geoLat>48.000000N</geoLat><geoLong>016.000000E</geoLong></Ahp>
  <Ahp><AhpUid><codeId>A2</codeId></AhpUid><OrgUid><txtName>ORG</txtName></OrgUid><txtName>Two</txtName><codeType>AD</codeType><geoLat>49.000000N</geoLat><geoLong>017.000000E</geoLong></Ahp>
  <Rwy><RwyUid><AhpUid><codeId>A1</codeId></AhpUid><txtDesig>09/27</txtDesig></RwyUid></Rwy>
</OFMX-Snapshot>`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	reader := FileReader{}
	doc, err := reader.Read(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("reader.Read failed: %v", err)
	}

	if doc.SnapshotMeta.Version != "0.1.0" {
		t.Fatalf("expected version 0.1.0, got %q", doc.SnapshotMeta.Version)
	}

	if doc.FeatureCounts["Ahp"] != 2 {
		t.Fatalf("expected Ahp count 2, got %d", doc.FeatureCounts["Ahp"])
	}

	if doc.FeatureCounts["Rwy"] != 1 {
		t.Fatalf("expected Rwy count 1, got %d", doc.FeatureCounts["Rwy"])
	}

	if len(doc.Airports) != 2 {
		t.Fatalf("expected 2 parsed airports, got %d", len(doc.Airports))
	}

	if doc.Airports[0].Lat == 0 || doc.Airports[0].Lon == 0 {
		t.Fatalf("expected airport coordinates to be parsed, got lat=%f lon=%f", doc.Airports[0].Lat, doc.Airports[0].Lon)
	}
}

func TestFileReaderReadFailsWhenRequiredSnapshotAttrMissing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "snapshot_missing_attr.ofmx")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
</OFMX-Snapshot>`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	reader := FileReader{}
	_, err := reader.Read(context.Background(), inputPath)
	if err == nil {
		t.Fatal("expected error for missing required version attribute")
	}
}

func TestFileReaderReadParsesAirspaceAndObstacle(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "snapshot_airspace_obstacle.ofmx")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
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
		t.Fatalf("write input: %v", err)
	}

	reader := FileReader{}
	doc, err := reader.Read(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("reader.Read failed: %v", err)
	}

	if len(doc.Airspaces) != 1 || doc.Airspaces[0].ID != "LKR1" {
		t.Fatalf("expected one parsed airspace LKR1, got %+v", doc.Airspaces)
	}

	if len(doc.AirspaceBorders) != 1 || len(doc.AirspaceBorders[0].Points) != 3 {
		t.Fatalf("expected one border with 3 points, got %+v", doc.AirspaceBorders)
	}

	if len(doc.Obstacles) != 1 || doc.Obstacles[0].ID != "OBS001" {
		t.Fatalf("expected one parsed obstacle OBS001, got %+v", doc.Obstacles)
	}
}

func TestParseCoordinateSupportsDecimalAndDMS(t *testing.T) {
	t.Parallel()

	latDec, err := parseCoordinate("49.123456N", true)
	if err != nil {
		t.Fatalf("decimal lat parse failed: %v", err)
	}
	if latDec < 49.12 || latDec > 49.13 {
		t.Fatalf("unexpected decimal lat value: %f", latDec)
	}

	lonDec, err := parseCoordinate("014.654321E", false)
	if err != nil {
		t.Fatalf("decimal lon parse failed: %v", err)
	}
	if lonDec < 14.65 || lonDec > 14.66 {
		t.Fatalf("unexpected decimal lon value: %f", lonDec)
	}

	latDMS, err := parseCoordinate("490600N", true)
	if err != nil {
		t.Fatalf("dms lat parse failed: %v", err)
	}
	if latDMS <= 49.09 || latDMS >= 49.11 {
		t.Fatalf("unexpected dms lat value: %f", latDMS)
	}
}

func TestParseCoordinateRejectsOutOfRange(t *testing.T) {
	t.Parallel()

	if _, err := parseCoordinate("991234N", true); err == nil {
		t.Fatal("expected out-of-range latitude error")
	}
}
