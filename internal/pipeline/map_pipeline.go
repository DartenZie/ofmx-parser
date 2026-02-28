// Package pipeline orchestrates ingest, transform, validation, and output.
//
// Author: Miroslav Pašek
package pipeline

import (
	"context"
	"os"

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
func (s MapService) Execute(ctx context.Context, input domain.OFMXDocument, req domain.MapExportRequest) (domain.MapGeoJSONArtifacts, error) {
	dataset, err := s.mapper.MapToMapDataset(ctx, input)
	if err != nil {
		return domain.MapGeoJSONArtifacts{}, domain.NewError(domain.ErrTransform, "failed to map OFMX to map dataset", err)
	}

	if req.TempDir == "" {
		tmpDir, err := os.MkdirTemp("", "ofmx-map-")
		if err != nil {
			return domain.MapGeoJSONArtifacts{}, domain.NewError(domain.ErrOutput, "failed to create temporary map directory", err)
		}
		req.TempDir = tmpDir
	}

	artifacts, err := s.geoJSONWriter.Write(ctx, dataset, req.TempDir)
	if err != nil {
		return domain.MapGeoJSONArtifacts{}, err
	}

	if err := s.tilemaker.Run(ctx, req, artifacts); err != nil {
		return domain.MapGeoJSONArtifacts{}, err
	}

	return artifacts, nil
}
