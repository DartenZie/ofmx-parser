package output

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

func TestWriteFileAtomicReplacesExistingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "out.txt")
	if err := writeFileAtomic(context.Background(), path, []byte("first"), 0o644); err != nil {
		t.Fatalf("first atomic write failed: %v", err)
	}
	if err := writeFileAtomic(context.Background(), path, []byte("second"), 0o644); err != nil {
		t.Fatalf("second atomic write failed: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output file failed: %v", err)
	}
	if string(b) != "second" {
		t.Fatalf("expected latest content, got %q", string(b))
	}
}

func TestWriteFileAtomicCleansTempOnRenameFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	targetDir := filepath.Join(dir, "target")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}

	err := writeFileAtomic(context.Background(), targetDir, []byte("x"), 0o644)
	if err == nil {
		t.Fatal("expected atomic write failure when target path is a directory")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temp dir failed: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".ofmx-parser-") {
			t.Fatalf("expected temporary file cleanup, found leftover %q", entry.Name())
		}
	}
}

func TestWritersRespectCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := (XMLFileWriter{}).Write(ctx, sampleOutputDocument(), filepath.Join(t.TempDir(), "out.xml"))
	if err == nil {
		t.Fatal("expected XML writer cancellation error")
	}

	err = (JSONReportWriter{}).Write(ctx, sampleParseReport(), filepath.Join(t.TempDir(), "report.json"))
	if err == nil {
		t.Fatal("expected report writer cancellation error")
	}

	_, err = (GeoJSONFileWriter{}).Write(ctx, sampleMapDataset(), t.TempDir())
	if err == nil {
		t.Fatal("expected geojson writer cancellation error")
	}
}

func sampleOutputDocument() domain.OutputDocument {
	return domain.OutputDocument{
		Cycle:       "20260115",
		Region:      "CZ",
		GeneratedAt: "2026-01-15T00:00:00Z",
		Schema:      "output.xsd",
	}
}

func sampleParseReport() domain.ParseReport {
	return domain.ParseReport{FeatureCounts: map[string]int{"Ahp": 1}}
}

func sampleMapDataset() domain.MapDataset {
	return domain.MapDataset{}
}
