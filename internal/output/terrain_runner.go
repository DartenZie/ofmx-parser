// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pasek
package output

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
func (r ExecTerrainRunner) Run(ctx context.Context, req domain.TerrainExportRequest, plan domain.TerrainBuildPlan, inventory domain.DEMSourceInventory) (domain.TerrainBuildArtifacts, error) {
	if err := os.MkdirAll(req.BuildDir, 0o755); err != nil {
		return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create terrain build dir %q", req.BuildDir), err)
	}
	if err := os.MkdirAll(plan.TilesDir, 0o755); err != nil {
		return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create terrain tiles dir %q", plan.TilesDir), err)
	}

	bin := normalizeToolchain(req.Toolchain)
	for _, path := range []string{bin.GDALBuildVRTBin, bin.GDALFillNodataBin, bin.GDALWarpBin, bin.GDALTranslateBin, bin.GDALAddoBin, bin.GDALCalcBin, bin.GDALMergeBin, bin.GDAL2TilesBin, bin.GDALDEMBin, bin.PMTilesBin} {
		if _, err := exec.LookPath(path); err != nil {
			return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, fmt.Sprintf("terrain tool binary %q not found (strict-fail terrain mode)", path), err)
		}
	}

	sources := make([]string, 0, len(inventory.Files))
	for _, src := range inventory.Files {
		sources = append(sources, src.Path)
	}
	sort.Strings(sources)

	if err := runCmd(ctx, bin.GDALBuildVRTBin, append([]string{plan.MosaicVRTPath}, sources...)...); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	if err := runCmd(ctx, bin.GDALFillNodataBin,
		"-md", strconv.Itoa(plan.NodataDistance),
		"-si", strconv.Itoa(plan.NodataSmoothing),
		"-of", "GTiff",
		plan.MosaicVRTPath,
		plan.FilledDEMPath,
	); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

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
		plan.FilledDEMPath,
		plan.WarpedDEMPath,
	); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	terrainRGBPath, err := buildTerrariumRGB(ctx, bin, req.BuildDir, plan.WarpedDEMPath)
	if err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}
	mbtilesPath := req.BuildDir + "/terrain.mbtiles"
	if err := runCmd(ctx, bin.GDALTranslateBin,
		"-of", "MBTILES",
		"-co", "TILE_FORMAT=PNG",
		"-co", "RESAMPLING=NEAREST",
		"-co", "BLOCKSIZE="+strconv.Itoa(plan.TileSize),
		"-co", "MINZOOM="+strconv.Itoa(plan.MinZoom),
		"-co", "MAXZOOM="+strconv.Itoa(plan.MaxZoom),
		terrainRGBPath,
		mbtilesPath,
	); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	if plan.MaxZoom > plan.MinZoom {
		addoArgs := []string{"-r", "average", mbtilesPath}
		factor := 2
		for i := 0; i < 12; i++ {
			addoArgs = append(addoArgs, strconv.Itoa(factor))
			factor *= 2
		}
		if err := runCmd(ctx, bin.GDALAddoBin, addoArgs...); err != nil {
			return domain.TerrainBuildArtifacts{}, err
		}
	}

	if err := runCmd(ctx, bin.GDAL2TilesBin,
		"--xyz",
		"--tilesize", strconv.Itoa(plan.TileSize),
		"--zoom", fmt.Sprintf("%d-%d", plan.MinZoom, plan.MaxZoom),
		"--processes", "1",
		terrainRGBPath,
		plan.TilesDir,
	); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	if err := runCmd(ctx, bin.GDALDEMBin,
		"hillshade",
		plan.WarpedDEMPath,
		plan.HillshadePath,
		"-z", "1.0",
		"-az", "315",
		"-alt", "45",
	); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	rawPMTilesPath := req.BuildDir + "/terrain.raw.pmtiles"
	if err := runCmd(ctx, bin.PMTilesBin, "convert", mbtilesPath, rawPMTilesPath); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	bbox := fmt.Sprintf("%s,%s,%s,%s",
		formatFloat(plan.AOIBounds.MinLon),
		formatFloat(plan.AOIBounds.MinLat),
		formatFloat(plan.AOIBounds.MaxLon),
		formatFloat(plan.AOIBounds.MaxLat),
	)
	if err := runCmd(ctx, bin.PMTilesBin,
		"extract",
		rawPMTilesPath,
		req.PMTilesOutputPath,
		"--bbox="+bbox,
		"--minzoom="+strconv.Itoa(plan.MinZoom),
		"--maxzoom="+strconv.Itoa(plan.MaxZoom),
	); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}

	if _, err := os.Stat(req.PMTilesOutputPath); err != nil {
		return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, fmt.Sprintf("terrain pipeline did not produce PMTiles output %q", req.PMTilesOutputPath), err)
	}

	return domain.TerrainBuildArtifacts{
		PMTilesPath:   req.PMTilesOutputPath,
		TilesDir:      plan.TilesDir,
		FilledDEMPath: plan.FilledDEMPath,
		WarpedDEMPath: plan.WarpedDEMPath,
		HillshadePath: plan.HillshadePath,
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

func buildTerrariumRGB(ctx context.Context, bin domain.TerrainToolchain, buildDir, warpedDEMPath string) (string, error) {
	rBand := buildDir + "/terrarium_r.tif"
	gBand := buildDir + "/terrarium_g.tif"
	bBand := buildDir + "/terrarium_b.tif"
	rgbVRT := buildDir + "/terrarium_rgb.vrt"
	rgb := buildDir + "/terrarium_rgb.tif"

	if err := runCmd(ctx, bin.GDALCalcBin,
		"-A", warpedDEMPath,
		"--outfile", rBand,
		"--type", "Byte",
		"--NoDataValue", "0",
		"--calc", "clip(floor((A+32768)/256),0,255)",
	); err != nil {
		return "", err
	}

	if err := runCmd(ctx, bin.GDALCalcBin,
		"-A", warpedDEMPath,
		"--outfile", gBand,
		"--type", "Byte",
		"--NoDataValue", "0",
		"--calc", "clip(floor((A+32768)-floor((A+32768)/256)*256),0,255)",
	); err != nil {
		return "", err
	}

	if err := runCmd(ctx, bin.GDALCalcBin,
		"-A", warpedDEMPath,
		"--outfile", bBand,
		"--type", "Byte",
		"--NoDataValue", "0",
		"--calc", "clip(floor(((A+32768)-floor(A+32768))*256),0,255)",
	); err != nil {
		return "", err
	}

	if err := runCmd(ctx, bin.GDALBuildVRTBin,
		"-separate",
		rgbVRT,
		rBand,
		gBand,
		bBand,
	); err != nil {
		return "", err
	}

	if err := runCmd(ctx, bin.GDALTranslateBin,
		"-of", "GTiff",
		rgbVRT,
		rgb,
	); err != nil {
		return "", err
	}

	return rgb, nil
}
