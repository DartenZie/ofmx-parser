package pipeline

import (
	"context"
	"errors"
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
}

func (f fakeGeoJSONWriter) Write(_ context.Context, _ domain.MapDataset, _ string) (domain.MapGeoJSONArtifacts, error) {
	if f.err != nil {
		return domain.MapGeoJSONArtifacts{}, f.err
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
	svc := NewMapService(
		fakeMapMapper{dataset: domain.MapDataset{}},
		fakeGeoJSONWriter{artifacts: domain.MapGeoJSONArtifacts{AirportsPath: "a.geojson"}},
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
		fakeGeoJSONWriter{},
		&fakeTilemakerRunner{},
	)

	_, err := svc.Execute(context.Background(), domain.OFMXDocument{}, domain.MapExportRequest{})
	if err == nil {
		t.Fatal("expected mapper error")
	}
}
