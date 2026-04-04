// Package pipeline orchestrates ingest, transform, validation, and output.
//
// Author: Miroslav Pašek
package pipeline

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/DartenZie/ofmx-parser/internal/domain"
	"github.com/DartenZie/ofmx-parser/internal/output"
	"github.com/DartenZie/ofmx-parser/internal/transform"
)

// MapService executes the OFMX -> GeoJSON -> PMTiles export branch.
type MapService struct {
	mapper        transform.MapMapper
	geoJSONWriter output.MapGeoJSONWriter
	tilemaker     output.TilemakerRunner
}

// NewMapService constructs a map export service.
func NewMapService(mapper transform.MapMapper, writer output.MapGeoJSONWriter, runner output.TilemakerRunner) MapService {
	return MapService{mapper: mapper, geoJSONWriter: writer, tilemaker: runner}
}

// Execute generates aviation GeoJSON artifacts and invokes tilemaker.
func (s MapService) Execute(ctx context.Context, input domain.OFMXDocument, req domain.MapExportRequest) (artifacts domain.MapGeoJSONArtifacts, runErr error) {
	autoTempDir := false
	startedAt := time.Now()
	defer func() {
		duration := time.Since(startedAt).Round(time.Millisecond)
		if runErr != nil {
			log.Printf("PMTiles pipeline failed after %s", duration)
			return
		}
		log.Printf("PMTiles pipeline finished in %s", duration)
	}()

	mapStartedAt := time.Now()
	log.Printf("Mapping OFMX data to map dataset")

	dataset, err := s.mapper.MapToMapDataset(ctx, input)
	if err != nil {
		runErr = domain.NewError(domain.ErrTransform, "failed to map OFMX to map dataset", err)
		return domain.MapGeoJSONArtifacts{}, runErr
	}
	log.Printf("Mapped OFMX data to map dataset in %s", time.Since(mapStartedAt).Round(time.Millisecond))

	if req.TempDir == "" {
		tmpDir, err := os.MkdirTemp("", "ofmx-map-")
		if err != nil {
			runErr = domain.NewError(domain.ErrOutput, "failed to create temporary map directory", err)
			return domain.MapGeoJSONArtifacts{}, runErr
		}
		req.TempDir = tmpDir
		autoTempDir = true
	}

	if autoTempDir {
		defer func() {
			_ = os.RemoveAll(req.TempDir)
		}()
	}

	geoJSONStartedAt := time.Now()
	log.Printf("Writing GeoJSON runtime artifacts")
	artifacts, err = s.geoJSONWriter.Write(ctx, dataset, req.TempDir)
	if err != nil {
		runErr = err
		return domain.MapGeoJSONArtifacts{}, runErr
	}
	log.Printf("Wrote GeoJSON runtime artifacts in %s", time.Since(geoJSONStartedAt).Round(time.Millisecond))

	if err := output.WriteGeoJSONDebugBundle(ctx, artifacts, req.GeoJSONOutputDir); err != nil {
		runErr = err
		return domain.MapGeoJSONArtifacts{}, runErr
	}

	tilemakerStartedAt := time.Now()
	log.Printf("Running tilemaker and writing PMTiles")

	if err := s.tilemaker.Run(ctx, req, artifacts); err != nil {
		runErr = err
		return domain.MapGeoJSONArtifacts{}, runErr
	}
	log.Printf("Tilemaker finished in %s", time.Since(tilemakerStartedAt).Round(time.Millisecond))

	return artifacts, nil
}
