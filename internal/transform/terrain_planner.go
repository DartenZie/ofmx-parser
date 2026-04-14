// Package transform maps source data into deterministic build plans.
//
// Author: Miroslav Pasek
package transform

import (
	"context"
	"path/filepath"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// TerrainPlanner creates deterministic preprocessing plans.
type TerrainPlanner interface {
	Plan(ctx context.Context, req domain.TerrainExportRequest, inventory domain.DEMSourceInventory) (domain.TerrainBuildPlan, error)
}

// DefaultTerrainPlanner provides default path/layout planning.
type DefaultTerrainPlanner struct{}

// Plan creates a deterministic terrain build plan for one build directory.
func (p DefaultTerrainPlanner) Plan(_ context.Context, req domain.TerrainExportRequest, _ domain.DEMSourceInventory) (domain.TerrainBuildPlan, error) {
	return domain.TerrainBuildPlan{
		MosaicVRTPath:   filepath.Join(req.BuildDir, "mosaic.vrt"),
		FilledDEMPath:   filepath.Join(req.BuildDir, "dem.filled.tif"),
		WarpedDEMPath:   filepath.Join(req.BuildDir, "dem.webmerc.tif"),
		TilesDir:        filepath.Join(req.BuildDir, "tiles"),
		AOIBounds:       req.AOIBounds,
		Encoding:        req.Encoding,
		TileSize:        req.TileSize,
		MinZoom:         req.MinZoom,
		MaxZoom:         req.MaxZoom,
		NodataDistance:  req.NodataFillMaxDistance,
		NodataSmoothing: req.NodataFillSmoothingIter,
		BuildTimestamp:  req.BuildTimestamp,
		VerticalDatum:   req.VerticalDatum,
		SchemaVersion:   req.SchemaVersion,
		SourceVersion:   req.Version,
	}, nil
}
