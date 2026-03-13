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

	input := fixtureInput(t, "pipeline_basic.ofmx")

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
