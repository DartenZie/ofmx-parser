package ingest

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
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

func TestFileReaderReadExpandsFrontierSegmentFromGbr(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "snapshot_frontier_expand.ofmx")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ase>
    <AseUid><codeType>R</codeType><codeId>LKFNT1</codeId></AseUid>
    <txtName>Frontier Zone</txtName>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>R</codeType><codeId>LKFNT1</codeId></AseUid></AbdUid>
    <Avx><codeType>GRC</codeType><geoLat>50.000000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx>
      <GbrUid mid="BORDER-1"><txtName>TEST_BORDER</txtName></GbrUid>
      <codeType>FNT</codeType>
      <geoLat>49.900000N</geoLat>
      <geoLong>014.200000E</geoLong>
      <codeDatum>WGE</codeDatum>
    </Avx>
    <Avx><codeType>GRC</codeType><geoLat>49.800000N</geoLat><geoLong>014.400000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>50.000000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
  </Abd>
  <Gbr>
    <GbrUid mid="BORDER-1"><txtName>TEST_BORDER</txtName></GbrUid>
    <codeType />
    <Gbv><codeType>GRC</codeType><geoLat>50.200000N</geoLat><geoLong>013.800000E</geoLong><codeDatum>WGE</codeDatum></Gbv>
    <Gbv><codeType>GRC</codeType><geoLat>50.000000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Gbv>
    <Gbv><codeType>GRC</codeType><geoLat>49.900000N</geoLat><geoLong>014.200000E</geoLong><codeDatum>WGE</codeDatum></Gbv>
    <Gbv><codeType>GRC</codeType><geoLat>49.800000N</geoLat><geoLong>014.400000E</geoLong><codeDatum>WGE</codeDatum></Gbv>
    <Gbv><codeType>GRC</codeType><geoLat>49.700000N</geoLat><geoLong>014.600000E</geoLong><codeDatum>WGE</codeDatum></Gbv>
  </Gbr>
</OFMX-Snapshot>`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	doc, err := FileReader{}.Read(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("reader.Read failed: %v", err)
	}

	if len(doc.AirspaceBorders) != 1 {
		t.Fatalf("expected one airspace border, got %d", len(doc.AirspaceBorders))
	}

	pts := doc.AirspaceBorders[0].Points
	if len(pts) < 4 {
		t.Fatalf("expected expanded border with >=4 points, got %d", len(pts))
	}

	containsIntermediate := false
	for _, p := range pts {
		if p.Lat == 49.9 && p.Lon == 14.2 {
			containsIntermediate = true
			break
		}
	}
	if !containsIntermediate {
		t.Fatalf("expected expanded frontier polyline point (49.9, 14.2), got %+v", pts)
	}

	if pts[0].Lat != pts[len(pts)-1].Lat || pts[0].Lon != pts[len(pts)-1].Lon {
		t.Fatalf("expected closed ring after expansion, first=%+v last=%+v", pts[0], pts[len(pts)-1])
	}
}

func TestFileReaderReadMissingFrontierGbrWarnsAndContinues(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "snapshot_frontier_missing_gbr.ofmx")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ase>
    <AseUid><codeType>R</codeType><codeId>LKFNT2</codeId></AseUid>
    <txtName>Missing Border Zone</txtName>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>R</codeType><codeId>LKFNT2</codeId></AseUid></AbdUid>
    <Avx><codeType>GRC</codeType><geoLat>49.000000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx>
      <GbrUid mid="MISSING-BORDER"><txtName>MISSING_BORDER</txtName></GbrUid>
      <codeType>FNT</codeType>
      <geoLat>49.100000N</geoLat>
      <geoLong>014.200000E</geoLong>
      <codeDatum>WGE</codeDatum>
    </Avx>
    <Avx><codeType>GRC</codeType><geoLat>48.900000N</geoLat><geoLong>014.300000E</geoLong><codeDatum>WGE</codeDatum></Avx>
  </Abd>
</OFMX-Snapshot>`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	warnings := make([]string, 0, 1)
	reader := FileReader{
		Warningf: func(format string, args ...any) {
			warnings = append(warnings, fmt.Sprintf(format, args...))
		},
	}

	doc, err := reader.Read(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("reader.Read failed: %v", err)
	}

	if len(doc.AirspaceBorders) != 1 || len(doc.AirspaceBorders[0].Points) < 3 {
		t.Fatalf("expected fallback border points, got %+v", doc.AirspaceBorders)
	}

	if len(warnings) == 0 {
		t.Fatal("expected missing border warning")
	}
	if !strings.Contains(warnings[0], "missing_border_uid=\"MISSING-BORDER\"") {
		t.Fatalf("expected warning to include missing border UID, got %q", warnings[0])
	}
}

func TestFileReaderReadExpandsClockwiseArcVertices(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "snapshot_arc_expand.ofmx")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ase>
    <AseUid><codeType>R</codeType><codeId>LKARC1</codeId></AseUid>
    <txtName>Arc Zone</txtName>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>R</codeType><codeId>LKARC1</codeId></AseUid></AbdUid>
    <Avx>
      <codeType>CWA</codeType>
      <geoLat>50.100000N</geoLat>
      <geoLong>014.000000E</geoLong>
      <codeDatum>WGE</codeDatum>
      <geoLatArc>50.000000N</geoLatArc>
      <geoLongArc>014.000000E</geoLongArc>
    </Avx>
    <Avx><codeType>GRC</codeType><geoLat>50.000000N</geoLat><geoLong>014.100000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>49.900000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
    <Avx><codeType>GRC</codeType><geoLat>50.100000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum></Avx>
  </Abd>
</OFMX-Snapshot>`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	doc, err := FileReader{}.Read(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("reader.Read failed: %v", err)
	}

	if len(doc.AirspaceBorders) != 1 {
		t.Fatalf("expected one border, got %d", len(doc.AirspaceBorders))
	}

	pts := doc.AirspaceBorders[0].Points
	if len(pts) <= 4 {
		t.Fatalf("expected arc densification to add points, got %d", len(pts))
	}

	midFound := false
	for _, p := range pts {
		if p.Lat > 50.02 && p.Lat < 50.09 && p.Lon > 14.02 && p.Lon < 14.09 {
			midFound = true
			break
		}
	}
	if !midFound {
		t.Fatalf("expected intermediate arc points between start and end, got %+v", pts)
	}
}

func TestFileReaderReadArcMaxChordControlsPointCount(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "snapshot_arc_chord.ofmx")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ase>
    <AseUid><codeType>R</codeType><codeId>LKARC2</codeId></AseUid>
    <txtName>Arc Chord Zone</txtName>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>R</codeType><codeId>LKARC2</codeId></AseUid></AbdUid>
    <Avx>
      <codeType>CWA</codeType>
      <geoLat>50.100000N</geoLat>
      <geoLong>014.000000E</geoLong>
      <codeDatum>WGE</codeDatum>
      <geoLatArc>50.000000N</geoLatArc>
      <geoLongArc>014.000000E</geoLongArc>
    </Avx>
    <Avx><codeType>CWA</codeType><geoLat>50.000000N</geoLat><geoLong>014.100000E</geoLong><codeDatum>WGE</codeDatum><geoLatArc>50.000000N</geoLatArc><geoLongArc>014.000000E</geoLongArc></Avx>
    <Avx><codeType>CWA</codeType><geoLat>49.900000N</geoLat><geoLong>014.000000E</geoLong><codeDatum>WGE</codeDatum><geoLatArc>50.000000N</geoLatArc><geoLongArc>014.000000E</geoLongArc></Avx>
    <Avx><codeType>CWA</codeType><geoLat>50.000000N</geoLat><geoLong>013.900000E</geoLong><codeDatum>WGE</codeDatum><geoLatArc>50.000000N</geoLatArc><geoLongArc>014.000000E</geoLongArc></Avx>
  </Abd>
</OFMX-Snapshot>`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	dense, err := FileReader{ArcMaxChordLengthMeters: 300}.Read(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("dense reader failed: %v", err)
	}

	sparse, err := FileReader{ArcMaxChordLengthMeters: 3000}.Read(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("sparse reader failed: %v", err)
	}

	denseCount := len(dense.AirspaceBorders[0].Points)
	sparseCount := len(sparse.AirspaceBorders[0].Points)
	if denseCount <= sparseCount {
		t.Fatalf("expected denser arc to produce more points: dense=%d sparse=%d", denseCount, sparseCount)
	}
}

func TestFileReaderReadMapsCircleBorder(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "snapshot_circle.ofmx")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ase>
    <AseUid><codeType>R</codeType><codeId>LKCIR1</codeId></AseUid>
    <txtName>Circle Zone</txtName>
  </Ase>
  <Abd>
    <AbdUid><AseUid><codeType>R</codeType><codeId>LKCIR1</codeId></AseUid></AbdUid>
    <Circle>
      <geoLatCen>50.000000N</geoLatCen>
      <geoLongCen>014.000000E</geoLongCen>
      <codeDatum>WGE</codeDatum>
      <valRadius>2</valRadius>
      <uomRadius>NM</uomRadius>
    </Circle>
  </Abd>
</OFMX-Snapshot>`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	doc, err := FileReader{}.Read(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("reader.Read failed: %v", err)
	}

	if len(doc.AirspaceBorders) != 1 {
		t.Fatalf("expected one border, got %d", len(doc.AirspaceBorders))
	}

	pts := doc.AirspaceBorders[0].Points
	if len(pts) < 9 {
		t.Fatalf("expected circle to be densified to at least 9 points, got %d", len(pts))
	}

	if math.Abs(pts[0].Lat-pts[len(pts)-1].Lat) > 1e-7 || math.Abs(pts[0].Lon-pts[len(pts)-1].Lon) > 1e-7 {
		t.Fatalf("expected closed circle ring, first=%+v last=%+v", pts[0], pts[len(pts)-1])
	}
}
