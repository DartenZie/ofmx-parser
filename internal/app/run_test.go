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

func TestRunWithReaderTerrainOnlyDoesNotReadOFMX(t *testing.T) {
	t.Parallel()

	reader := &countingReader{}
	err := runWithReader(context.Background(), config.CLIConfig{
		TerrainSourceDir:         "copdem",
		TerrainAOIBBox:           "12.0,48.0,13.0,49.0",
		TerrainVersion:           "COPDEM-30-2026-04",
		TerrainPMTilesOutputPath: filepath.Join(t.TempDir(), "terrain.pmtiles"),
		TerrainGDALBuildVRTBin:   "missing-binary",
	}, config.FileConfig{}, reader)

	if err == nil {
		t.Fatal("expected terrain run to fail due to missing binary")
	}
	if reader.reads != 0 {
		t.Fatalf("expected no OFMX reads in terrain-only mode, got %d", reader.reads)
	}
}

func TestRunWithReaderBundleModeProducesArchive(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "out.ofpkg")

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
		InputPath:        "ignored.ofmx",
		OutputPath:       filepath.Join(tmpDir, "out.xml"),
		BundleOutputPath: bundlePath,
	}, config.FileConfig{}, reader)
	if err != nil {
		t.Fatalf("runWithReader failed: %v", err)
	}

	st, err := os.Stat(bundlePath)
	if err != nil {
		t.Fatalf("bundle file not created: %v", err)
	}
	if st.Size() == 0 {
		t.Fatal("bundle file is empty")
	}

	// Individual XML file should not remain on disk (written to temp staging dir).
	if _, err := os.Stat(filepath.Join(tmpDir, "out.xml")); err == nil {
		t.Fatal("expected individual XML file to not exist when bundling; it should be in staging dir only")
	}
}

func TestBuildBundleRequestCollectsEntries(t *testing.T) {
	t.Parallel()

	cfg := config.CLIConfig{
		OutputPath:                   "/tmp/nav.xml",
		ReportPath:                   "/tmp/report.json",
		PMTilesOutputPath:            "/tmp/map.pmtiles",
		TerrainPMTilesOutputPath:     "/tmp/terrain.pmtiles",
		TerrainManifestOutputPath:    "/tmp/terrain.manifest.json",
		TerrainBuildReportOutputPath: "/tmp/terrain.build-report.json",
		BundleOutputPath:             "/tmp/out.ofpkg",
	}
	outDoc := domain.OutputDocument{Cycle: "20260115", Region: "CZ"}

	req := buildBundleRequest(cfg, outDoc)

	if req.OutputPath != "/tmp/out.ofpkg" {
		t.Errorf("expected output path /tmp/out.ofpkg, got %q", req.OutputPath)
	}
	if len(req.Entries) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(req.Entries))
	}

	roles := make(map[string]string, len(req.Entries))
	for _, e := range req.Entries {
		roles[e.Role] = e.ArchivePath
	}

	expectedRoles := map[string]string{
		"navsnapshot":          "payload/navsnapshot.xml",
		"parse-report":         "reports/report.json",
		"map-pmtiles":          "payload/map.pmtiles",
		"terrain-pmtiles":      "payload/terrain.pmtiles",
		"terrain-manifest":     "reports/terrain.manifest.json",
		"terrain-build-report": "reports/terrain.build-report.json",
	}

	for role, expectedPath := range expectedRoles {
		if got, ok := roles[role]; !ok {
			t.Errorf("missing entry with role %q", role)
		} else if got != expectedPath {
			t.Errorf("role %q: expected archive path %q, got %q", role, expectedPath, got)
		}
	}

	if req.Metadata.Cycle != "20260115" {
		t.Errorf("expected cycle 20260115, got %q", req.Metadata.Cycle)
	}
	if req.Metadata.Region != "CZ" {
		t.Errorf("expected region CZ, got %q", req.Metadata.Region)
	}
}
