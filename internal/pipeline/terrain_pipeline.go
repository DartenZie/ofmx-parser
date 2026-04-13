// Package pipeline orchestrates ingest, transform, validation, and output.
//
// Author: Miroslav Pasek
package pipeline

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/DartenZie/ofmx-parser/internal/domain"
	"github.com/DartenZie/ofmx-parser/internal/ingest"
	"github.com/DartenZie/ofmx-parser/internal/output"
	"github.com/DartenZie/ofmx-parser/internal/transform"
)

// TerrainService executes Copernicus DEM preprocessing and packaging.
type TerrainService struct {
	ingestor  ingest.DEMSourceIngestor
	planner   transform.TerrainPlanner
	runner    output.TerrainRunner
	meta      output.TerrainMetadataWriter
	validator output.TerrainValidator
}

// NewTerrainService constructs a terrain export service.
func NewTerrainService(ingestor ingest.DEMSourceIngestor, planner transform.TerrainPlanner, runner output.TerrainRunner, meta output.TerrainMetadataWriter, validator output.TerrainValidator) TerrainService {
	return TerrainService{
		ingestor:  ingestor,
		planner:   planner,
		runner:    runner,
		meta:      meta,
		validator: validator,
	}
}

// Execute runs ingest, preprocessing, validation, and metadata output.
func (s TerrainService) Execute(ctx context.Context, req domain.TerrainExportRequest) (domain.TerrainBuildReport, error) {
	startedAt := time.Now()
	log.Printf("Starting terrain pipeline")

	autoBuildDir := false
	if req.BuildDir == "" {
		tmp, err := os.MkdirTemp("", "ofmx-terrain-")
		if err != nil {
			return domain.TerrainBuildReport{}, domain.NewError(domain.ErrOutput, "failed to create terrain temp directory", err)
		}
		req.BuildDir = tmp
		autoBuildDir = true
	}
	if autoBuildDir {
		defer func() {
			_ = os.RemoveAll(req.BuildDir)
		}()
	}

	if req.ManifestOutputPath == "" {
		req.ManifestOutputPath = filepath.Join(filepath.Dir(req.PMTilesOutputPath), "terrain.manifest.json")
	}

	inventory, err := s.ingestor.Ingest(ctx, req.SourceDir, req.SourceChecksumsPath)
	if err != nil {
		return domain.TerrainBuildReport{}, err
	}

	plan, err := s.planner.Plan(ctx, req, inventory)
	if err != nil {
		return domain.TerrainBuildReport{}, err
	}

	artifacts, err := s.runner.Run(ctx, req, plan, inventory)
	if err != nil {
		return domain.TerrainBuildReport{}, err
	}

	pmtilesChecksum, err := output.SHA256File(artifacts.PMTilesPath)
	if err != nil {
		return domain.TerrainBuildReport{}, domain.NewError(domain.ErrOutput, "failed to compute PMTiles checksum", err)
	}

	manifest := output.BuildTerrainManifest(req, inventory, pmtilesChecksum)
	if err := s.meta.WriteManifest(ctx, req.ManifestOutputPath, manifest); err != nil {
		return domain.TerrainBuildReport{}, err
	}

	validation, err := s.validator.Validate(ctx, req, artifacts, manifest)
	if err != nil {
		return domain.TerrainBuildReport{}, err
	}

	report := domain.TerrainBuildReport{
		Version:        req.Version,
		BuildTimestamp: manifest.BuildTimestamp,
		ManifestPath:   req.ManifestOutputPath,
		PMTilesPath:    artifacts.PMTilesPath,
		Validation:     validation,
		SourceFiles:    inventory.Files,
	}
	if err := s.meta.WriteBuildReport(ctx, req.BuildReportOutputPath, report); err != nil {
		return domain.TerrainBuildReport{}, err
	}

	log.Printf("Terrain pipeline finished in %s", time.Since(startedAt).Round(time.Millisecond))
	return report, nil
}
