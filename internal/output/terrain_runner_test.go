// Package output validates and serializes custom XML output.
package output

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// ---- wgs84ToTileXY ----------------------------------------------------------

func TestWGS84ToTileXY_KnownValues(t *testing.T) {
	tests := []struct {
		lon, lat     float64
		z            int
		wantX, wantY int
	}{
		// zoom 0: whole world is one tile.
		{0, 0, 0, 0, 0},
		// zoom 1: four tiles, origin is top-left (NW quadrant).
		{-90, 45, 1, 0, 0},  // NW
		{90, 45, 1, 1, 0},   // NE
		{-90, -45, 1, 0, 1}, // SW
		{90, -45, 1, 1, 1},  // SE
		// Prague approx lon=14.42, lat=50.08 at zoom 6 → tile 34,21.
		{14.42, 50.08, 6, 34, 21},
	}

	for _, tc := range tests {
		x, y := wgs84ToTileXY(tc.lon, tc.lat, tc.z)
		if x != tc.wantX || y != tc.wantY {
			t.Errorf("wgs84ToTileXY(%.2f, %.2f, %d) = (%d,%d), want (%d,%d)",
				tc.lon, tc.lat, tc.z, x, y, tc.wantX, tc.wantY)
		}
	}
}

func TestWGS84ToTileXY_Clamping(t *testing.T) {
	// Extremely high latitude is clamped to the Mercator limit.
	x, y := wgs84ToTileXY(0, 90, 4)
	n := int(1) << 4
	if x < 0 || x >= n || y < 0 || y >= n {
		t.Errorf("clamped tile (%d,%d) out of bounds for z=4", x, y)
	}

	// Extremely low latitude similarly clamped.
	x, y = wgs84ToTileXY(0, -90, 4)
	if x < 0 || x >= n || y < 0 || y >= n {
		t.Errorf("clamped tile (%d,%d) out of bounds for z=4 (south pole)", x, y)
	}
}

// ---- tileToWGS84Bounds -------------------------------------------------------

func TestTileToWGS84Bounds_Roundtrip(t *testing.T) {
	// For a known tile, the WGS-84 bounds should contain the tile's centre.
	z, x, y := 6, 34, 21
	minLon, minLat, maxLon, maxLat := tileToWGS84Bounds(x, y, z)

	if minLon >= maxLon {
		t.Errorf("minLon %.4f >= maxLon %.4f", minLon, maxLon)
	}
	if minLat >= maxLat {
		t.Errorf("minLat %.4f >= maxLat %.4f", minLat, maxLat)
	}

	// Prague should be inside the bounds of its tile.
	pragueLon, pragueLat := 14.42, 50.08
	if pragueLon < minLon || pragueLon > maxLon {
		t.Errorf("Prague lon %.4f not inside tile lon [%.4f, %.4f]", pragueLon, minLon, maxLon)
	}
	if pragueLat < minLat || pragueLat > maxLat {
		t.Errorf("Prague lat %.4f not inside tile lat [%.4f, %.4f]", pragueLat, minLat, maxLat)
	}
}

func TestTileToWGS84Bounds_ZoomZero(t *testing.T) {
	// At zoom 0, tile 0/0 covers the whole Mercator world.
	minLon, minLat, maxLon, maxLat := tileToWGS84Bounds(0, 0, 0)
	if math.Abs(minLon-(-180)) > 0.001 {
		t.Errorf("expected minLon≈-180, got %.4f", minLon)
	}
	if math.Abs(maxLon-180) > 0.001 {
		t.Errorf("expected maxLon≈180, got %.4f", maxLon)
	}
	if minLat >= maxLat {
		t.Errorf("expected minLat < maxLat at z0, got %.4f, %.4f", minLat, maxLat)
	}
}

// ---- tileRangeForClip --------------------------------------------------------

func TestTileRangeForClip_CZBbox(t *testing.T) {
	// CZ bounding box: 12.09,48.55,18.87,51.06
	bbox := domain.BoundingBox{MinLon: 12.09, MinLat: 48.55, MaxLon: 18.87, MaxLat: 51.06}

	for _, z := range []int{5, 6, 7, 8, 9, 10} {
		minX, maxX, minY, maxY := tileRangeForClip(bbox, z)
		if minX > maxX {
			t.Errorf("z=%d: minX=%d > maxX=%d", z, minX, maxX)
		}
		if minY > maxY {
			t.Errorf("z=%d: minY=%d > maxY=%d", z, minY, maxY)
		}

		// Derived tile count should be positive.
		count := (maxX - minX + 1) * (maxY - minY + 1)
		if count <= 0 {
			t.Errorf("z=%d: tile count=%d, want > 0", z, count)
		}
	}
}

func TestTileRangeForClip_NormalOrder(t *testing.T) {
	// Even if lon/lat order from wgs84ToTileXY is swapped, results are normalised.
	bbox := domain.BoundingBox{MinLon: -10, MinLat: 35, MaxLon: 30, MaxLat: 70}
	minX, maxX, minY, maxY := tileRangeForClip(bbox, 6)
	if minX > maxX {
		t.Errorf("minX=%d > maxX=%d after normalisation", minX, maxX)
	}
	if minY > maxY {
		t.Errorf("minY=%d > maxY=%d after normalisation", minY, maxY)
	}
}

// ---- tileToWGS84Bounds / wgs84ToTileXY cross-check --------------------------

func TestTileContainsItsOwnCentre(t *testing.T) {
	// Verify that wgs84ToTileXY(centre of tile x,y,z) == (x,y).
	// Test a handful of CZ-region tiles at zoom 8.
	z := 8
	tests := []struct{ x, y int }{{136, 85}, {137, 86}, {140, 87}, {141, 88}}

	for _, tc := range tests {
		minLon, minLat, maxLon, maxLat := tileToWGS84Bounds(tc.x, tc.y, z)
		centreLon := (minLon + maxLon) / 2
		centreLat := (minLat + maxLat) / 2
		gotX, gotY := wgs84ToTileXY(centreLon, centreLat, z)
		if gotX != tc.x || gotY != tc.y {
			t.Errorf("centre of tile (%d,%d,%d) [%.4f,%.4f] maps to (%d,%d)",
				z, tc.x, tc.y, centreLon, centreLat, gotX, gotY)
		}
	}
}

// ---- layerNameFromPath -------------------------------------------------------

func TestLayerNameFromPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/tmp/countries_boundary.geojson", "countries_boundary"},
		{"/data/cz_border.shp", "cz_border"},
		{"polygon.json", "polygon"},
		{"noext", "noext"},
	}
	for _, tc := range tests {
		got := layerNameFromPath(tc.input)
		if got != tc.want {
			t.Errorf("layerNameFromPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---- prepareClipPolygon -----------------------------------------------------

// TestPrepareClipPolygon_PassthroughForPolygonFile verifies that a GeoJSON file
// that already has Polygon geometry is returned as-is without conversion.
func TestPrepareClipPolygon_PassthroughForPolygonFile(t *testing.T) {
	// Write a minimal polygon GeoJSON into a temp file.
	dir := t.TempDir()
	polyPath := filepath.Join(dir, "poly.geojson")
	polyContent := `{
  "type": "FeatureCollection",
  "features": [{
    "type": "Feature",
    "geometry": {
      "type": "Polygon",
      "coordinates": [[[12.0,48.0],[18.0,48.0],[18.0,51.0],[12.0,51.0],[12.0,48.0]]]
    },
    "properties": {}
  }]
}`
	if err := os.WriteFile(polyPath, []byte(polyContent), 0o644); err != nil {
		t.Fatalf("failed to write polygon fixture: %v", err)
	}

	ctx := context.Background()
	got, err := prepareClipPolygon(ctx, dir, polyPath, "")
	if err != nil {
		t.Fatalf("prepareClipPolygon returned error: %v", err)
	}
	if got != polyPath {
		t.Errorf("expected passthrough for polygon file, got %q (want %q)", got, polyPath)
	}
}

// TestPrepareClipPolygon_ConvertsLineStringFile verifies that a GeoJSON file
// with LineString geometry is converted to a polygon file in buildDir.
// This test requires ogr2ogr and ogrinfo to be on PATH; it is skipped otherwise.
func TestPrepareClipPolygon_ConvertsLineStringFile(t *testing.T) {
	if _, err := exec.LookPath("ogrinfo"); err != nil {
		t.Skip("ogrinfo not available")
	}
	if _, err := exec.LookPath("ogr2ogr"); err != nil {
		t.Skip("ogr2ogr not available")
	}

	// Write a minimal LineString GeoJSON.
	dir := t.TempDir()
	lsPath := filepath.Join(dir, "border.geojson")
	lsContent := `{
  "type": "FeatureCollection",
  "features": [
    {"type":"Feature","geometry":{"type":"LineString","coordinates":[[12.0,48.0],[18.0,48.0],[18.0,51.0],[12.0,51.0]]},"properties":{"name":"CZECHREPUBLIC_GERMANY"}},
    {"type":"Feature","geometry":{"type":"LineString","coordinates":[[12.0,48.0],[12.0,51.0],[15.0,51.0]]},"properties":{"name":"CZECHREPUBLIC_POLAND"}}
  ]
}`
	if err := os.WriteFile(lsPath, []byte(lsContent), 0o644); err != nil {
		t.Fatalf("failed to write linestring fixture: %v", err)
	}

	ctx := context.Background()
	got, err := prepareClipPolygon(ctx, dir, lsPath, "CZECHREPUBLIC")
	if err != nil {
		t.Fatalf("prepareClipPolygon returned error: %v", err)
	}

	// Expect converted polygon file in buildDir.
	if !strings.HasSuffix(got, "clip_polygon.geojson") {
		t.Errorf("expected converted polygon path, got %q", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("converted polygon file does not exist: %v", err)
	}
}

func TestPrepareClipPolygon_ErrorsWhenCountryFilterMatchesNothing(t *testing.T) {
	if _, err := exec.LookPath("ogrinfo"); err != nil {
		t.Skip("ogrinfo not available")
	}
	if _, err := exec.LookPath("ogr2ogr"); err != nil {
		t.Skip("ogr2ogr not available")
	}

	dir := t.TempDir()
	lsPath := filepath.Join(dir, "border.geojson")
	lsContent := `{
  "type": "FeatureCollection",
  "features": [
    {"type":"Feature","geometry":{"type":"LineString","coordinates":[[12.0,48.0],[18.0,48.0],[18.0,51.0],[12.0,51.0]]},"properties":{"name":"CZECHREPUBLIC_GERMANY"}}
  ]
}`
	if err := os.WriteFile(lsPath, []byte(lsContent), 0o644); err != nil {
		t.Fatalf("failed to write linestring fixture: %v", err)
	}

	ctx := context.Background()
	_, err := prepareClipPolygon(ctx, dir, lsPath, "NOT_A_COUNTRY")
	if err == nil {
		t.Fatal("expected error when country filter matches no features")
	}
}
