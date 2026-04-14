// Package output validates and serializes custom XML output.
package output

// packTilesDirToPMTiles converts an XYZ tile directory produced by
// gdal2tiles.py directly into a PMTiles v3 archive, eliminating both the
// MBTiles intermediate and the pmtiles-convert subprocess.
//
// PMTiles v3 specification reference:
//
//	https://github.com/protomaps/PMTiles/blob/main/spec/v3/spec.md
//
// High-level structure of a PMTiles v3 file:
//
//	[127-byte fixed header]
//	[root directory  – gzip-compressed varint-encoded entries, ≤ 16 KiB]
//	[JSON metadata   – gzip-compressed]
//	[leaf directories – gzip-compressed, only present when root would exceed 16 KiB]
//	[tile data       – raw PNG bytes, in Hilbert-curve tile ID order]

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// pmtilesHeaderLen is the fixed size of the PMTiles v3 binary header.
const pmtilesHeaderLen = 127

// pmRootDirMaxBytes is the maximum allowed compressed size of the root
// directory (header is 127 bytes; the remaining space in the first 16 KiB
// is available for the root directory).
const pmRootDirMaxBytes = 16384 - pmtilesHeaderLen

// pmTileType values (only PNG is used here).
const pmTileTypePNG uint8 = 2

// pmCompression values.
const (
	pmCompressionNone uint8 = 1
	pmCompressionGzip uint8 = 2
)

// pmEntryV3 is one directory entry.
//
// RunLength semantics (PMTiles v3 spec):
//   - RunLength == 0  → leaf-directory pointer (Offset/Length point to a leaf dir)
//   - RunLength >= 1  → tile entry; ALL tile IDs in [TileID, TileID+RunLength)
//     are served by the blob at (Offset, Length).  They must all be identical
//     bytes.  Two consecutive tiles with DIFFERENT content must be two separate
//     entries each with RunLength=1.
type pmEntryV3 struct {
	TileID    uint64
	Offset    uint64
	Length    uint32
	RunLength uint32
}

// --------------------------------------------------------------------------
// Hilbert curve tile ID
// --------------------------------------------------------------------------

// hilbertRotate reflects (x,y) within a quadrant of half-side s.
// Matches the canonical PMTiles reference implementation exactly.
func hilbertRotate(s, x, y, rx, ry uint32) (uint32, uint32) {
	if ry == 0 {
		if rx != 0 {
			x = s - 1 - x
			y = s - 1 - y
		}
		return y, x
	}
	return x, y
}

// zxyToTileID converts (zoom, x, y) in XYZ/slippy scheme to a PMTiles v3
// tile ID using the standard Hilbert-curve mapping.
//
// Port of ZxyToID from github.com/protomaps/go-pmtiles/pmtiles/tile_id.go,
// reproduced here to avoid pulling in the full go-pmtiles dependency.
func zxyToTileID(z uint8, x, y uint32) uint64 {
	if z == 0 {
		return 0
	}
	// Base offset = sum of tiles at all zooms below z = (4^z - 1) / 3.
	acc := uint64((1<<(z*2))-1) / 3
	// n counts down the current bit position (z-1 … 0).
	n := z - 1
	for s := uint32(1) << n; s > 0; s >>= 1 {
		rx := s & x // bitmask: non-zero means bit is set
		ry := s & y
		// Contribution of this level to the Hilbert index.
		acc += uint64((3*rx)^ry) << n
		x, y = hilbertRotate(s, x, y, rx, ry)
		n--
	}
	return acc
}

// --------------------------------------------------------------------------
// Varint encoding
// --------------------------------------------------------------------------

func appendUvarint(b []byte, v uint64) []byte {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], v)
	return append(b, tmp[:n]...)
}

// --------------------------------------------------------------------------
// Directory serialization
// --------------------------------------------------------------------------

// serializeEntries encodes a slice of pmEntryV3 using the PMTiles v3 delta-
// encoding format and gzip-compresses the result.
//
// Wire format (uncompressed):
//
//	varint(N)
//	for each entry: varint(delta_tileID)      — delta from previous TileID
//	for each entry: varint(runLength)
//	for each entry: varint(length)
//	for each entry: varint(offset+1 OR 0)     — 0 means consecutive with prev
func serializeEntries(entries []pmEntryV3) []byte {
	var raw []byte
	raw = appendUvarint(raw, uint64(len(entries)))

	var lastID uint64
	for _, e := range entries {
		raw = appendUvarint(raw, e.TileID-lastID)
		lastID = e.TileID
	}
	for _, e := range entries {
		raw = appendUvarint(raw, uint64(e.RunLength))
	}
	for _, e := range entries {
		raw = appendUvarint(raw, uint64(e.Length))
	}
	for i, e := range entries {
		if i > 0 && entries[i-1].Offset+uint64(entries[i-1].Length) == e.Offset {
			raw = appendUvarint(raw, 0) // consecutive sentinel
		} else {
			raw = appendUvarint(raw, e.Offset+1)
		}
	}

	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = w.Write(raw)
	_ = w.Close()
	return buf.Bytes()
}

// buildDirectories builds root and (if needed) leaf directory bytes from a
// flat sorted entry list. If the entire list fits in pmRootDirMaxBytes
// compressed, no leaf directories are produced. Otherwise entries are split
// into leaves of ≥ 4096 entries each, iteratively growing the leaf size until
// the root-pointer directory itself fits within the limit.
//
// Matches the logic of optimizeDirectories in the reference implementation.
func buildDirectories(entries []pmEntryV3) (rootBytes, leafBytes []byte) {
	rootBytes = serializeEntries(entries)
	if len(rootBytes) <= pmRootDirMaxBytes {
		return rootBytes, nil
	}

	// Need leaf directories. Start with leafSize = max(4096, len/3500) and
	// grow by 20% until the root (which becomes leaf pointers only) fits.
	leafSize := float64(len(entries)) / 3500
	if leafSize < 4096 {
		leafSize = 4096
	}
	for {
		ls := int(leafSize)
		var rootEntries []pmEntryV3
		var leaves []byte
		for i := 0; i < len(entries); i += ls {
			end := i + ls
			if end > len(entries) {
				end = len(entries)
			}
			chunk := serializeEntries(entries[i:end])
			rootEntries = append(rootEntries, pmEntryV3{
				TileID:    entries[i].TileID,
				Offset:    uint64(len(leaves)),
				Length:    uint32(len(chunk)),
				RunLength: 0, // leaf pointer
			})
			leaves = append(leaves, chunk...)
		}
		rootBytes = serializeEntries(rootEntries)
		if len(rootBytes) <= pmRootDirMaxBytes {
			return rootBytes, leaves
		}
		leafSize *= 1.2
	}
}

// --------------------------------------------------------------------------
// Header serialization (127 bytes, little-endian)
// --------------------------------------------------------------------------

type pmHeader struct {
	SpecVersion         uint8
	RootOffset          uint64
	RootLength          uint64
	MetadataOffset      uint64
	MetadataLength      uint64
	LeafDirOffset       uint64
	LeafDirLength       uint64
	TileDataOffset      uint64
	TileDataLength      uint64
	AddressedTilesCount uint64
	TileEntriesCount    uint64
	TileContentsCount   uint64
	Clustered           bool
	InternalCompression uint8
	TileCompression     uint8
	TileType            uint8
	MinZoom             uint8
	MaxZoom             uint8
	MinLonE7            int32
	MinLatE7            int32
	MaxLonE7            int32
	MaxLatE7            int32
	CenterZoom          uint8
	CenterLonE7         int32
	CenterLatE7         int32
}

func serializeHeader(h pmHeader) []byte {
	b := make([]byte, pmtilesHeaderLen)
	copy(b[0:7], "PMTiles")
	b[7] = h.SpecVersion

	putU64 := func(off int, v uint64) { binary.LittleEndian.PutUint64(b[off:], v) }
	putI32 := func(off int, v int32) { binary.LittleEndian.PutUint32(b[off:], uint32(v)) }

	putU64(8, h.RootOffset)
	putU64(16, h.RootLength)
	putU64(24, h.MetadataOffset)
	putU64(32, h.MetadataLength)
	putU64(40, h.LeafDirOffset)
	putU64(48, h.LeafDirLength)
	putU64(56, h.TileDataOffset)
	putU64(64, h.TileDataLength)
	putU64(72, h.AddressedTilesCount)
	putU64(80, h.TileEntriesCount)
	putU64(88, h.TileContentsCount)

	if h.Clustered {
		b[96] = 1
	}
	b[97] = h.InternalCompression
	b[98] = h.TileCompression
	b[99] = h.TileType
	b[100] = h.MinZoom
	b[101] = h.MaxZoom
	putI32(102, h.MinLonE7)
	putI32(106, h.MinLatE7)
	putI32(110, h.MaxLonE7)
	putI32(114, h.MaxLatE7)
	b[118] = h.CenterZoom
	putI32(119, h.CenterLonE7)
	putI32(123, h.CenterLatE7)

	return b
}

// --------------------------------------------------------------------------
// Tile path collection (metadata only, no data loaded yet)
// --------------------------------------------------------------------------

type tileRef struct {
	tileID uint64
	path   string
}

func collectTileRefs(tilesDir string) ([]tileRef, error) {
	var refs []tileRef
	err := filepath.WalkDir(tilesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".png") {
			return nil
		}
		rel, err := filepath.Rel(tilesDir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 3 {
			return nil
		}
		z64, e1 := strconv.ParseUint(parts[0], 10, 8)
		x64, e2 := strconv.ParseUint(parts[1], 10, 32)
		yStr := strings.TrimSuffix(parts[2], ".png")
		y64, e3 := strconv.ParseUint(yStr, 10, 32)
		if e1 != nil || e2 != nil || e3 != nil {
			return nil
		}
		refs = append(refs, tileRef{
			tileID: zxyToTileID(uint8(z64), uint32(x64), uint32(y64)),
			path:   path,
		})
		return nil
	})
	return refs, err
}

// --------------------------------------------------------------------------
// Main writer
// --------------------------------------------------------------------------

// packTilesDirToPMTiles walks an XYZ gdal2tiles output directory and writes a
// PMTiles v3 archive to outputPath.
//
// Design:
//   - Tile paths are collected and sorted by Hilbert tile ID (no data loaded yet).
//   - Tiles are streamed one at a time to a temporary file that becomes the
//     tile-data section of the final archive, so peak memory is O(one tile)
//     rather than O(entire pyramid) (Bug 4 fix).
//   - Identical blobs are deduplicated; the hash key is FNV-1a 128-bit.
//   - RunLength is only incremented for tiles that point to the exact same
//     blob (same offset+length); consecutive distinct tiles always get
//     separate entries (Bug 1 fix).
//   - Hilbert rotation uses the correct quadrant-local reflection formula
//     (Bug 2 fix).
//   - Directory entries are split into leaf directories when the root would
//     exceed pmRootDirMaxBytes (Bug 3 fix).
func packTilesDirToPMTiles(tilesDir, outputPath string, minZoom, maxZoom int, aoi domain.BoundingBox) error {
	// --- Phase 1: collect tile paths, sort by Hilbert tile ID ---
	refs, err := collectTileRefs(tilesDir)
	if err != nil {
		return fmt.Errorf("collect tile refs: %w", err)
	}
	if len(refs) == 0 {
		return fmt.Errorf("no tiles found in %q", tilesDir)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].tileID < refs[j].tileID })

	// --- Phase 2: open output file, reserve header space ---
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create pmtiles %q: %w", outputPath, err)
	}
	defer out.Close()

	// Write a placeholder header; we will seek back and overwrite it once all
	// offsets are known.
	placeholder := make([]byte, pmtilesHeaderLen)
	if _, err := out.Write(placeholder); err != nil {
		return fmt.Errorf("write header placeholder: %w", err)
	}

	// --- Phase 3: stream tiles to the output file, build directory entries ---
	// We reserve space for root+metadata+leaves after the header but before
	// tile data. Since we don't know their sizes yet, we stream tile data to a
	// temporary file and copy it once the directory is finalised.
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".pmtiles-tiledata-*")
	if err != nil {
		return fmt.Errorf("create temp tile data file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	type dedupVal struct {
		offset uint64
		length uint32
	}
	// hashKey is a full SHA-256 digest (32 bytes). Using a cryptographic hash
	// avoids the carry-propagation flaw in the previous custom FNV-1a 128-bit
	// implementation and eliminates any realistic collision risk.
	type hashKey [sha256.Size]byte
	dedup := make(map[hashKey]dedupVal, len(refs))

	var entries []pmEntryV3
	var tileDataLen, addressedTiles, tileContents uint64

	for _, ref := range refs {
		data, err := os.ReadFile(ref.path)
		if err != nil {
			return fmt.Errorf("read tile %q: %w", ref.path, err)
		}
		addressedTiles++

		hk := sha256.Sum256(data)

		if found, ok := dedup[hk]; ok {
			// Identical blob already stored.  Only extend the run-length of the
			// last entry when it already points to this exact blob AND the tile
			// IDs are consecutive.  Otherwise create a new entry.
			// (PMTiles RunLength means "N consecutive tile IDs share this blob".)
			if len(entries) > 0 {
				last := &entries[len(entries)-1]
				if last.TileID+uint64(last.RunLength) == ref.tileID &&
					last.Offset == found.offset && last.Length == found.length {
					last.RunLength++
					continue
				}
			}
			entries = append(entries, pmEntryV3{
				TileID:    ref.tileID,
				Offset:    found.offset,
				Length:    found.length,
				RunLength: 1,
			})
			continue
		}

		// New unique blob: write to temp file.
		offset := tileDataLen
		if _, err := tmp.Write(data); err != nil {
			return fmt.Errorf("write tile data: %w", err)
		}
		length := uint32(len(data))
		tileDataLen += uint64(length)
		dedup[hk] = dedupVal{offset, length}
		tileContents++

		// Consecutive distinct blobs are NEVER merged into one entry — only
		// identical blobs sharing the same (offset, length) may be RLE'd.
		entries = append(entries, pmEntryV3{
			TileID:    ref.tileID,
			Offset:    offset,
			Length:    length,
			RunLength: 1,
		})
	}

	// --- Phase 4: build directories and metadata ---
	rootDirBytes, leafDirBytes := buildDirectories(entries)

	centerZoom := (minZoom + maxZoom) / 2
	centerLon := (aoi.MinLon + aoi.MaxLon) / 2
	centerLat := (aoi.MinLat + aoi.MaxLat) / 2
	metaMap := map[string]any{
		"name":        "terrain",
		"description": "Terrarium terrain tiles",
		"format":      "png",
		"minzoom":     strconv.Itoa(minZoom),
		"maxzoom":     strconv.Itoa(maxZoom),
		"bounds":      fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", aoi.MinLon, aoi.MinLat, aoi.MaxLon, aoi.MaxLat),
		"center":      fmt.Sprintf("%.6f,%.6f,%d", centerLon, centerLat, centerZoom),
	}
	rawMeta, err := json.Marshal(metaMap)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	var metaBuf bytes.Buffer
	mw, _ := gzip.NewWriterLevel(&metaBuf, gzip.BestCompression)
	_, _ = mw.Write(rawMeta)
	_ = mw.Close()
	metaBytes := metaBuf.Bytes()

	// --- Phase 5: compute offsets and write final file ---
	// Layout: header | root dir | metadata | leaf dirs | tile data
	rootOffset := uint64(pmtilesHeaderLen)
	metaOffset := rootOffset + uint64(len(rootDirBytes))
	leafDirOffset := metaOffset + uint64(len(metaBytes))
	tileDataOffset := leafDirOffset + uint64(len(leafDirBytes))

	e7 := func(deg float64) int32 { return int32(math.Round(deg * 1e7)) }
	hdr := pmHeader{
		SpecVersion:         3,
		RootOffset:          rootOffset,
		RootLength:          uint64(len(rootDirBytes)),
		MetadataOffset:      metaOffset,
		MetadataLength:      uint64(len(metaBytes)),
		LeafDirOffset:       leafDirOffset,
		LeafDirLength:       uint64(len(leafDirBytes)),
		TileDataOffset:      tileDataOffset,
		TileDataLength:      tileDataLen,
		AddressedTilesCount: addressedTiles,
		TileEntriesCount:    uint64(len(entries)),
		TileContentsCount:   tileContents,
		Clustered:           true,
		InternalCompression: pmCompressionGzip,
		TileCompression:     pmCompressionNone, // PNG tiles are not additionally compressed
		TileType:            pmTileTypePNG,
		MinZoom:             uint8(minZoom),
		MaxZoom:             uint8(maxZoom),
		MinLonE7:            e7(aoi.MinLon),
		MinLatE7:            e7(aoi.MinLat),
		MaxLonE7:            e7(aoi.MaxLon),
		MaxLatE7:            e7(aoi.MaxLat),
		CenterZoom:          uint8(centerZoom),
		CenterLonE7:         e7(centerLon),
		CenterLatE7:         e7(centerLat),
	}

	// Seek back to offset 0 and write the real header.
	if _, err := out.Seek(0, 0); err != nil {
		return fmt.Errorf("seek to header: %w", err)
	}
	if _, err := out.Write(serializeHeader(hdr)); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := out.Write(rootDirBytes); err != nil {
		return fmt.Errorf("write root dir: %w", err)
	}
	if _, err := out.Write(metaBytes); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	if len(leafDirBytes) > 0 {
		if _, err := out.Write(leafDirBytes); err != nil {
			return fmt.Errorf("write leaf dirs: %w", err)
		}
	}
	// Copy tile data from the temp file.
	if _, err := tmp.Seek(0, 0); err != nil {
		return fmt.Errorf("seek temp file: %w", err)
	}
	if _, err := out.Seek(int64(tileDataOffset), 0); err != nil {
		return fmt.Errorf("seek to tile data section: %w", err)
	}
	if _, err := copyFileData(out, tmp); err != nil {
		return fmt.Errorf("copy tile data: %w", err)
	}

	return nil
}

// copyFileData copies from src to dst using a reusable buffer.
func copyFileData(dst, src *os.File) (int64, error) {
	buf := make([]byte, 256*1024) // 256 KiB copy buffer
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return total, nil
			}
			return total, err
		}
	}
}
