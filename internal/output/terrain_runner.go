// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pasek
package output

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

const (
	defaultGDALBuildVRTBin   = "gdalbuildvrt"
	defaultGDALFillNodataBin = "gdal_fillnodata.py"
	defaultGDALWarpBin       = "gdalwarp"
	defaultGDALTranslateBin  = "gdal_translate"
	defaultGDALAddoBin       = "gdaladdo"
	defaultGDALCalcBin       = "gdal_calc.py"
	defaultGDALMergeBin      = "gdal_merge.py"
	defaultGDAL2TilesBin     = "gdal2tiles.py"
	defaultGDALDEMBin        = "gdaldem"
	defaultGDALInfoBin       = "gdalinfo"
	defaultGDALLocationBin   = "gdallocationinfo"
	defaultPMTilesBin        = "pmtiles"
)

// TerrainRunner executes the terrain preprocessing and packaging toolchain.
type TerrainRunner interface {
	Run(ctx context.Context, req domain.TerrainExportRequest, plan domain.TerrainBuildPlan, inventory domain.DEMSourceInventory) (domain.TerrainBuildArtifacts, error)
}

// ExecTerrainRunner runs terrain tooling via os/exec.
type ExecTerrainRunner struct{}

// Run executes deterministic preprocessing, pyramid generation, and PMTiles packaging.
//
// Processing order (optimized):
//  1. gdalbuildvrt    – mosaic all AOI-intersecting source files.
//  2. gdalwarp -te    – crop+reproject to AOI in EPSG:3857 first.
//  3. gdal_fillnodata – fill nodata on the already-cropped raster only.
//  4. buildTerrariumRGB – single gdal_calc pass writing all 3 bands at once.
//  5. gdal2tiles.py   – single tile pyramid, used for both PMTiles and validation.
//  6. packTilesDirToPMTiles – write PMTiles v3 directly from tile dir (no MBTiles).
func (r ExecTerrainRunner) Run(ctx context.Context, req domain.TerrainExportRequest, plan domain.TerrainBuildPlan, inventory domain.DEMSourceInventory) (domain.TerrainBuildArtifacts, error) {
	if err := os.MkdirAll(req.BuildDir, 0o755); err != nil {
		return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create terrain build dir %q", req.BuildDir), err)
	}
	if err := os.MkdirAll(plan.TilesDir, 0o755); err != nil {
		return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create terrain tiles dir %q", plan.TilesDir), err)
	}

	bin := normalizeToolchain(req.Toolchain)
	// gdal_translate: Terrarium RGB assembly (GTiff output).
	// pmtiles: still required by the validator's `pmtiles show` call.
	// gdaladdo and gdaldem are no longer in the critical path.
	for _, path := range []string{bin.GDALBuildVRTBin, bin.GDALFillNodataBin, bin.GDALWarpBin, bin.GDALTranslateBin, bin.GDALCalcBin, bin.GDAL2TilesBin, bin.PMTilesBin} {
		if _, err := exec.LookPath(path); err != nil {
			return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, fmt.Sprintf("terrain tool binary %q not found (strict-fail terrain mode)", path), err)
		}
	}

	// Step 1: mosaic.
	sources := make([]string, 0, len(inventory.Files))
	for _, src := range inventory.Files {
		sources = append(sources, src.Path)
	}
	sort.Strings(sources)

	if err := runCmd(ctx, bin.GDALBuildVRTBin, append([]string{plan.MosaicVRTPath}, sources...)...); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	// Step 2: crop + reproject to AOI in EPSG:3857 first (Issue #2: nodata fill
	// will then operate only on the AOI extent, not the full mosaic).
	croppedDEMPath := req.BuildDir + "/dem.cropped.tif"
	if err := runCmd(ctx, bin.GDALWarpBin,
		"-t_srs", "EPSG:3857",
		"-r", "bilinear",
		"-te_srs", "EPSG:4326",
		"-te",
		formatFloat(plan.AOIBounds.MinLon),
		formatFloat(plan.AOIBounds.MinLat),
		formatFloat(plan.AOIBounds.MaxLon),
		formatFloat(plan.AOIBounds.MaxLat),
		"-dstnodata", "-32768",
		plan.MosaicVRTPath,
		croppedDEMPath,
	); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	// Step 3: nodata fill on the cropped AOI raster only.
	if err := runCmd(ctx, bin.GDALFillNodataBin,
		"-md", strconv.Itoa(plan.NodataDistance),
		"-si", strconv.Itoa(plan.NodataSmoothing),
		"-of", "GTiff",
		croppedDEMPath,
		plan.FilledDEMPath,
	); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	// The filled, cropped raster serves as the warped DEM for downstream steps.
	warpedDEMPath := plan.FilledDEMPath

	// Step 4: Terrarium RGB encoding.
	terrainRGBPath, err := buildTerrariumRGB(ctx, bin, req.BuildDir, warpedDEMPath)
	if err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	// Step 5: Single tile pyramid via gdal2tiles (Issue #3: replaces the
	// MBTiles+gdaladdo path as the canonical tiling step; output is used for
	// both validation and PMTiles packaging).
	//
	// Parallelism defaults to all available CPUs (Issue #4).
	processes := req.GDAL2TilesProcesses
	if processes <= 0 {
		processes = runtime.NumCPU()
	}
	// --resampling near is required for Terrarium-encoded data. The RGB bytes
	// encode elevation non-linearly (R carries 256m per unit). Averaging across
	// an R-band boundary (e.g. R=127 and R=128) produces a byte average that
	// decodes to a wrong elevation, causing apparent 100+ m seam discontinuities
	// at tile edges. Nearest-neighbour copies source pixels exactly, so tile
	// edges remain true samples with no interpolation artefacts.
	if err := runCmd(ctx, bin.GDAL2TilesBin,
		"--xyz",
		"--resampling", "near",
		"--tilesize", strconv.Itoa(plan.TileSize),
		"--zoom", fmt.Sprintf("%d-%d", plan.MinZoom, plan.MaxZoom),
		"--processes", strconv.Itoa(processes),
		terrainRGBPath,
		plan.TilesDir,
	); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	// Step 6: Write PMTiles v3 directly from the XYZ tile directory in pure Go.
	// Eliminates both the MBTiles intermediate file and the pmtiles-convert
	// subprocess call — tiles are read once from disk and written straight to
	// the final PMTiles archive.
	if err := packTilesDirToPMTiles(plan.TilesDir, req.PMTilesOutputPath, plan.MinZoom, plan.MaxZoom, plan.AOIBounds); err != nil {
		return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, "failed to write PMTiles from tile dir", err)
	}

	if _, err := os.Stat(req.PMTilesOutputPath); err != nil {
		return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, fmt.Sprintf("terrain pipeline did not produce PMTiles output %q", req.PMTilesOutputPath), err)
	}

	return domain.TerrainBuildArtifacts{
		PMTilesPath:   req.PMTilesOutputPath,
		TilesDir:      plan.TilesDir,
		FilledDEMPath: plan.FilledDEMPath,
		WarpedDEMPath: warpedDEMPath,
	}, nil
}

func normalizeToolchain(tc domain.TerrainToolchain) domain.TerrainToolchain {
	if strings.TrimSpace(tc.GDALBuildVRTBin) == "" {
		tc.GDALBuildVRTBin = defaultGDALBuildVRTBin
	}
	if strings.TrimSpace(tc.GDALFillNodataBin) == "" {
		tc.GDALFillNodataBin = defaultGDALFillNodataBin
	}
	if strings.TrimSpace(tc.GDALWarpBin) == "" {
		tc.GDALWarpBin = defaultGDALWarpBin
	}
	if strings.TrimSpace(tc.GDALTranslateBin) == "" {
		tc.GDALTranslateBin = defaultGDALTranslateBin
	}
	if strings.TrimSpace(tc.GDALAddoBin) == "" {
		tc.GDALAddoBin = defaultGDALAddoBin
	}
	if strings.TrimSpace(tc.GDALCalcBin) == "" {
		tc.GDALCalcBin = defaultGDALCalcBin
	}
	if strings.TrimSpace(tc.GDALMergeBin) == "" {
		tc.GDALMergeBin = defaultGDALMergeBin
	}
	if strings.TrimSpace(tc.GDAL2TilesBin) == "" {
		tc.GDAL2TilesBin = defaultGDAL2TilesBin
	}
	if strings.TrimSpace(tc.GDALDEMBin) == "" {
		tc.GDALDEMBin = defaultGDALDEMBin
	}
	if strings.TrimSpace(tc.GDALInfoBin) == "" {
		tc.GDALInfoBin = defaultGDALInfoBin
	}
	if strings.TrimSpace(tc.GDALLocationInfoBin) == "" {
		tc.GDALLocationInfoBin = defaultGDALLocationBin
	}
	if strings.TrimSpace(tc.PMTilesBin) == "" {
		tc.PMTilesBin = defaultPMTilesBin
	}

	return tc
}

func runCmd(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("%s failed: %v: %s", bin, err, strings.TrimSpace(string(out))), err)
	}
	return nil
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 8, 64)
}

// buildTerrariumRGB encodes the warped DEM as a 3-band Terrarium RGB GeoTIFF
// in a single gdal_calc.py invocation. The previous implementation used three
// separate gdal_calc passes (one per band) plus gdalbuildvrt + gdal_translate,
// reading the DEM five times total. A single pass with three --calc expressions
// reads the DEM once and writes all bands together.
func buildTerrariumRGB(ctx context.Context, bin domain.TerrainToolchain, buildDir, warpedDEMPath string) (string, error) {
	rgb := buildDir + "/terrarium_rgb.tif"

	// Three --calc expressions in one call produce a 3-band output GeoTIFF.
	// Band ordering: R=1, G=2, B=3.
	//   R = floor((elev+32768) / 256)          (high byte)
	//   G = floor((elev+32768) mod 256)         (middle byte)
	//   B = floor(frac(elev+32768) * 256)       (fractional byte)
	if err := runCmd(ctx, bin.GDALCalcBin,
		"-A", warpedDEMPath,
		"--outfile", rgb,
		"--type", "Byte",
		"--NoDataValue", "0",
		"--format", "GTiff",
		"--calc", "clip(floor((A+32768)/256),0,255)",
		"--calc", "clip(floor((A+32768)-floor((A+32768)/256)*256),0,255)",
		"--calc", "clip(floor(((A+32768)-floor(A+32768))*256),0,255)",
	); err != nil {
		return "", err
	}

	return rgb, nil
}
