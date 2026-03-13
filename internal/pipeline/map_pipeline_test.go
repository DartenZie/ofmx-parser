package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

type fakeMapMapper struct {
	dataset domain.MapDataset
	err     error
}

func (f fakeMapMapper) MapToMapDataset(_ context.Context, _ domain.OFMXDocument) (domain.MapDataset, error) {
	if f.err != nil {
		return domain.MapDataset{}, f.err
	}
	return f.dataset, nil
}

type fakeGeoJSONWriter struct {
	artifacts domain.MapGeoJSONArtifacts
	err       error
	lastDir   string
}

func (f *fakeGeoJSONWriter) Write(_ context.Context, _ domain.MapDataset, dir string) (domain.MapGeoJSONArtifacts, error) {
	f.lastDir = dir
	if f.err != nil {
		return domain.MapGeoJSONArtifacts{}, f.err
	}
	if f.artifacts.AirportsPath == "" {
		f.artifacts.AirportsPath = filepath.Join(dir, "a.geojson")
	}
	return f.artifacts, nil
}

type fakeTilemakerRunner struct {
	err    error
	called bool
}

func (f *fakeTilemakerRunner) Run(_ context.Context, _ domain.MapExportRequest, _ domain.MapGeoJSONArtifacts) error {
	f.called = true
	return f.err
}

func TestMapServiceExecuteRunsAllStages(t *testing.T) {
	t.Parallel()

	runner := &fakeTilemakerRunner{}
	writer := &fakeGeoJSONWriter{artifacts: domain.MapGeoJSONArtifacts{AirportsPath: "a.geojson"}}
	svc := NewMapService(
		fakeMapMapper{dataset: domain.MapDataset{}},
		writer,
		runner,
	)

	req := domain.MapExportRequest{
		PBFInputPath:      "in.osm.pbf",
		PMTilesOutputPath: filepath.Join(t.TempDir(), "out.pmtiles"),
	}

	artifacts, err := svc.Execute(context.Background(), domain.OFMXDocument{}, req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if artifacts.AirportsPath != "a.geojson" {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}

	if !runner.called {
		t.Fatal("expected tilemaker runner to be called")
	}
}

func TestMapServiceExecuteWrapsMapperError(t *testing.T) {
	t.Parallel()

	svc := NewMapService(
		fakeMapMapper{err: errors.New("map-failed")},
		&fakeGeoJSONWriter{},
		&fakeTilemakerRunner{},
	)

	_, err := svc.Execute(context.Background(), domain.OFMXDocument{}, domain.MapExportRequest{})
	if err == nil {
		t.Fatal("expected mapper error")
	}
}

func TestMapServiceExecuteCleansAutoTempDir(t *testing.T) {
	t.Parallel()

	runner := &fakeTilemakerRunner{}
	writer := &fakeGeoJSONWriter{}
	svc := NewMapService(
		fakeMapMapper{dataset: domain.MapDataset{}},
		writer,
		runner,
	)

	_, err := svc.Execute(context.Background(), domain.OFMXDocument{}, domain.MapExportRequest{
		PBFInputPath:      "in.osm.pbf",
		PMTilesOutputPath: filepath.Join(t.TempDir(), "out.pmtiles"),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if writer.lastDir == "" {
		t.Fatal("expected writer to receive temp dir")
	}

	if _, err := os.Stat(writer.lastDir); !os.IsNotExist(err) {
		t.Fatalf("expected auto temp dir to be removed, stat err=%v", err)
	}
}

func TestMapServiceExecuteKeepsExplicitTempDir(t *testing.T) {
	t.Parallel()

	runner := &fakeTilemakerRunner{}
	writer := &fakeGeoJSONWriter{}
	svc := NewMapService(
		fakeMapMapper{dataset: domain.MapDataset{}},
		writer,
		runner,
	)

	explicitDir := filepath.Join(t.TempDir(), "map-runtime")
	if err := os.MkdirAll(explicitDir, 0o755); err != nil {
		t.Fatalf("create explicit temp dir: %v", err)
	}

	_, err := svc.Execute(context.Background(), domain.OFMXDocument{}, domain.MapExportRequest{
		PBFInputPath:      "in.osm.pbf",
		PMTilesOutputPath: filepath.Join(t.TempDir(), "out.pmtiles"),
		TempDir:           explicitDir,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if _, err := os.Stat(explicitDir); err != nil {
		t.Fatalf("expected explicit temp dir to persist: %v", err)
	}
}
