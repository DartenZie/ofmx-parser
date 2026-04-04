package output

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// WriteGeoJSONDebugBundle copies generated GeoJSON layer files into dir for debugging.
func WriteGeoJSONDebugBundle(ctx context.Context, artifacts domain.MapGeoJSONArtifacts, dir string) error {
	if dir == "" {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return domain.NewError(domain.ErrOutput, "GeoJSON debug write cancelled", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create GeoJSON debug directory %q", dir), err)
	}

	for _, src := range []string{
		artifacts.AirportsPath,
		artifacts.ZonesPath,
		artifacts.PointsOfInterestPath,
		artifacts.AirspaceBordersPath,
		artifacts.CountriesBoundaryPath,
	} {
		if src == "" {
			continue
		}

		payload, err := os.ReadFile(src)
		if err != nil {
			return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to read GeoJSON source file %q", src), err)
		}

		dst := filepath.Join(dir, filepath.Base(src))
		if err := writeFileAtomic(ctx, dst, payload, 0o644); err != nil {
			return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to write GeoJSON debug file %q", dst), err)
		}
	}

	return nil
}
