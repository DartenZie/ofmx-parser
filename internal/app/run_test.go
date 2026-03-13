package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/config"
	"github.com/DartenZie/ofmx-parser/internal/domain"
)

type countingReader struct {
	reads int
	doc   domain.OFMXDocument
}

func (r *countingReader) Read(_ context.Context, _ string) (domain.OFMXDocument, error) {
	r.reads++
	return r.doc, nil
}

func TestRunWithReaderDualModeReadsInputOnce(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	xmlPath := filepath.Join(tmpDir, "out.xml")
	pmtilesPath := filepath.Join(tmpDir, "out.pmtiles")
	pbfPath := filepath.Join(tmpDir, "base.osm.pbf")
	mapTempDir := filepath.Join(tmpDir, "map-runtime")
	fakeTilemakerPath := filepath.Join(tmpDir, "tilemaker-fake")

	if err := os.WriteFile(pbfPath, []byte("pbf"), 0o600); err != nil {
		t.Fatalf("write pbf fixture: %v", err)
	}

	script := "#!/usr/bin/env sh\n" +
		"set -eu\n" +
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
	if err := os.WriteFile(fakeTilemakerPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tilemaker: %v", err)
	}

	reader := &countingReader{doc: domain.OFMXDocument{
		SnapshotMeta: domain.OFMXSnapshotMetadata{
			Origin:    "unit-test",
			Regions:   "CZ",
			Created:   "2026-01-01T00:00:00Z",
			Effective: "2026-01-15T00:00:00Z",
		},
		FeatureCounts: map[string]int{},
	}}

	err := runWithReader(context.Background(), config.CLIConfig{
		InputPath:         "ignored.ofmx",
		OutputPath:        xmlPath,
		PBFInputPath:      pbfPath,
		PMTilesOutputPath: pmtilesPath,
		TilemakerBin:      fakeTilemakerPath,
		MapTempDir:        mapTempDir,
	}, config.FileConfig{}, reader)
	if err != nil {
		t.Fatalf("runWithReader failed: %v", err)
	}

	if reader.reads != 1 {
		t.Fatalf("expected single ingest read in dual mode, got %d", reader.reads)
	}
}

func TestResolveConfigPathPrefersExplicitPath(t *testing.T) {
	t.Parallel()

	got := resolveConfigPath("custom.yaml", func(string) bool { return false })
	if got != "custom.yaml" {
		t.Fatalf("expected explicit config path, got %q", got)
	}
}

func TestResolveConfigPathFindsDefaultInConfigsDir(t *testing.T) {
	t.Parallel()

	exists := func(path string) bool {
		return path == filepath.Join("configs", "parser.yaml")
	}

	got := resolveConfigPath("", exists)
	if got != filepath.Join("configs", "parser.yaml") {
		t.Fatalf("expected parser.yaml default, got %q", got)
	}
}

func TestResolveConfigPathFallsBackToExample(t *testing.T) {
	t.Parallel()

	exists := func(path string) bool {
		return path == filepath.Join("configs", "parser.example.yaml")
	}

	got := resolveConfigPath("", exists)
	if got != filepath.Join("configs", "parser.example.yaml") {
		t.Fatalf("expected parser.example.yaml fallback, got %q", got)
	}
}
