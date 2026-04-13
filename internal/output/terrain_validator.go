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
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// TerrainValidator executes quality gates and returns validation metrics.
type TerrainValidator interface {
	Validate(ctx context.Context, req domain.TerrainExportRequest, artifacts domain.TerrainBuildArtifacts, manifest domain.TerrainManifest) (domain.TerrainValidationResult, error)
}

// DefaultTerrainValidator validates coverage, seam quality, elevation checks, and metadata consistency.
type DefaultTerrainValidator struct{}

func (v DefaultTerrainValidator) Validate(ctx context.Context, req domain.TerrainExportRequest, artifacts domain.TerrainBuildArtifacts, manifest domain.TerrainManifest) (domain.TerrainValidationResult, error) {
	missing, err := countMissingTiles(artifacts.TilesDir, req.AOIBounds, req.MinZoom, req.MaxZoom)
	if err != nil {
		return domain.TerrainValidationResult{}, err
	}
	if missing > 0 {
		return domain.TerrainValidationResult{}, domain.NewError(domain.ErrValidate, fmt.Sprintf("terrain coverage validation failed: %d missing tiles", missing), nil)
	}

	maxSeamDelta, err := computeMaxSeamDelta(artifacts.TilesDir, req.AOIBounds, req.MinZoom, req.MaxZoom)
	if err != nil {
		return domain.TerrainValidationResult{}, err
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

	if err := validateHillshadeSanity(ctx, req.Toolchain, artifacts.HillshadePath); err != nil {
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
		HillshadeOK:           true,
		MetadataConsistencyOK: true,
	}, nil
}

func validateHillshadeSanity(ctx context.Context, tc domain.TerrainToolchain, hillshadePath string) error {
	tc = normalizeToolchain(tc)
	if _, err := exec.LookPath(tc.GDALInfoBin); err != nil {
		return domain.NewError(domain.ErrValidate, fmt.Sprintf("terrain tool binary %q not found for hillshade validation", tc.GDALInfoBin), err)
	}
	cmd := exec.CommandContext(ctx, tc.GDALInfoBin, "-stats", hillshadePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return domain.NewError(domain.ErrValidate, fmt.Sprintf("hillshade sanity validation failed: %v: %s", err, strings.TrimSpace(string(out))), err)
	}
	text := string(out)
	if !strings.Contains(text, "STATISTICS_MINIMUM") || !strings.Contains(text, "STATISTICS_MAXIMUM") {
		return domain.NewError(domain.ErrValidate, "hillshade sanity validation failed: missing stats in gdalinfo output", nil)
	}
	return nil
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

	var sumSq float64
	compared := 0
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

		cmd := exec.CommandContext(ctx, tc.GDALLocationInfoBin, "-valonly", "-wgs84", demPath, formatFloat(lon), formatFloat(lat))
		out, err := cmd.CombinedOutput()
		if err != nil {
			return 0, compared, domain.NewError(domain.ErrValidate, fmt.Sprintf("gdallocationinfo failed: %v: %s", err, strings.TrimSpace(string(out))), err)
		}
		actual, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		if err != nil {
			continue
		}

		delta := actual - expected
		sumSq += delta * delta
		compared++
	}

	if compared == 0 {
		return 0, 0, nil
	}
	return math.Sqrt(sumSq / float64(compared)), compared, nil
}

func countMissingTiles(root string, bbox domain.BoundingBox, minZoom, maxZoom int) (int, error) {
	missing := 0
	for z := minZoom; z <= maxZoom; z++ {
		minX, maxX, minY, maxY := tileRangeForBBox(bbox, z)
		for x := minX; x <= maxX; x++ {
			for y := minY; y <= maxY; y++ {
				tilePath := filepath.Join(root, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+".png")
				if _, err := os.Stat(tilePath); err != nil {
					if os.IsNotExist(err) {
						missing++
						continue
					}
					return 0, domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to stat tile %q", tilePath), err)
				}
			}
		}
	}
	return missing, nil
}

func computeMaxSeamDelta(root string, bbox domain.BoundingBox, minZoom, maxZoom int) (uint8, error) {
	var maxDelta uint8
	for z := minZoom; z <= maxZoom; z++ {
		minX, maxX, minY, maxY := tileRangeForBBox(bbox, z)
		for x := minX; x <= maxX; x++ {
			for y := minY; y <= maxY; y++ {
				current := filepath.Join(root, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+".png")
				right := filepath.Join(root, strconv.Itoa(z), strconv.Itoa(x+1), strconv.Itoa(y)+".png")
				bottom := filepath.Join(root, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y+1)+".png")

				if d, ok, err := edgeDelta(current, right, true); err != nil {
					return 0, err
				} else if ok && d > maxDelta {
					maxDelta = d
				}

				if d, ok, err := edgeDelta(current, bottom, false); err != nil {
					return 0, err
				} else if ok && d > maxDelta {
					maxDelta = d
				}
			}
		}
	}
	return maxDelta, nil
}

func edgeDelta(aPath, bPath string, vertical bool) (uint8, bool, error) {
	if _, err := os.Stat(aPath); err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to stat tile %q", aPath), err)
	}
	if _, err := os.Stat(bPath); err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to stat tile %q", bPath), err)
	}

	aBytes, err := os.ReadFile(aPath)
	if err != nil {
		return 0, false, domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to read tile %q", aPath), err)
	}
	bBytes, err := os.ReadFile(bPath)
	if err != nil {
		return 0, false, domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to read tile %q", bPath), err)
	}

	aImg, err := png.Decode(bytes.NewReader(aBytes))
	if err != nil {
		return 0, false, domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to decode tile PNG %q", aPath), err)
	}
	bImg, err := png.Decode(bytes.NewReader(bBytes))
	if err != nil {
		return 0, false, domain.NewError(domain.ErrValidate, fmt.Sprintf("failed to decode tile PNG %q", bPath), err)
	}

	aBounds := aImg.Bounds()
	bBounds := bImg.Bounds()
	if aBounds.Dx() != bBounds.Dx() || aBounds.Dy() != bBounds.Dy() {
		return 0, false, domain.NewError(domain.ErrValidate, fmt.Sprintf("tile dimensions mismatch between %q and %q", aPath, bPath), nil)
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
			if !ok {
				continue
			}
			deltas = append(deltas, d)
		}
		if len(deltas) < 8 {
			return 0, false, nil
		}
		return percentileUint8(deltas, 0.95), true, nil
	}

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
		if !ok {
			continue
		}
		deltas = append(deltas, d)
	}
	if len(deltas) < 8 {
		return 0, false, nil
	}
	return percentileUint8(deltas, 0.95), true, nil
}

func terrariumSeamDelta(aEdge, bEdge, aInner, bInner color.Color) (uint8, bool) {
	aInnerElev, okAInner := terrariumToElevation(aInner)
	bInnerElev, okBInner := terrariumToElevation(bInner)
	aElev, okAEdge := terrariumToElevation(aEdge)
	bElev, okBEdge := terrariumToElevation(bEdge)

	if !okAInner || !okBInner || !okAEdge || !okBEdge {
		return 0, false
	}

	cross := math.Abs(aElev - bElev)
	leftGradient := math.Abs(aElev - aInnerElev)
	rightGradient := math.Abs(bInnerElev - bElev)

	// Compare gradient mismatch across tile boundaries rather than raw edge
	// value differences. Raw cross-edge values naturally diverge on slopes
	// because adjacent tile edge pixels represent different sample positions.
	meters := math.Abs(leftGradient - rightGradient)

	// If the cross-edge jump is already smaller than local gradients, treat it as
	// continuous and force seam delta to zero.
	if cross <= math.Max(leftGradient, rightGradient) {
		meters = 0
	}
	if meters > 255 {
		meters = 255
	}
	return uint8(math.Round(meters)), true
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
