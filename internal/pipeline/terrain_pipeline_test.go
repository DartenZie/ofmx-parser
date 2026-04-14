package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

type fakeDEMIngestor struct {
	inventory domain.DEMSourceInventory
	err       error
}

func (f fakeDEMIngestor) Ingest(_ context.Context, _, _ string, _ domain.BoundingBox) (domain.DEMSourceInventory, error) {
	if f.err != nil {
		return domain.DEMSourceInventory{}, f.err
	}
	return f.inventory, nil
}

type fakeTerrainPlanner struct {
	plan domain.TerrainBuildPlan
	err  error
}

func (f fakeTerrainPlanner) Plan(_ context.Context, _ domain.TerrainExportRequest, _ domain.DEMSourceInventory) (domain.TerrainBuildPlan, error) {
	if f.err != nil {
		return domain.TerrainBuildPlan{}, f.err
	}
	return f.plan, nil
}

type fakeTerrainRunner struct {
	artifacts domain.TerrainBuildArtifacts
	err       error
}

func (f fakeTerrainRunner) Run(_ context.Context, _ domain.TerrainExportRequest, _ domain.TerrainBuildPlan, _ domain.DEMSourceInventory) (domain.TerrainBuildArtifacts, error) {
	if f.err != nil {
		return domain.TerrainBuildArtifacts{}, f.err
	}
	return f.artifacts, nil
}

type fakeTerrainMetaWriter struct {
	manifestPath string
	reportPath   string
	err          error
}

func (f *fakeTerrainMetaWriter) WriteManifest(_ context.Context, path string, _ domain.TerrainManifest) error {
	f.manifestPath = path
	return f.err
}

func (f *fakeTerrainMetaWriter) WriteBuildReport(_ context.Context, path string, _ domain.TerrainBuildReport) error {
	f.reportPath = path
	return f.err
}

type fakeTerrainValidator struct {
	result domain.TerrainValidationResult
	err    error
}

func (f fakeTerrainValidator) Validate(_ context.Context, _ domain.TerrainExportRequest, _ domain.TerrainBuildArtifacts, _ domain.TerrainManifest) (domain.TerrainValidationResult, error) {
	if f.err != nil {
		return domain.TerrainValidationResult{}, f.err
	}
	return f.result, nil
}

func TestTerrainServiceExecuteRunsAllStages(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	pmtilesPath := filepath.Join(tmp, "terrain.pmtiles")
	if err := os.WriteFile(pmtilesPath, []byte("pmtiles"), 0o600); err != nil {
		t.Fatalf("write pmtiles fixture: %v", err)
	}

	meta := &fakeTerrainMetaWriter{}
	svc := NewTerrainService(
		fakeDEMIngestor{inventory: domain.DEMSourceInventory{Files: []domain.DEMSourceFile{{RelativePath: "tile_1.tif", SHA256Checksum: "abc"}}}},
		fakeTerrainPlanner{plan: domain.TerrainBuildPlan{}},
		fakeTerrainRunner{artifacts: domain.TerrainBuildArtifacts{PMTilesPath: pmtilesPath, TilesDir: tmp, FilledDEMPath: filepath.Join(tmp, "filled.tif")}},
		meta,
		fakeTerrainValidator{result: domain.TerrainValidationResult{CoverageOK: true, SeamsOK: true}},
	)

	report, err := svc.Execute(context.Background(), domain.TerrainExportRequest{
		SourceDir:             "copdem",
		Version:               "v1",
		PMTilesOutputPath:     pmtilesPath,
		ManifestOutputPath:    filepath.Join(tmp, "terrain.manifest.json"),
		BuildReportOutputPath: filepath.Join(tmp, "build-report.json"),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if report.PMTilesPath != pmtilesPath {
		t.Fatalf("unexpected report PMTiles path: %q", report.PMTilesPath)
	}
	if meta.manifestPath == "" {
		t.Fatal("expected manifest write call")
	}
	if meta.reportPath == "" {
		t.Fatal("expected build report write call")
	}
}

func TestTerrainServiceExecuteFailsFastOnValidation(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	pmtilesPath := filepath.Join(tmp, "terrain.pmtiles")
	if err := os.WriteFile(pmtilesPath, []byte("pmtiles"), 0o600); err != nil {
		t.Fatalf("write pmtiles fixture: %v", err)
	}

	svc := NewTerrainService(
		fakeDEMIngestor{inventory: domain.DEMSourceInventory{}},
		fakeTerrainPlanner{plan: domain.TerrainBuildPlan{}},
		fakeTerrainRunner{artifacts: domain.TerrainBuildArtifacts{PMTilesPath: pmtilesPath}},
		&fakeTerrainMetaWriter{},
		fakeTerrainValidator{err: errors.New("validation failed")},
	)

	_, err := svc.Execute(context.Background(), domain.TerrainExportRequest{
		SourceDir:         "copdem",
		Version:           "v1",
		PMTilesOutputPath: pmtilesPath,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
