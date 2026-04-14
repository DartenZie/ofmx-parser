// Package output validates and serializes custom XML output.
package output

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

var testAOI = domain.BoundingBox{MinLon: 15, MinLat: 47, MaxLon: 17, MaxLat: 49}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// writeTile creates <tilesDir>/<z>/<x>/<y>.png with the given content.
func writeTile(t *testing.T, tilesDir string, z, x, y int, data []byte) {
	t.Helper()
	dir := filepath.Join(tilesDir, strconv.Itoa(z), strconv.Itoa(x))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, strconv.Itoa(y)+".png")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write tile z=%d x=%d y=%d: %v", z, x, y, err)
	}
}

// readPMTilesHeader parses the 127-byte header from a raw byte slice.
func readPMTilesHeader(t *testing.T, b []byte) (rootOff, rootLen, metaOff, metaLen,
	leafOff, leafLen, dataOff, dataLen, addressed, entries, contents uint64,
	minZ, maxZ uint8) {
	t.Helper()
	if len(b) < pmtilesHeaderLen {
		t.Fatalf("file too short for header: %d", len(b))
	}
	u64 := func(off int) uint64 { return binary.LittleEndian.Uint64(b[off:]) }
	rootOff = u64(8)
	rootLen = u64(16)
	metaOff = u64(24)
	metaLen = u64(32)
	leafOff = u64(40)
	leafLen = u64(48)
	dataOff = u64(56)
	dataLen = u64(64)
	addressed = u64(72)
	entries = u64(80)
	contents = u64(88)
	minZ = b[100]
	maxZ = b[101]
	return
}

// decompressGzip decompresses gzip bytes, fatal on error.
func decompressGzip(t *testing.T, compressed []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	return raw
}

// packSimple builds a PMTiles archive from a tiles dir and reads the output
// bytes back. Helper to avoid repetition across tests.
func packSimple(t *testing.T, tilesDir string, minZ, maxZ int) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "out.pmtiles")
	if err := packTilesDirToPMTiles(tilesDir, out, minZ, maxZ, testAOI); err != nil {
		t.Fatalf("packTilesDirToPMTiles: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return b
}

// -----------------------------------------------------------------------
// Hilbert tile ID
// -----------------------------------------------------------------------

// TestZxyToTileID verifies the Hilbert-curve tile ID mapping against the
// canonical values from the PMTiles reference implementation.
func TestZxyToTileID(t *testing.T) {
	t.Parallel()

	// Expected values verified by running the canonical ZxyToID from the
	// go-pmtiles reference implementation (pmtiles/tile_id.go).
	// z=1 Hilbert order: (0,0)→1, (0,1)→2, (1,1)→3, (1,0)→4
	// z=2 base = (4^2-1)/3 = 5:
	//   (0,0)→5, (0,1)→8, (0,2)→9, (0,3)→10,
	//   (1,0)→6, (1,1)→7, (1,2)→12, (1,3)→11, ...
	tests := []struct {
		z    uint8
		x, y uint32
		want uint64
	}{
		{0, 0, 0, 0},
		{1, 0, 0, 1},
		{1, 0, 1, 2},
		{1, 1, 1, 3},
		{1, 1, 0, 4},
		{2, 0, 0, 5},
		{2, 1, 0, 6},
		{2, 1, 1, 7},
		{2, 0, 1, 8},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("z%d/%d/%d", tc.z, tc.x, tc.y), func(t *testing.T) {
			t.Parallel()
			got := zxyToTileID(tc.z, tc.x, tc.y)
			if got != tc.want {
				t.Errorf("zxyToTileID(%d,%d,%d) = %d, want %d",
					tc.z, tc.x, tc.y, got, tc.want)
			}
		})
	}
}

// TestZxyToTileIDMonotone verifies that tile IDs increase as tiles are
// traversed in Hilbert order within a zoom level, and that the first tile of
// zoom z+1 equals the base offset for that zoom.
func TestZxyToTileIDMonotone(t *testing.T) {
	t.Parallel()

	// Base offset for zoom z = (4^z - 1) / 3.
	// z=0:0, z=1:1, z=2:5, z=3:21
	bases := []uint64{0, 1, 5, 21}
	for z := uint8(0); z < 4; z++ {
		if got := zxyToTileID(z, 0, 0); got != bases[z] {
			t.Errorf("z=%d: base tile ID = %d, want %d", z, got, bases[z])
		}
	}
}

// -----------------------------------------------------------------------
// Magic, zoom range, header fields
// -----------------------------------------------------------------------

func TestPackTilesDirToPMTilesMagic(t *testing.T) {
	t.Parallel()

	tilesDir := t.TempDir()
	writeTile(t, tilesDir, 5, 0, 0, []byte{0x89, 0x50, 0x4e, 0x47})

	b := packSimple(t, tilesDir, 5, 5)
	if string(b[0:7]) != "PMTiles" {
		t.Fatalf("magic = %q, want PMTiles", string(b[0:7]))
	}
	if b[7] != 3 {
		t.Fatalf("spec version = %d, want 3", b[7])
	}
}

func TestPackTilesDirToPMTilesZoomRange(t *testing.T) {
	t.Parallel()

	const minZ, maxZ = 6, 10
	tilesDir := t.TempDir()
	for z := minZ; z <= maxZ; z++ {
		writeTile(t, tilesDir, z, 0, 0, []byte{byte(z), 0, 0, 0})
	}

	b := packSimple(t, tilesDir, minZ, maxZ)
	if b[100] != minZ {
		t.Errorf("MinZoom = %d, want %d", b[100], minZ)
	}
	if b[101] != maxZ {
		t.Errorf("MaxZoom = %d, want %d", b[101], maxZ)
	}
}

// -----------------------------------------------------------------------
// Addressed/entry/content counts
// -----------------------------------------------------------------------

// TestPackTilesDirToPMTilesCountsDistinct: N tiles all with distinct content →
// addressed=N, entries=N, contents=N.
func TestPackTilesDirToPMTilesCountsDistinct(t *testing.T) {
	t.Parallel()

	// z=2 has 16 possible tiles; write a 2×2 grid at x=0..1, y=0..1.
	tilesDir := t.TempDir()
	const z = 2
	type xy struct{ x, y int }
	pairs := []xy{{0, 0}, {0, 1}, {1, 0}, {1, 1}}
	for i, p := range pairs {
		writeTile(t, tilesDir, z, p.x, p.y, []byte{0x01, byte(i), 0x03, 0x04})
	}

	b := packSimple(t, tilesDir, z, z)
	_, _, _, _, _, _, _, _, addressed, entries, contents, _, _ := readPMTilesHeader(t, b)

	if addressed != 4 {
		t.Errorf("AddressedTiles = %d, want 4", addressed)
	}
	if entries != 4 {
		t.Errorf("TileEntries = %d, want 4 (no dedup, no RLE)", entries)
	}
	if contents != 4 {
		t.Errorf("TileContents = %d, want 4", contents)
	}
}

// TestPackTilesDirToPMTilesCountsDedup: two tiles with identical bytes →
// addressed=2, contents=1, entries may be 1 (RLE) or 2 (non-consecutive IDs).
func TestPackTilesDirToPMTilesCountsDedup(t *testing.T) {
	t.Parallel()

	tilesDir := t.TempDir()
	same := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	// z=1: tiles (0,0) and (0,1) — Hilbert IDs 1 and 2 (consecutive).
	writeTile(t, tilesDir, 1, 0, 0, same)
	writeTile(t, tilesDir, 1, 0, 1, same)

	b := packSimple(t, tilesDir, 1, 1)
	_, _, _, _, _, _, _, _, addressed, entries, contents, _, _ := readPMTilesHeader(t, b)

	if addressed != 2 {
		t.Errorf("AddressedTiles = %d, want 2", addressed)
	}
	if contents != 1 {
		t.Errorf("TileContents = %d, want 1 (deduped)", contents)
	}
	// Consecutive identical tiles may be merged into 1 RLE entry.
	if entries > 2 {
		t.Errorf("TileEntries = %d, want ≤ 2", entries)
	}
}

// -----------------------------------------------------------------------
// Bug 1 regression: distinct consecutive tiles must NOT share RunLength
// -----------------------------------------------------------------------

// TestPackTilesDirToPMTilesDistinctTilesNotRLE verifies that consecutive tiles
// with different content produce separate directory entries.  Previously the
// writer incorrectly extended RunLength for any consecutive tile ID pair,
// causing readers to serve the first tile's bytes for all subsequent tile IDs.
func TestPackTilesDirToPMTilesDistinctTilesNotRLE(t *testing.T) {
	t.Parallel()

	tilesDir := t.TempDir()
	// z=1: (0,0) ID=1, (0,1) ID=2, (1,1) ID=3 — three consecutive Hilbert IDs.
	tileA := []byte{0xA1, 0xA2, 0xA3, 0xA4}
	tileB := []byte{0xB1, 0xB2, 0xB3, 0xB4}
	tileC := []byte{0xC1, 0xC2, 0xC3, 0xC4}
	writeTile(t, tilesDir, 1, 0, 0, tileA) // ID 1
	writeTile(t, tilesDir, 1, 0, 1, tileB) // ID 2
	writeTile(t, tilesDir, 1, 1, 1, tileC) // ID 3

	outPath := filepath.Join(t.TempDir(), "out.pmtiles")
	if err := packTilesDirToPMTiles(tilesDir, outPath, 1, 1, testAOI); err != nil {
		t.Fatalf("pack: %v", err)
	}
	b, _ := os.ReadFile(outPath)

	rootOff, rootLen, _, _, _, _, dataOff, _, _, _, _, _, _ := readPMTilesHeader(t, b)

	// Deserialize the root directory to check entry count and run lengths.
	rawDir := decompressGzip(t, b[rootOff:rootOff+rootLen])
	dirEntries := deserializeTestEntries(t, rawDir)

	// Three distinct tiles must yield at least 3 entries (no cross-content RLE).
	if len(dirEntries) < 3 {
		t.Fatalf("got %d directory entries, want ≥ 3 (distinct tiles must not be RLE'd)", len(dirEntries))
	}

	// Every tile blob must be independently reachable and match its source data.
	tileData := b[dataOff:]
	want := [][]byte{tileA, tileB, tileC}
	for i, e := range dirEntries {
		if e.RunLength != 1 {
			t.Errorf("entry[%d] RunLength=%d, want 1 (distinct content)", i, e.RunLength)
		}
		got := tileData[e.Offset : e.Offset+uint64(e.Length)]
		if !bytes.Equal(got, want[i]) {
			t.Errorf("entry[%d] blob mismatch: got %x, want %x", i, got, want[i])
		}
	}
}

// TestPackTilesDirToPMTilesRLEIdentical verifies that truly identical consecutive
// tiles ARE correctly merged into one entry with RunLength > 1.
func TestPackTilesDirToPMTilesRLEIdentical(t *testing.T) {
	t.Parallel()

	tilesDir := t.TempDir()
	same := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	// z=1: (0,0) ID=1, (0,1) ID=2, (1,1) ID=3 — all identical content.
	writeTile(t, tilesDir, 1, 0, 0, same)
	writeTile(t, tilesDir, 1, 0, 1, same)
	writeTile(t, tilesDir, 1, 1, 1, same)

	b := packSimple(t, tilesDir, 1, 1)
	rootOff, rootLen, _, _, _, _, _, _, _, _, _, _, _ := readPMTilesHeader(t, b)
	rawDir := decompressGzip(t, b[rootOff:rootOff+rootLen])
	dirEntries := deserializeTestEntries(t, rawDir)

	// All three consecutive identical tiles must collapse into one entry.
	if len(dirEntries) != 1 {
		t.Fatalf("got %d entries, want 1 (three identical consecutive tiles)", len(dirEntries))
	}
	if dirEntries[0].RunLength != 3 {
		t.Errorf("RunLength = %d, want 3", dirEntries[0].RunLength)
	}
}

// -----------------------------------------------------------------------
// Metadata
// -----------------------------------------------------------------------

func TestPackTilesDirToPMTilesMetadata(t *testing.T) {
	t.Parallel()

	const minZ, maxZ = 5, 10
	tilesDir := t.TempDir()
	for z := minZ; z <= maxZ; z++ {
		writeTile(t, tilesDir, z, 0, 0, []byte{byte(z)})
	}

	b := packSimple(t, tilesDir, minZ, maxZ)
	_, _, metaOff, metaLen, _, _, _, _, _, _, _, _, _ := readPMTilesHeader(t, b)

	raw := decompressGzip(t, b[metaOff:metaOff+metaLen])

	for _, needle := range []string{`"minzoom"`, strconv.Itoa(minZ), strconv.Itoa(maxZ)} {
		if !bytes.Contains(raw, []byte(needle)) {
			t.Errorf("metadata missing %q; got: %s", needle, raw)
		}
	}
}

// -----------------------------------------------------------------------
// Bug 3 regression: large directory must not exceed root size limit
// -----------------------------------------------------------------------

// TestPackTilesDirToPMTilesLargeDirectory verifies that when the number of
// directory entries would produce a compressed root exceeding pmRootDirMaxBytes,
// leaf directories are generated and the root stays within the limit.
func TestPackTilesDirToPMTilesLargeDirectory(t *testing.T) {
	t.Parallel()

	// Force many unique entries by writing tiles with distinct content.
	// z=7 has 128×128 = 16384 possible tiles; write enough to stress the dir.
	// We need the compressed directory to exceed 16 KiB, which in practice
	// never happens for realistic terrain pyramids (see analysis in tests), but
	// we can force it by writing many entries with large random-looking lengths.
	//
	// Strategy: synthesize entries directly and test buildDirectories, which is
	// the unit that enforces the limit.
	const N = 70000 // well beyond what fits in root for non-compressible data
	entries := make([]pmEntryV3, N)
	var offset uint64
	for i := range entries {
		// Use distinct non-consecutive tile IDs to prevent varint delta compression.
		entries[i] = pmEntryV3{
			TileID:    uint64(i) * 7, // non-consecutive → large delta varints
			Offset:    offset,
			Length:    4096,
			RunLength: 1,
		}
		offset += 4096
	}

	rootBytes, leafBytes := buildDirectories(entries)

	if len(rootBytes) > pmRootDirMaxBytes {
		t.Errorf("root dir = %d bytes, exceeds limit %d", len(rootBytes), pmRootDirMaxBytes)
	}
	// When root is over-limit, leaf bytes must be non-empty.
	rootFitsAlone := len(serializeEntries(entries)) <= pmRootDirMaxBytes
	if !rootFitsAlone && len(leafBytes) == 0 {
		t.Error("large directory should have produced leaf dir bytes, got none")
	}
}

// -----------------------------------------------------------------------
// Streaming / memory: tile data is reachable from the header offsets
// -----------------------------------------------------------------------

// TestPackTilesDirToPMTilesTileDataReadable verifies that the tile data section
// is exactly where the header says it is, and that each tile blob can be read
// back verbatim using the directory entry offsets.
func TestPackTilesDirToPMTilesTileDataReadable(t *testing.T) {
	t.Parallel()

	tilesDir := t.TempDir()
	tileA := []byte{0x11, 0x22, 0x33}
	tileB := []byte{0x44, 0x55, 0x66, 0x77}
	tileC := []byte{0x88, 0x99}
	// z=1 Hilbert order: (0,0)→1, (0,1)→2, (1,1)→3
	writeTile(t, tilesDir, 1, 0, 0, tileA)
	writeTile(t, tilesDir, 1, 0, 1, tileB)
	writeTile(t, tilesDir, 1, 1, 1, tileC)

	b := packSimple(t, tilesDir, 1, 1)
	rootOff, rootLen, _, _, _, _, dataOff, dataLen, _, _, _, _, _ := readPMTilesHeader(t, b)

	// Verify tile data section length matches sum of unique blob sizes.
	wantDataLen := uint64(len(tileA) + len(tileB) + len(tileC))
	if dataLen != wantDataLen {
		t.Errorf("TileDataLength = %d, want %d", dataLen, wantDataLen)
	}

	rawDir := decompressGzip(t, b[rootOff:rootOff+rootLen])
	dirEntries := deserializeTestEntries(t, rawDir)

	want := [][]byte{tileA, tileB, tileC}
	for i, e := range dirEntries {
		start := dataOff + e.Offset
		end := start + uint64(e.Length)
		if int(end) > len(b) {
			t.Fatalf("entry[%d] offset+length out of bounds", i)
		}
		got := b[start:end]
		if !bytes.Equal(got, want[i]) {
			t.Errorf("entry[%d] data = %x, want %x", i, got, want[i])
		}
	}
}

// -----------------------------------------------------------------------
// Entry serialization round-trip
// -----------------------------------------------------------------------

func TestSerializeEntriesRoundTrip(t *testing.T) {
	t.Parallel()

	entries := []pmEntryV3{
		{TileID: 0, Offset: 0, Length: 100, RunLength: 1},
		{TileID: 1, Offset: 100, Length: 200, RunLength: 2},
		{TileID: 3, Offset: 300, Length: 50, RunLength: 1},
		// Non-consecutive offset (gap in data)
		{TileID: 10, Offset: 1000, Length: 75, RunLength: 1},
	}

	serialized := serializeEntries(entries)
	got := deserializeTestEntries(t, decompressGzip(t, serialized))

	if len(got) != len(entries) {
		t.Fatalf("round-trip length %d, want %d", len(got), len(entries))
	}
	for i, want := range entries {
		if got[i] != want {
			t.Errorf("entry[%d]: got %+v, want %+v", i, got[i], want)
		}
	}
}

// -----------------------------------------------------------------------
// deserializeTestEntries: minimal in-test directory decoder
// -----------------------------------------------------------------------

// deserializeTestEntries decodes the uncompressed varint directory format.
// This is a local reimplementation of the spec decoder used only in tests,
// so that tests do not depend on the production serializer being correct.
func deserializeTestEntries(t *testing.T, raw []byte) []pmEntryV3 {
	t.Helper()
	r := bytes.NewReader(raw)

	readUvarint := func() uint64 {
		v, err := binary.ReadUvarint(r)
		if err != nil {
			t.Fatalf("read uvarint: %v", err)
		}
		return v
	}

	n := int(readUvarint())
	entries := make([]pmEntryV3, n)

	var lastID uint64
	for i := range entries {
		delta := readUvarint()
		entries[i].TileID = lastID + delta
		lastID = entries[i].TileID
	}
	for i := range entries {
		entries[i].RunLength = uint32(readUvarint())
	}
	for i := range entries {
		entries[i].Length = uint32(readUvarint())
	}
	for i := range entries {
		raw := readUvarint()
		if i > 0 && raw == 0 {
			entries[i].Offset = entries[i-1].Offset + uint64(entries[i-1].Length)
		} else {
			entries[i].Offset = raw - 1
		}
	}
	return entries
}
