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

	input := fixtureInput(t, "report_basic.ofmx")

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
