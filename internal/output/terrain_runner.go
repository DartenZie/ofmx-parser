// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pasek
package output

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

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
//  4. quantizeDEM     – optional: round elevation to nearest N metres to reduce PNG entropy.
//  5. buildTerrariumRGB – single gdal_calc pass writing all 3 bands at once.
//  6. gdal2tiles.py   – single tile pyramid, used for both PMTiles and validation.
//  7. clipTilesOutsidePolygon – optional: zero-out (remove) tiles that fall entirely
//     outside the supplied clip polygon (e.g. exact country boundary).
//  8. packTilesDirToPMTiles – write PMTiles v3 directly from tile dir (no MBTiles).
func (r ExecTerrainRunner) Run(ctx context.Context, req domain.TerrainExportRequest, plan domain.TerrainBuildPlan, inventory domain.DEMSourceInventory) (domain.TerrainBuildArtifacts, error) {
	startedAt := time.Now()
	log.Printf("Terrain runner: start (sources=%d, zoom=%d-%d, buildDir=%q)", len(inventory.Files), plan.MinZoom, plan.MaxZoom, req.BuildDir)

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

	stepStartedAt := time.Now()
	if err := runCmd(ctx, bin.GDALBuildVRTBin, append([]string{plan.MosaicVRTPath}, sources...)...); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}
	log.Printf("Terrain runner: step 1/8 gdalbuildvrt finished in %s", time.Since(stepStartedAt).Round(time.Millisecond))

	// Step 2: crop + reproject to AOI in EPSG:3857 first (Issue #2: nodata fill
	// will then operate only on the AOI extent, not the full mosaic).
	croppedDEMPath := req.BuildDir + "/dem.cropped.tif"
	stepStartedAt = time.Now()
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
	log.Printf("Terrain runner: step 2/8 gdalwarp finished in %s", time.Since(stepStartedAt).Round(time.Millisecond))

	// Step 3: nodata fill on the cropped AOI raster only.
	stepStartedAt = time.Now()
	if err := runCmd(ctx, bin.GDALFillNodataBin,
		"-md", strconv.Itoa(plan.NodataDistance),
		"-si", strconv.Itoa(plan.NodataSmoothing),
		"-of", "GTiff",
		croppedDEMPath,
		plan.FilledDEMPath,
	); err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}
	log.Printf("Terrain runner: step 3/8 gdal_fillnodata finished in %s", time.Since(stepStartedAt).Round(time.Millisecond))

	// The filled, cropped raster serves as the warped DEM for downstream steps.
	warpedDEMPath := plan.FilledDEMPath

	// Step 4 (optional): elevation quantization.
	// Rounds each pixel to the nearest ElevationQuantizationM metres before
	// Terrarium encoding. This dramatically reduces blue-channel entropy, making
	// tiles significantly more compressible by PNG's DEFLATE, at the cost of
	// sub-metre elevation precision (acceptable for 1 m quantization).
	if plan.ElevationQuantizationM > 0 {
		stepStartedAt = time.Now()
		quantized, err := quantizeDEM(ctx, bin, plan.FilledDEMPath, plan.QuantizedDEMPath, plan.ElevationQuantizationM)
		if err != nil {
			return domain.TerrainBuildArtifacts{}, err
		}
		warpedDEMPath = quantized
		log.Printf("Terrain runner: step 4/8 quantize DEM (%.3fm) finished in %s", plan.ElevationQuantizationM, time.Since(stepStartedAt).Round(time.Millisecond))
	} else {
		log.Printf("Terrain runner: step 4/8 quantize DEM skipped (quantization disabled)")
	}

	// Step 5: Terrarium RGB encoding.
	stepStartedAt = time.Now()
	terrainRGBPath, err := buildTerrariumRGB(ctx, bin, req.BuildDir, warpedDEMPath)
	if err != nil {
		return domain.TerrainBuildArtifacts{}, err
	}
	log.Printf("Terrain runner: step 5/8 Terrarium RGB encoding finished in %s", time.Since(stepStartedAt).Round(time.Millisecond))

	// Step 6: Single tile pyramid via gdal2tiles (Issue #3: replaces the
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
	stepStartedAt = time.Now()
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
	log.Printf("Terrain runner: step 6/8 gdal2tiles finished in %s (processes=%d)", time.Since(stepStartedAt).Round(time.Millisecond), processes)

	// Step 7 (optional): remove tiles that fall entirely outside the clip polygon.
	// gdal2tiles always generates a full rectangular tile grid for the AOI bbox;
	// this step deletes tiles whose coverage does not intersect the polygon at all,
	// reducing the tile count (and thus PMTiles size) for irregularly shaped AOIs
	// such as country outlines.
	if plan.ClipPolygonPath != "" {
		beforeCount, err := countPNGTiles(plan.TilesDir)
		if err != nil {
			return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, "failed to count tiles before clipping", err)
		}

		stepStartedAt = time.Now()
		clipPath, err := prepareClipPolygon(ctx, req.BuildDir, plan.ClipPolygonPath, plan.ClipPolygonCountryName)
		if err != nil {
			return domain.TerrainBuildArtifacts{}, err
		}
		removed, err := clipTilesOutsidePolygon(ctx, bin, plan.TilesDir, clipPath, plan.MinZoom, plan.MaxZoom, plan.AOIBounds)
		if err != nil {
			return domain.TerrainBuildArtifacts{}, err
		}

		afterCount, err := countPNGTiles(plan.TilesDir)
		if err != nil {
			return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, "failed to count tiles after clipping", err)
		}
		log.Printf("Terrain runner: step 7/8 clip tiles finished in %s (before=%d, removed=%d, after=%d, polygon=%q, countryFilter=%q)",
			time.Since(stepStartedAt).Round(time.Millisecond), beforeCount, removed, afterCount, clipPath, plan.ClipPolygonCountryName)
	} else {
		log.Printf("Terrain runner: step 7/8 clip tiles skipped (no clip polygon configured)")
	}

	// Step 8: Write PMTiles v3 directly from the XYZ tile directory in pure Go.
	// Eliminates both the MBTiles intermediate file and the pmtiles-convert
	// subprocess call — tiles are read once from disk and written straight to
	// the final PMTiles archive.
	stepStartedAt = time.Now()
	if err := packTilesDirToPMTiles(plan.TilesDir, req.PMTilesOutputPath, plan.MinZoom, plan.MaxZoom, plan.AOIBounds); err != nil {
		return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, "failed to write PMTiles from tile dir", err)
	}
	log.Printf("Terrain runner: step 8/8 PMTiles packing finished in %s", time.Since(stepStartedAt).Round(time.Millisecond))

	pmtilesStat, err := os.Stat(req.PMTilesOutputPath)
	if err != nil {
		return domain.TerrainBuildArtifacts{}, domain.NewError(domain.ErrOutput, fmt.Sprintf("terrain pipeline did not produce PMTiles output %q", req.PMTilesOutputPath), err)
	}
	log.Printf("Terrain runner: finished in %s (output=%q, size=%d bytes)", time.Since(startedAt).Round(time.Millisecond), req.PMTilesOutputPath, pmtilesStat.Size())

	return domain.TerrainBuildArtifacts{
		PMTilesPath:   req.PMTilesOutputPath,
		TilesDir:      plan.TilesDir,
		FilledDEMPath: plan.FilledDEMPath,
		WarpedDEMPath: warpedDEMPath,
	}, nil
}

func countPNGTiles(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".png") {
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
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
	startedAt := time.Now()
	log.Printf("Terrain runner: command start: %s", bin)
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("%s failed: %v: %s", bin, err, strings.TrimSpace(string(out))), err)
	}
	log.Printf("Terrain runner: command done: %s (%s)", bin, time.Since(startedAt).Round(time.Millisecond))
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

// quantizeDEM rounds elevation values in a Float32/Float64 GeoTIFF to the
// nearest multiple of stepM metres. This eliminates sub-metre noise introduced
// by bilinear resampling, dramatically reducing the entropy of the Terrarium
// blue channel and improving PNG DEFLATE compression ratios.
//
// The output is a new Float32 GeoTIFF written to outPath; nodata pixels
// (value == -32768) are preserved unchanged.
func quantizeDEM(ctx context.Context, bin domain.TerrainToolchain, inPath, outPath string, stepM float64) (string, error) {
	// gdal_calc expression: round(A / step) * step, preserving nodata=-32768.
	// where(A==-32768, -32768, round(A/step)*step)
	// gdal_calc clips the conditional but Float32 output keeps the sign.
	step := strconv.FormatFloat(stepM, 'f', -1, 64)
	calcExpr := fmt.Sprintf("where(A==-32768,-32768,numpy.round(A/%s)*%s)", step, step)
	if err := runCmd(ctx, bin.GDALCalcBin,
		"-A", inPath,
		"--outfile", outPath,
		"--type", "Float32",
		"--NoDataValue", "-32768",
		"--format", "GTiff",
		"--calc", calcExpr,
	); err != nil {
		return "", domain.NewError(domain.ErrOutput, "elevation quantization step failed", err)
	}
	return outPath, nil
}

// prepareClipPolygon ensures the clip polygon file is in polygon geometry form.
// If the source file already contains Polygon or MultiPolygon geometry it is
// returned unchanged. If it contains LineString geometry (e.g. the
// countries_boundary.geojson produced by the map pipeline) ogr2ogr is used to
// build a convex hull of the collected segments and the result is written to
// buildDir/clip_polygon.geojson. countryName is an optional filter applied to
// the "name" property of the features; when non-empty only features whose
// name contains that string are included in the hull.
func prepareClipPolygon(ctx context.Context, buildDir, polygonPath, countryName string) (string, error) {
	ogrinfo := "ogrinfo"
	if _, err := exec.LookPath(ogrinfo); err != nil {
		return "", domain.NewError(domain.ErrOutput, "clip polygon requested but ogrinfo is not available in PATH", err)
	}

	// Detect the geometry type of the first layer.
	cmd := exec.CommandContext(ctx, ogrinfo, "-al", "-so", polygonPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", domain.NewError(domain.ErrOutput,
			fmt.Sprintf("failed to inspect clip polygon file %q: %s", polygonPath, strings.TrimSpace(string(out))), err)
	}
	outStr := string(out)

	// Look for a "Geometry:" line in the ogrinfo summary output.
	isLineString := false
	for _, line := range strings.Split(outStr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Geometry:") {
			geomType := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "Geometry:")))
			if strings.Contains(geomType, "line") {
				isLineString = true
			}
			break
		}
	}

	if !isLineString {
		// Already polygon (or unknown) – use as-is.
		return polygonPath, nil
	}

	// Build a convex-hull polygon from the LineString features using ogr2ogr
	// with the SQLite dialect. When countryName is set only matching features
	// are included; otherwise all features are collected.
	outPath := buildDir + "/clip_polygon.geojson"

	// Remove any stale file from a previous build so ogr2ogr does not fail.
	_ = os.Remove(outPath)

	var sqlExpr string
	if countryName != "" {
		sqlExpr = fmt.Sprintf(
			"SELECT ST_ConvexHull(ST_Collect(geometry)) AS geometry FROM %s WHERE name LIKE '%%%s%%'",
			layerNameFromPath(polygonPath), countryName,
		)
	} else {
		sqlExpr = fmt.Sprintf(
			"SELECT ST_ConvexHull(ST_Collect(geometry)) AS geometry FROM %s",
			layerNameFromPath(polygonPath),
		)
	}

	ogr2ogr := "ogr2ogr"
	if _, err := exec.LookPath(ogr2ogr); err != nil {
		return "", domain.NewError(domain.ErrOutput, "clip polygon conversion requires ogr2ogr in PATH", err)
	}

	cmd = exec.CommandContext(ctx, ogr2ogr, "-f", "GeoJSON", "-dialect", "SQLITE", "-sql", sqlExpr, "-nlt", "POLYGON", outPath, polygonPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", domain.NewError(domain.ErrOutput,
			fmt.Sprintf("failed to build clip polygon from border lines: %s", strings.TrimSpace(string(out))), err)
	}

	// Verify the output actually contains a polygon feature.
	cmd = exec.CommandContext(ctx, ogrinfo, "-al", "-so", outPath)
	out, err = cmd.CombinedOutput()
	if err != nil {
		return "", domain.NewError(domain.ErrOutput,
			fmt.Sprintf("failed to inspect generated clip polygon %q: %s", outPath, strings.TrimSpace(string(out))), err)
	}
	outStr = string(out)
	hasPolygon := false
	for _, line := range strings.Split(outStr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Geometry:") {
			geomType := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "Geometry:")))
			if strings.Contains(geomType, "polygon") {
				hasPolygon = true
			}
			break
		}
	}
	if !hasPolygon {
		if countryName != "" {
			return "", domain.NewError(domain.ErrOutput,
				fmt.Sprintf("failed to build polygon from clip border file %q using country filter %q (no polygon geometry produced)", polygonPath, countryName), nil)
		}
		return "", domain.NewError(domain.ErrOutput,
			fmt.Sprintf("failed to build polygon from clip border file %q (no polygon geometry produced)", polygonPath), nil)
	}

	return outPath, nil
}

// layerNameFromPath returns the ogr2ogr layer name for a GeoJSON file.
// For GeoJSON files produced by the map pipeline the layer name equals the
// file base name without extension.
func layerNameFromPath(p string) string {
	base := filepath.Base(p)
	if ext := filepath.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}
	return base
}

// clipTilesOutsidePolygon removes PNG tiles from the XYZ tile directory whose
// geographic extent does not overlap the given polygon file (GeoJSON or
// Shapefile). Tiles that intersect the polygon are kept; fully-outside tiles
// are deleted from disk so they are not packed into the PMTiles archive.
//
// Intersection is determined by testing the tile's WGS-84 bounding box against
// the polygon using ogr2ogr with a spatial filter: if the spatial filter
// returns zero features the tile is outside the polygon.
//
// This is the primary mechanism for reducing file size when the AOI is an
// irregular shape (e.g. country outline) rather than a simple bounding box.
func clipTilesOutsidePolygon(ctx context.Context, bin domain.TerrainToolchain, tilesDir, polygonPath string, minZoom, maxZoom int, aoi domain.BoundingBox) (int, error) {
	removed := 0
	// Walk every zoom level and every tile, test intersection, delete if outside.
	for z := minZoom; z <= maxZoom; z++ {
		minX, maxX, minY, maxY := tileRangeForClip(aoi, z)
		for x := minX; x <= maxX; x++ {
			for y := minY; y <= maxY; y++ {
				tilePath := fmt.Sprintf("%s/%d/%d/%d.png", tilesDir, z, x, y)
				if _, err := os.Stat(tilePath); os.IsNotExist(err) {
					continue
				}
				// Compute the WGS-84 extent of this tile.
				minLon, minLat, maxLon, maxLat := tileToWGS84Bounds(x, y, z)
				// Use ogr2ogr to do a quick spatial filter on the polygon.
				// If no features are returned, the tile does not overlap the polygon.
				intersects, err := tileIntersectsPolygon(ctx, bin, polygonPath, minLon, minLat, maxLon, maxLat)
				if err != nil {
					return 0, domain.NewError(domain.ErrOutput,
						fmt.Sprintf("polygon intersection check failed for tile %d/%d/%d", z, x, y), err)
				}
				if !intersects {
					if err := os.Remove(tilePath); err != nil && !os.IsNotExist(err) {
						return 0, domain.NewError(domain.ErrOutput,
							fmt.Sprintf("failed to remove out-of-polygon tile %s", tilePath), err)
					}
					removed++
				}
			}
		}
	}
	return removed, nil
}

// tileRangeForClip returns the XY tile range for a bounding box at zoom z.
// Mirrors the logic in the validator's tileRangeForBBox.
func tileRangeForClip(bbox domain.BoundingBox, z int) (minX, maxX, minY, maxY int) {
	minX, maxY = wgs84ToTileXY(bbox.MinLon, bbox.MinLat, z)
	maxX, minY = wgs84ToTileXY(bbox.MaxLon, bbox.MaxLat, z)
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	return
}

// wgs84ToTileXY converts WGS-84 lon/lat to XYZ slippy-map tile coordinates.
func wgs84ToTileXY(lon, lat float64, z int) (x, y int) {
	n := float64(int(1) << z)
	if lat > 85.05112878 {
		lat = 85.05112878
	}
	if lat < -85.05112878 {
		lat = -85.05112878
	}
	x = int(math.Floor((lon + 180.0) / 360.0 * n))
	latRad := lat * math.Pi / 180.0
	y = int(math.Floor((1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n))
	maxTile := int(n) - 1
	if x < 0 {
		x = 0
	}
	if x > maxTile {
		x = maxTile
	}
	if y < 0 {
		y = 0
	}
	if y > maxTile {
		y = maxTile
	}
	return
}

// tileToWGS84Bounds returns the WGS-84 bounding box for an XYZ slippy-map tile.
func tileToWGS84Bounds(x, y, z int) (minLon, minLat, maxLon, maxLat float64) {
	n := float64(int(1) << z)
	minLon = float64(x)/n*360.0 - 180.0
	maxLon = float64(x+1)/n*360.0 - 180.0
	maxLat = math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180.0 / math.Pi
	minLat = math.Atan(math.Sinh(math.Pi*(1-2*float64(y+1)/n))) * 180.0 / math.Pi
	return
}

// tileIntersectsPolygon returns true when the tile bounding box (in WGS-84)
// overlaps at least one feature in the polygon file.
// It uses ogr2ogr with a -spat filter to count matching features; if ogr2ogr
// is not available the function falls back to returning true (keep all tiles).
func tileIntersectsPolygon(ctx context.Context, bin domain.TerrainToolchain, polygonPath string, minLon, minLat, maxLon, maxLat float64) (bool, error) {
	// We use ogrinfo -al -so with a spatial filter; if it reports 0 features the
	// tile is outside. ogrinfo is part of every standard GDAL installation.
	ogrinfo := "ogrinfo"
	if _, err := exec.LookPath(ogrinfo); err != nil {
		return false, domain.NewError(domain.ErrOutput, "clip polygon requested but ogrinfo is not available in PATH", err)
	}

	minLonStr := strconv.FormatFloat(minLon, 'f', 8, 64)
	minLatStr := strconv.FormatFloat(minLat, 'f', 8, 64)
	maxLonStr := strconv.FormatFloat(maxLon, 'f', 8, 64)
	maxLatStr := strconv.FormatFloat(maxLat, 'f', 8, 64)

	cmd := exec.CommandContext(ctx, ogrinfo, "-al", "-so", "-spat", minLonStr, minLatStr, maxLonStr, maxLatStr, polygonPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, domain.NewError(domain.ErrOutput,
			fmt.Sprintf("ogrinfo spatial filter failed for clip polygon %q: %s", polygonPath, strings.TrimSpace(string(out))), err)
	}

	// ogrinfo -so output contains "Feature Count: N" when features match.
	outStr := string(out)
	for _, line := range strings.Split(outStr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Feature Count:") {
			rest := strings.TrimPrefix(line, "Feature Count:")
			n, err := strconv.Atoi(strings.TrimSpace(rest))
			if err == nil {
				return n > 0, nil
			}
		}
	}
	return false, domain.NewError(domain.ErrOutput,
		fmt.Sprintf("ogrinfo output did not contain parsable feature count for clip polygon %q", polygonPath), nil)
}
