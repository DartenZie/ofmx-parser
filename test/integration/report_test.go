package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/app"
)

func TestAppRunWritesParseReportJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.ofmx")
	outputPath := filepath.Join(tmpDir, "output.xml")
	reportPath := filepath.Join(tmpDir, "report.json")

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="2026-01-01T00:00:00Z" effective="2026-01-01T00:00:00Z">
  <Ahp>
    <AhpUid><codeId>LKTB</codeId></AhpUid>
    <OrgUid><txtName>Authority</txtName></OrgUid>
    <txtName>Brno Airport</txtName>
    <codeType>AD</codeType>
    <geoLat>49.150000N</geoLat>
    <geoLong>016.683333E</geoLong>
    <valElev>235</valElev>
  </Ahp>
  <Rwy></Rwy>
</OFMX-Snapshot>`

	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatalf("write input fixture: %v", err)
	}

	err := app.Run(context.Background(), []string{"--input", inputPath, "--output", outputPath, "--report", reportPath})
	if err != nil {
		t.Fatalf("app run failed: %v", err)
	}

	b, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}

	report := string(b)
	if !strings.Contains(report, `"total_features": 2`) {
		t.Fatalf("expected total_features=2 in report, got %q", report)
	}

	if !strings.Contains(report, `"Ahp": 1`) {
		t.Fatalf("expected Ahp count in report, got %q", report)
	}
}
