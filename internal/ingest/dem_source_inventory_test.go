// Package ingest reads and parses input sources.
package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// TestBoundsFromFilename verifies that well-formed Copernicus DEM filenames
// are parsed correctly and that malformed names return (_, false).
func TestBoundsFromFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantOK  bool
		wantBBx domain.BoundingBox
	}{
		{
			name:    "Copernicus_DSM_10_N48_00_E016_00_DEM.tif",
			wantOK:  true,
			wantBBx: domain.BoundingBox{MinLon: 16, MinLat: 48, MaxLon: 17, MaxLat: 49},
		},
		{
			name:    "Copernicus_DSM_10_S03_00_W060_00_DEM.tif",
			wantOK:  true,
			wantBBx: domain.BoundingBox{MinLon: -60, MinLat: -3, MaxLon: -59, MaxLat: -2},
		},
		{
			name:    "Copernicus_DSM_10_N00_00_E000_00_DEM.tif",
			wantOK:  true,
			wantBBx: domain.BoundingBox{MinLon: 0, MinLat: 0, MaxLon: 1, MaxLat: 1},
		},
		{
			name:   "random_elevation_file.tif",
			wantOK: false,
		},
		{
			name:   "N48E016.tif",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bbox, ok := boundsFromFilename(tc.name)
			if ok != tc.wantOK {
				t.Fatalf("boundsFromFilename(%q) ok=%v, want %v", tc.name, ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if bbox != tc.wantBBx {
				t.Fatalf("boundsFromFilename(%q) bbox=%+v, want %+v", tc.name, bbox, tc.wantBBx)
			}
		})
	}
}

// TestIntersects verifies axis-aligned bbox overlap logic.
func TestIntersects(t *testing.T) {
	t.Parallel()

	aoi := domain.BoundingBox{MinLon: 15, MinLat: 47, MaxLon: 17, MaxLat: 49}

	tests := []struct {
		desc string
		b    domain.BoundingBox
		want bool
	}{
		{"fully inside", domain.BoundingBox{16, 48, 16.5, 48.5}, true},
		{"partial overlap west", domain.BoundingBox{14, 48, 16, 48.5}, true},
		{"touching east edge", domain.BoundingBox{17, 48, 18, 49}, true},
		{"touching north edge", domain.BoundingBox{16, 49, 17, 50}, true},
		{"entirely west", domain.BoundingBox{10, 48, 14.9, 48.9}, false},
		{"entirely east", domain.BoundingBox{17.1, 48, 18, 49}, false},
		{"entirely south", domain.BoundingBox{16, 45, 17, 46.9}, false},
		{"entirely north", domain.BoundingBox{16, 49.1, 17, 50}, false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			if got := intersects(aoi, tc.b); got != tc.want {
				t.Fatalf("intersects(aoi, %+v) = %v, want %v", tc.b, got, tc.want)
			}
		})
	}
}

// TestFileDEMSourcesIngestorAOIFilter verifies that only files whose names
// encode coordinates intersecting the AOI are returned, and that files outside
// the AOI are silently discarded.
func TestFileDEMSourcesIngestorAOIFilter(t *testing.T) {
	t.Parallel()

	// Tiles: N48/E016 is inside the AOI; N60/E000 is outside.
	tiles := []string{
		"Copernicus_DSM_10_N48_00_E016_00_DEM.tif",
		"Copernicus_DSM_10_N60_00_E000_00_DEM.tif",
	}

	dir := t.TempDir()
	for _, name := range tiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0o600); err != nil {
			t.Fatalf("write fixture %q: %v", name, err)
		}
	}

	aoi := domain.BoundingBox{MinLon: 15, MinLat: 47, MaxLon: 17, MaxLat: 49}
	inv, err := FileDEMSourcesIngestor{}.Ingest(context.Background(), dir, "", aoi)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if len(inv.Files) != 1 {
		t.Fatalf("expected 1 file in inventory, got %d", len(inv.Files))
	}
	if inv.Files[0].RelativePath != "Copernicus_DSM_10_N48_00_E016_00_DEM.tif" {
		t.Fatalf("unexpected file in inventory: %q", inv.Files[0].RelativePath)
	}
}

// TestFileDEMSourcesIngestorAllFilteredOut verifies that an error is returned
// when no DEM files intersect the AOI.
func TestFileDEMSourcesIngestorAllFilteredOut(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "Copernicus_DSM_10_N60_00_E000_00_DEM.tif"),
		[]byte("fake"), 0o600,
	); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	aoi := domain.BoundingBox{MinLon: 15, MinLat: 47, MaxLon: 17, MaxLat: 49}
	_, err := FileDEMSourcesIngestor{}.Ingest(context.Background(), dir, "", aoi)
	if err == nil {
		t.Fatal("expected error when no files intersect AOI, got nil")
	}
	if !strings.Contains(err.Error(), "no DEM source files intersect") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestFileDEMSourcesIngestorLazyHash verifies that SHA-256 is not computed
// when no checksums file is provided (lazy hashing, Issue #7).
func TestFileDEMSourcesIngestorLazyHash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "Copernicus_DSM_10_N48_00_E016_00_DEM.tif"),
		[]byte("fake-dem-data"), 0o600,
	); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	aoi := domain.BoundingBox{MinLon: 15, MinLat: 47, MaxLon: 17, MaxLat: 49}
	inv, err := FileDEMSourcesIngestor{}.Ingest(context.Background(), dir, "", aoi)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	if len(inv.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(inv.Files))
	}
	if inv.Files[0].SHA256Checksum != "" {
		t.Fatalf("expected empty checksum when no checksums file provided, got %q", inv.Files[0].SHA256Checksum)
	}
}

// TestFileDEMSourcesIngestorChecksumVerification verifies that checksums are
// computed and validated when a checksums file is provided.
func TestFileDEMSourcesIngestorChecksumVerification(t *testing.T) {
	t.Parallel()

	const content = "fake-dem-data"
	filename := "Copernicus_DSM_10_N48_00_E016_00_DEM.tif"

	dir := t.TempDir()
	tilePath := filepath.Join(dir, filename)
	if err := os.WriteFile(tilePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Compute the real checksum.
	checksum, err := sha256File(tilePath)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}

	checksumFile := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksumFile, []byte(fmt.Sprintf("%s  %s\n", checksum, filename)), 0o600); err != nil {
		t.Fatalf("write checksums file: %v", err)
	}

	aoi := domain.BoundingBox{MinLon: 15, MinLat: 47, MaxLon: 17, MaxLat: 49}
	inv, err := FileDEMSourcesIngestor{}.Ingest(context.Background(), dir, checksumFile, aoi)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	if len(inv.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(inv.Files))
	}
	if inv.Files[0].SHA256Checksum != checksum {
		t.Fatalf("expected checksum %q, got %q", checksum, inv.Files[0].SHA256Checksum)
	}
}

// TestFileDEMSourcesIngestorChecksumMismatch verifies that a checksum mismatch
// returns a typed error.
func TestFileDEMSourcesIngestorChecksumMismatch(t *testing.T) {
	t.Parallel()

	filename := "Copernicus_DSM_10_N48_00_E016_00_DEM.tif"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("data"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	checksumFile := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksumFile,
		[]byte(fmt.Sprintf("%s  %s\n", strings.Repeat("0", 64), filename)), 0o600,
	); err != nil {
		t.Fatalf("write checksums file: %v", err)
	}

	aoi := domain.BoundingBox{MinLon: 15, MinLat: 47, MaxLon: 17, MaxLat: 49}
	_, err := FileDEMSourcesIngestor{}.Ingest(context.Background(), dir, checksumFile, aoi)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
