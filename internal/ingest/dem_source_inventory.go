// Package ingest reads and parses input sources.
//
// Author: Miroslav Pasek
package ingest

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// DEMSourceIngestor loads and validates source DEM files.
type DEMSourceIngestor interface {
	Ingest(ctx context.Context, sourceDir, checksumsPath string, aoi domain.BoundingBox) (domain.DEMSourceInventory, error)
}

// FileDEMSourcesIngestor reads DEM files from the filesystem.
type FileDEMSourcesIngestor struct{}

// copDEMPattern matches Copernicus DEM filenames that encode tile bounds, e.g.
// Copernicus_DSM_10_N48_00_E016_00_DEM.tif
// Captured groups: lat hemisphere (1), lat (2), lon hemisphere (3), lon (4).
var copDEMPattern = regexp.MustCompile(`(?i)_([NS])(\d+)_\d+_([EW])(\d+)_\d+_`)

// boundsFromFilename attempts to derive the 1-degree tile bounding box from a
// Copernicus DEM filename. Returns (bbox, true) on success or (_, false) when
// the filename does not match the expected convention.
func boundsFromFilename(name string) (domain.BoundingBox, bool) {
	m := copDEMPattern.FindStringSubmatch(name)
	if m == nil {
		return domain.BoundingBox{}, false
	}
	latVal, err1 := strconv.ParseFloat(m[2], 64)
	lonVal, err2 := strconv.ParseFloat(m[4], 64)
	if err1 != nil || err2 != nil {
		return domain.BoundingBox{}, false
	}
	if strings.EqualFold(m[1], "S") {
		latVal = -latVal
	}
	if strings.EqualFold(m[3], "W") {
		lonVal = -lonVal
	}
	// Each Copernicus tile covers exactly 1 degree in both axes.
	return domain.BoundingBox{
		MinLon: lonVal,
		MinLat: latVal,
		MaxLon: lonVal + 1,
		MaxLat: latVal + 1,
	}, true
}

// gdalinfoBounds calls gdalinfo -json and extracts the raster's WGS84 extent.
func gdalinfoBounds(ctx context.Context, path string) (domain.BoundingBox, error) {
	cmd := exec.CommandContext(ctx, "gdalinfo", "-json", path)
	out, err := cmd.Output()
	if err != nil {
		return domain.BoundingBox{}, fmt.Errorf("gdalinfo failed for %q: %w", path, err)
	}

	var doc struct {
		WGS84Extent struct {
			Coordinates [][][2]float64 `json:"coordinates"`
		} `json:"wgs84Extent"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return domain.BoundingBox{}, fmt.Errorf("gdalinfo JSON parse failed for %q: %w", path, err)
	}

	coords := doc.WGS84Extent.Coordinates
	if len(coords) == 0 || len(coords[0]) == 0 {
		return domain.BoundingBox{}, fmt.Errorf("gdalinfo returned empty wgs84Extent for %q", path)
	}

	bbox := domain.BoundingBox{
		MinLon: coords[0][0][0],
		MinLat: coords[0][0][1],
		MaxLon: coords[0][0][0],
		MaxLat: coords[0][0][1],
	}
	for _, ring := range coords {
		for _, pt := range ring {
			if pt[0] < bbox.MinLon {
				bbox.MinLon = pt[0]
			}
			if pt[0] > bbox.MaxLon {
				bbox.MaxLon = pt[0]
			}
			if pt[1] < bbox.MinLat {
				bbox.MinLat = pt[1]
			}
			if pt[1] > bbox.MaxLat {
				bbox.MaxLat = pt[1]
			}
		}
	}
	return bbox, nil
}

// intersects returns true when bbox a and b overlap (touching edges count).
func intersects(a, b domain.BoundingBox) bool {
	return a.MinLon <= b.MaxLon && a.MaxLon >= b.MinLon &&
		a.MinLat <= b.MaxLat && a.MaxLat >= b.MinLat
}

// Ingest scans DEM files, filters to AOI, validates integrity, and returns a
// deterministic inventory. SHA-256 is only computed when a checksums file is
// provided; otherwise the field is left empty.
func (i FileDEMSourcesIngestor) Ingest(ctx context.Context, sourceDir, checksumsPath string, aoi domain.BoundingBox) (domain.DEMSourceInventory, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to read DEM source dir %q", sourceDir), err)
	}

	var candidates []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		nameLower := strings.ToLower(entry.Name())
		if strings.HasSuffix(nameLower, ".tif") || strings.HasSuffix(nameLower, ".tiff") {
			candidates = append(candidates, filepath.Join(sourceDir, entry.Name()))
		}
	}

	sort.Strings(candidates)
	if len(candidates) == 0 {
		return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, "no Copernicus DEM source files (*.tif/*.tiff) found", nil)
	}

	// Filter to only files whose bounding box intersects the AOI.
	// Files whose names follow the Copernicus convention are resolved without
	// any I/O. For the remainder, gdalinfo is called in parallel (Issue #7)
	// to avoid O(N) serial subprocess latency on large mixed-naming directories.
	type filterResult struct {
		path string
		keep bool
		err  error
	}
	results := make([]filterResult, len(candidates))

	// Separate fast (filename) and slow (gdalinfo) paths.
	type slowWork struct {
		idx  int
		path string
	}
	var slow []slowWork
	for idx, absPath := range candidates {
		if bbox, ok := boundsFromFilename(filepath.Base(absPath)); ok {
			results[idx] = filterResult{path: absPath, keep: intersects(bbox, aoi)}
		} else {
			slow = append(slow, slowWork{idx, absPath})
		}
	}

	if len(slow) > 0 {
		// Fan-out gdalinfo calls, capped at 8 concurrent processes.
		type gdalResult struct {
			idx  int
			bbox domain.BoundingBox
			err  error
		}
		ch := make(chan gdalResult, len(slow))
		sem := make(chan struct{}, 8)
		for _, w := range slow {
			w := w
			sem <- struct{}{}
			go func() {
				defer func() { <-sem }()
				bbox, err := gdalinfoBounds(ctx, w.path)
				ch <- gdalResult{w.idx, bbox, err}
			}()
		}
		// Drain exactly len(slow) results.
		for range slow {
			r := <-ch
			if r.err != nil {
				results[r.idx] = filterResult{err: domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to determine bounds for DEM file %q", candidates[r.idx]), r.err)}
			} else {
				results[r.idx] = filterResult{path: candidates[r.idx], keep: intersects(r.bbox, aoi)}
			}
		}
	}

	var files []string
	for _, res := range results {
		if res.err != nil {
			return domain.DEMSourceInventory{}, res.err
		}
		if res.keep {
			files = append(files, res.path)
		}
	}
	sort.Strings(files)

	if len(files) == 0 {
		return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, "no DEM source files intersect the requested AOI", nil)
	}

	expectedChecksums, err := parseChecksumFile(checksumsPath)
	if err != nil {
		return domain.DEMSourceInventory{}, err
	}

	needChecksum := len(expectedChecksums) > 0

	inventory := domain.DEMSourceInventory{Files: make([]domain.DEMSourceFile, 0, len(files))}
	for _, absPath := range files {
		stat, err := os.Stat(absPath)
		if err != nil {
			return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to stat DEM source file %q", absPath), err)
		}

		rel := filepath.Base(absPath)

		var checksum string
		if needChecksum {
			checksum, err = sha256File(absPath)
			if err != nil {
				return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to checksum DEM source file %q", absPath), err)
			}

			expected, ok := expectedChecksums[rel]
			if !ok {
				return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("checksum entry missing for source file %q", rel), nil)
			}
			if !strings.EqualFold(expected, checksum) {
				return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("checksum mismatch for source file %q", rel), nil)
			}
		}

		inventory.Files = append(inventory.Files, domain.DEMSourceFile{
			Path:           absPath,
			RelativePath:   rel,
			SizeBytes:      stat.Size(),
			SHA256Checksum: checksum,
		})
	}

	if needChecksum && len(expectedChecksums) > len(inventory.Files) {
		for filename := range expectedChecksums {
			if !hasSourceFile(inventory.Files, filename) {
				return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("checksum references missing source file %q", filename), nil)
			}
		}
	}

	return inventory, nil
}

func hasSourceFile(files []domain.DEMSourceFile, filename string) bool {
	for _, file := range files {
		if file.RelativePath == filename {
			return true
		}
	}
	return false
}

func parseChecksumFile(path string) (map[string]string, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]string{}, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to open checksum file %q", path), err)
	}
	defer f.Close()

	checksums := map[string]string{}
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		parts := strings.Fields(raw)
		if len(parts) < 2 {
			return nil, domain.NewError(domain.ErrIngest, fmt.Sprintf("invalid checksum line %d in %q", line, path), nil)
		}
		checksum := strings.ToLower(strings.TrimSpace(parts[0]))
		filename := strings.TrimSpace(parts[len(parts)-1])
		if checksum == "" || filename == "" {
			return nil, domain.NewError(domain.ErrIngest, fmt.Sprintf("invalid checksum line %d in %q", line, path), nil)
		}
		checksums[filename] = checksum
	}
	if err := scanner.Err(); err != nil {
		return nil, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed reading checksum file %q", path), err)
	}

	return checksums, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
