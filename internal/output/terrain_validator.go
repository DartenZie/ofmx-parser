// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pasek
package output

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// TerrainValidator executes quality gates and returns validation metrics.
type TerrainValidator interface {
	Validate(ctx context.Context, req domain.TerrainExportRequest, artifacts domain.TerrainBuildArtifacts, manifest domain.TerrainManifest) (domain.TerrainValidationResult, error)
}

// DefaultTerrainValidator validates coverage, seam quality, elevation checks, and metadata consistency.
type DefaultTerrainValidator struct{}

func (v DefaultTerrainValidator) Validate(ctx context.Context, req domain.TerrainExportRequest, artifacts domain.TerrainBuildArtifacts, manifest domain.TerrainManifest) (domain.TerrainValidationResult, error) {
	clipPolygonPath := ""
	if strings.TrimSpace(req.ClipPolygonPath) != "" {
		prepared, err := prepareClipPolygon(ctx, req.BuildDir, req.ClipPolygonPath, req.ClipPolygonCountryName)
		if err != nil {
			return domain.TerrainValidationResult{}, err
		}
		clipPolygonPath = prepared
	}

	missing, maxSeamDelta, err := coverageAndSeams(ctx, artifacts.TilesDir, req.AOIBounds, req.MinZoom, req.MaxZoom, clipPolygonPath, req.Toolchain)
	if err != nil {
		return domain.TerrainValidationResult{}, err
	}
	if missing > 0 {
		return domain.TerrainValidationResult{}, domain.NewError(domain.ErrValidate, fmt.Sprintf("terrain coverage validation failed: %d missing tiles", missing), nil)
	}
	if maxSeamDelta > req.SeamPixelThreshold {
		return domain.TerrainValidationResult{}, domain.NewError(domain.ErrValidate, fmt.Sprintf("seam validation failed: max seam delta=%d threshold=%d", maxSeamDelta, req.SeamPixelThreshold), nil)
	}

	rmse, cpCompared, err := runElevationChecks(ctx, req, artifacts.FilledDEMPath)
	if err != nil {
		return domain.TerrainValidationResult{}, err
	}
	if cpCompared > 0 && rmse > req.RMSEThresholdM {
		return domain.TerrainValidationResult{}, domain.NewError(domain.ErrValidate, fmt.Sprintf("elevation validation failed: RMSE=%.3fm threshold=%.3fm", rmse, req.RMSEThresholdM), nil)
	}

	// Issue #4: replaced gdalinfo -stats (full raster scan subprocess) with a
	// Go-native PNG stat scan on the warped DEM's tile representation.
	// We scan the warped DEM (GeoTIFF) by decoding one representative tile from
	// the tile directory instead; if none is available we fall back to checking
	// that the WarpedDEMPath file exists and is non-empty.
	if err := validateRasterSanity(artifacts.WarpedDEMPath, artifacts.TilesDir, req.MinZoom, req.MaxZoom, req.AOIBounds); err != nil {
		return domain.TerrainValidationResult{}, err
	}

	if err := validatePMTilesMetadataConsistency(ctx, req.Toolchain, artifacts.PMTilesPath, manifest); err != nil {
		return domain.TerrainValidationResult{}, err
	}

	return domain.TerrainValidationResult{
		CoverageOK:            true,
		MissingTiles:          missing,
		MaxSeamDelta:          maxSeamDelta,
		SeamsOK:               true,
		RMSEm:                 rmse,
		ControlPointsCompared: cpCompared,
		ElevationChecksOK:     cpCompared == 0 || rmse <= req.RMSEThresholdM,
		RasterSanityOK:        true,
		MetadataConsistencyOK: true,
	}, nil
}

// validateRasterSanity checks that the warped DEM file exists and is non-empty,
// then scans the central tile from the tile directory to verify that at least
// one non-nodata pixel is present. This replaces the former gdalinfo -stats
// subprocess call (Issue #4), avoiding a full raster-wide scan.
func validateRasterSanity(warpedDEMPath, tilesDir string, minZoom, maxZoom int, bbox domain.BoundingBox) error {
	// Basic file sanity on the warped DEM.
	fi, err := os.Stat(warpedDEMPath)
	if err != nil {
		return domain.NewError(domain.ErrValidate, fmt.Sprintf("raster sanity: warped DEM not found: %q", warpedDEMPath), err)
	}
	if fi.Size() == 0 {
		return domain.NewError(domain.ErrValidate, fmt.Sprintf("raster sanity: warped DEM is empty: %q", warpedDEMPath), nil)
	}

	// Scan one representative tile at the middle zoom level to confirm data
	// presence (non-transparent pixel). Use the tile at the centre of the AOI.
	midZ := (minZoom + maxZoom) / 2
	midLon := (bbox.MinLon + bbox.MaxLon) / 2
	midLat := (bbox.MinLat + bbox.MaxLat) / 2
	cx, cy := lonLatToTileXY(midLon, midLat, midZ)
	tilePath := filepath.Join(tilesDir, strconv.Itoa(midZ), strconv.Itoa(cx), strconv.Itoa(cy)+".png")

	f, err := os.Open(tilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Tile is legitimately absent for edge AOIs; skip the pixel check.
			return nil
		}
		return domain.NewError(domain.ErrValidate, fmt.Sprintf("raster sanity: failed to open tile %q", tilePath), err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return domain.NewError(domain.ErrValidate, fmt.Sprintf("raster sanity: failed to decode tile %q", tilePath), err)
	}

	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				return nil // at least one non-transparent pixel found
			}
		}
	}
	return domain.NewError(domain.ErrValidate, "raster sanity: all sampled tile pixels are transparent (no valid terrain data)", nil)
}

func validatePMTilesMetadataConsistency(ctx context.Context, tc domain.TerrainToolchain, pmtilesPath string, manifest domain.TerrainManifest) error {
	tc = normalizeToolchain(tc)
	cmd := exec.CommandContext(ctx, tc.PMTilesBin, "show", "--header-json", pmtilesPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return domain.NewError(domain.ErrValidate, fmt.Sprintf("PMTiles metadata validation failed: %v: %s", err, strings.TrimSpace(string(out))), err)
	}

	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		return domain.NewError(domain.ErrValidate, "failed to parse PMTiles metadata JSON", err)
	}

	minZ, maxZ, ok := parseZoomFromPMTilesShow(doc)
	if !ok {
		return domain.NewError(domain.ErrValidate, "failed to read min/max zoom from PMTiles metadata", nil)
	}
	if minZ != manifest.MinZoom || maxZ != manifest.MaxZoom {
		return domain.NewError(domain.ErrValidate, fmt.Sprintf("PMTiles manifest zoom mismatch: header=%d-%d manifest=%d-%d", minZ, maxZ, manifest.MinZoom, manifest.MaxZoom), nil)
	}

	return nil
}

func parseZoomFromPMTilesShow(doc map[string]any) (int, int, bool) {
	minZoomKeys := []string{"min_zoom", "minZoom", "minzoom"}
	maxZoomKeys := []string{"max_zoom", "maxZoom", "maxzoom"}
	for _, minKey := range minZoomKeys {
		for _, maxKey := range maxZoomKeys {
			minV, minOK := asInt(doc[minKey])
			maxV, maxOK := asInt(doc[maxKey])
			if minOK && maxOK {
				return minV, maxV, true
			}
		}
	}

	headerAny, ok := doc["header"]
	if !ok {
		return 0, 0, false
	}
	header, ok := headerAny.(map[string]any)
	if !ok {
		return 0, 0, false
	}
	minV, minOK := asInt(header["min_zoom"])
	maxV, maxOK := asInt(header["max_zoom"])
	if !minOK || !maxOK {
		return 0, 0, false
	}
	return minV, maxV, true
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// runElevationChecks evaluates control-point RMSE against the DEM.
// Each point is queried in parallel (Issue #6) to avoid O(N) subprocess
// serial latency for large control-point sets.
func runElevationChecks(ctx context.Context, req domain.TerrainExportRequest, demPath string) (float64, int, error) {
	if strings.TrimSpace(req.ControlPointsPath) == "" {
		return 0, 0, nil
	}
	tc := normalizeToolchain(req.Toolchain)
	if _, err := exec.LookPath(tc.GDALLocationInfoBin); err != nil {
		return 0, 0, domain.NewError(domain.ErrValidate, fmt.Sprintf("terrain tool binary %q not found for elevation checks", tc.GDALLocationInfoBin), err)
	}

	f, err := os.Open(req.ControlPointsPath)
	if err != nil {
		return 0, 0, domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to open control points file %q", req.ControlPointsPath), err)
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReader(f))
	records, err := r.ReadAll()
	if err != nil {
		return 0, 0, domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to parse control points CSV %q", req.ControlPointsPath), err)
	}
	if len(records) <= 1 {
		return 0, 0, nil
	}

	type cpRow struct {
		lon, lat, expected float64
	}
	var points []cpRow
	for _, row := range records[1:] {
		if len(row) < 3 {
			continue
		}
		lon, err := strconv.ParseFloat(strings.TrimSpace(row[0]), 64)
		if err != nil {
			continue
		}
		lat, err := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		if err != nil {
			continue
		}
		expected, err := strconv.ParseFloat(strings.TrimSpace(row[2]), 64)
		if err != nil {
			continue
		}
		points = append(points, cpRow{lon, lat, expected})
	}
	if len(points) == 0 {
		return 0, 0, nil
	}

	type result struct {
		delta float64
		ok    bool
		err   error
	}
	results := make([]result, len(points))

	// Bound parallelism: each gdallocationinfo call loads the DEM into its own
	// GDAL context, so unconstrained concurrency would thrash I/O. Cap at 8.
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for idx, pt := range points {
		idx, pt := idx, pt
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			cmd := exec.CommandContext(ctx, tc.GDALLocationInfoBin, "-valonly", "-wgs84", demPath, formatFloat(pt.lon), formatFloat(pt.lat))
			out, err := cmd.CombinedOutput()
			if err != nil {
				results[idx] = result{err: domain.NewError(domain.ErrValidate, fmt.Sprintf("gdallocationinfo failed: %v: %s", err, strings.TrimSpace(string(out))), err)}
				return
			}
			actual, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
			if err != nil {
				return // skip unparseable output
			}
			delta := actual - pt.expected
			results[idx] = result{delta: delta, ok: true}
		}()
	}
	wg.Wait()

	var sumSq float64
	compared := 0
	for _, res := range results {
		if res.err != nil {
			return 0, compared, res.err
		}
		if res.ok {
			sumSq += res.delta * res.delta
			compared++
		}
	}

	if compared == 0 {
		return 0, 0, nil
	}
	return math.Sqrt(sumSq / float64(compared)), compared, nil
}

// coverageAndSeams performs a single pass over the expected tile range per zoom
// level, counting missing tiles and computing the maximum seam delta.
//
// Issue #3 — sliding row cache: instead of caching the entire zoom level (which
// can be thousands of images at high zooms), only two rows of decoded images are
// kept alive at a time: the "current" row (y) and the "next" row (y+1). Once
// the loop advances past a row, its images are discarded. Memory usage is
// O(width × 2) decoded images per zoom level rather than O(width × height).
func coverageAndSeams(ctx context.Context, root string, bbox domain.BoundingBox, minZoom, maxZoom int, clipPolygonPath string, tc domain.TerrainToolchain) (int, uint8, error) {
	totalMissing := 0
	var globalMaxDelta uint8

	for z := minZoom; z <= maxZoom; z++ {
		minX, maxX, minY, maxY := tileRangeForBBox(bbox, z)

		// Sliding two-row cache. Keys are x-coordinate integers.
		// row[0] = images for the row currently being processed (y).
		// row[1] = images for the next row (y+1), pre-loaded for bottom-seam checks.
		rowCache := [2]map[int]image.Image{
			make(map[int]image.Image, maxX-minX+2),
			make(map[int]image.Image, maxX-minX+2),
		}

		loadRow := func(row map[int]image.Image, y int) error {
			for x := minX; x <= maxX+1; x++ { // +1 to cover right-seam neighbours
				if _, ok := row[x]; ok {
					continue
				}
				path := filepath.Join(root, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+".png")
				fi, err := os.Stat(path)
				if err != nil || fi == nil {
					row[x] = nil // absent
					continue
				}
				b, err := os.ReadFile(path)
				if err != nil {
					return domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to read tile %q", path), err)
				}
				img, err := png.Decode(bytes.NewReader(b))
				if err != nil {
					return domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to decode tile PNG %q", path), err)
				}
				row[x] = img
			}
			return nil
		}

		for y := minY; y <= maxY; y++ {
			// Ensure current row is loaded.
			if err := loadRow(rowCache[0], y); err != nil {
				return 0, 0, err
			}
			// Pre-load next row for bottom-seam checks (only if within range).
			if y < maxY {
				if err := loadRow(rowCache[1], y+1); err != nil {
					return 0, 0, err
				}
			}

			for x := minX; x <= maxX; x++ {
				if clipPolygonPath != "" {
					tMinLon, tMinLat, tMaxLon, tMaxLat := tileToWGS84Bounds(x, y, z)
					intersects, err := tileIntersectsPolygon(ctx, tc, clipPolygonPath, tMinLon, tMinLat, tMaxLon, tMaxLat)
					if err != nil {
						return 0, 0, err
					}
					if !intersects {
						continue
					}
				}

				// Coverage check.
				tilePath := filepath.Join(root, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+".png")
				if _, err := os.Stat(tilePath); err != nil {
					if os.IsNotExist(err) {
						totalMissing++
						continue
					}
					return 0, 0, domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to stat tile %q", tilePath), err)
				}

				aImg := rowCache[0][x]
				if aImg == nil {
					continue
				}

				// Right-seam check (x → x+1, same row).
				if x < maxX {
					bImg := rowCache[0][x+1]
					if bImg != nil {
						if d, ok, err := seamDeltaFromImages(aImg, bImg, true); err != nil {
							return 0, 0, err
						} else if ok && d > globalMaxDelta {
							globalMaxDelta = d
						}
					}
				}

				// Bottom-seam check (y → y+1, same column).
				if y < maxY {
					bImg := rowCache[1][x]
					if bImg != nil {
						if d, ok, err := seamDeltaFromImages(aImg, bImg, false); err != nil {
							return 0, 0, err
						} else if ok && d > globalMaxDelta {
							globalMaxDelta = d
						}
					}
				}
			}

			// Slide the cache: promote row[1] → row[0], reset row[1].
			rowCache[0] = rowCache[1]
			rowCache[1] = make(map[int]image.Image, maxX-minX+2)
		}
	}

	return totalMissing, globalMaxDelta, nil
}

// seamDeltaFromImages computes the 95th-percentile seam delta between two
// already-decoded images. vertical=true means a left→right border.
func seamDeltaFromImages(aImg, bImg image.Image, vertical bool) (uint8, bool, error) {
	aBounds := aImg.Bounds()
	bBounds := bImg.Bounds()
	if aBounds.Dx() != bBounds.Dx() || aBounds.Dy() != bBounds.Dy() {
		return 0, false, domain.NewError(domain.ErrValidate, "tile dimensions mismatch in seam check", nil)
	}

	deltas := make([]uint8, 0, 256)
	if vertical {
		xA := aBounds.Max.X - 1
		xB := bBounds.Min.X
		xAInner := xA - 1
		xBInner := xB + 1
		if xAInner < aBounds.Min.X || xBInner >= bBounds.Max.X {
			return 0, false, nil
		}
		for y := aBounds.Min.Y; y < aBounds.Max.Y; y++ {
			d, ok := terrariumSeamDelta(
				aImg.At(xA, y),
				bImg.At(xB, y),
				aImg.At(xAInner, y),
				bImg.At(xBInner, y),
			)
			if ok {
				deltas = append(deltas, d)
			}
		}
	} else {
		yA := aBounds.Max.Y - 1
		yB := bBounds.Min.Y
		yAInner := yA - 1
		yBInner := yB + 1
		if yAInner < aBounds.Min.Y || yBInner >= bBounds.Max.Y {
			return 0, false, nil
		}
		for x := aBounds.Min.X; x < aBounds.Max.X; x++ {
			d, ok := terrariumSeamDelta(
				aImg.At(x, yA),
				bImg.At(x, yB),
				aImg.At(x, yAInner),
				bImg.At(x, yBInner),
			)
			if ok {
				deltas = append(deltas, d)
			}
		}
	}

	if len(deltas) < 8 {
		return 0, false, nil
	}
	return percentileUint8(deltas, 0.95), true, nil
}

func terrariumSeamDelta(aEdge, bEdge, aInner, bInner color.Color) (uint8, bool) {
	aElev, okAEdge := terrariumToElevation(aEdge)
	bElev, okBEdge := terrariumToElevation(bEdge)
	aInnerElev, okAInner := terrariumToElevation(aInner)
	bInnerElev, okBInner := terrariumToElevation(bInner)

	if !okAEdge || !okBEdge || !okAInner || !okBInner {
		return 0, false
	}

	// Elevation jump across the tile boundary.
	cross := math.Abs(aElev - bElev)

	// Maximum local terrain gradient on either side of the boundary.
	// Adjacent edge pixels in a nearest-neighbour tile pyramid represent
	// adjacent source samples, so a gradient of this magnitude is expected
	// even on perfectly seamless tiles.
	leftGradient := math.Abs(aElev - aInnerElev)
	rightGradient := math.Abs(bInnerElev - bElev)
	maxLocalGrad := math.Max(leftGradient, rightGradient)

	// The seam anomaly is the portion of the cross-edge jump that cannot be
	// explained by the local terrain slope. Anything within the local gradient
	// is expected; only the excess is anomalous.
	excess := cross - maxLocalGrad
	if excess < 0 {
		excess = 0
	}
	if excess > 255 {
		excess = 255
	}
	return uint8(math.Round(excess)), true
}

func terrariumToElevation(c color.Color) (float64, bool) {
	r16, g16, b16, a16 := c.RGBA()
	if a16 < 0xFFFF {
		return 0, false
	}

	r := float64(r16 >> 8)
	g := float64(g16 >> 8)
	b := float64(b16 >> 8)
	elev := (r*256.0 + g + b/256.0) - 32768.0
	if elev <= -32767.5 {
		return 0, false
	}

	return elev, true
}

func percentileUint8(values []uint8, p float64) uint8 {
	if len(values) == 0 {
		return 0
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	idx := int(math.Ceil(p*float64(len(values)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func tileRangeForBBox(bbox domain.BoundingBox, z int) (int, int, int, int) {
	minX, maxY := lonLatToTileXY(bbox.MinLon, bbox.MinLat, z)
	maxX, minY := lonLatToTileXY(bbox.MaxLon, bbox.MaxLat, z)

	if minX > maxX {
		minX, maxX = maxX, minX
	}
	if minY > maxY {
		minY, maxY = maxY, minY
	}

	return minX, maxX, minY, maxY
}

func lonLatToTileXY(lon, lat float64, z int) (int, int) {
	lat = math.Max(math.Min(lat, 85.05112878), -85.05112878)
	n := math.Exp2(float64(z))
	x := int(math.Floor((lon + 180.0) / 360.0 * n))
	latRad := lat * math.Pi / 180.0
	y := int(math.Floor((1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n))

	maxTile := int(n) - 1
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x > maxTile {
		x = maxTile
	}
	if y > maxTile {
		y = maxTile
	}

	return x, y
}
