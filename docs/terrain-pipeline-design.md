# Terrain Pipeline Design

## 1. Purpose

This document defines the runtime contract for deterministic Copernicus DEM preprocessing into PMTiles terrain artifacts.

## 2. Scope

- Input: Copernicus DEM source rasters for a specified AOI.
- Output: PMTiles terrain archive plus machine-readable manifest/report.
- Runtime target: MapLibre hillshade rendering and terrain-aware elevation queries.

## 3. CLI Contract (Normative)

Required terrain flags:

- `--terrain-source-dir`
- `--terrain-aoi-bbox`
- `--terrain-version`
- `--terrain-pmtiles-output`

Optional quality/determinism controls:

- `--terrain-source-checksums`
- `--terrain-manifest-output`
- `--terrain-build-report-output`
- `--terrain-build-timestamp`
- `--terrain-control-points`
- `--terrain-min-zoom`, `--terrain-max-zoom`
- `--terrain-tile-size`
- `--terrain-seam-threshold`
- `--terrain-rmse-threshold-m`
- `--terrain-gdal2tiles-processes` (default 0 = use all CPUs)

## 4. Processing Stages

1. Ingest source raster files from `--terrain-source-dir`.
   - Files whose bounding box (from filename or `gdalinfo`) does not intersect
     the AOI are silently skipped before any further I/O.
   - SHA-256 checksums are computed only when `--terrain-source-checksums` is
     provided (lazy hashing).
2. Validate source integrity (checksum file if provided).
3. Build deterministic preprocessing plan in build directory.
4. **Crop + reproject first**: `gdalwarp -te` narrows to the AOI in EPSG:3857
   before any nodata fill, so fill work is bounded to the AOI extent.
5. **Fill nodata** on the already-cropped raster only.
6. **Single-pass Terrarium RGB encoding**: one `gdal_calc.py` call with three
   `--calc` expressions writes all three bands to a GeoTIFF in a single raster
   read. No intermediate per-band files, no `gdalbuildvrt`, no second
   `gdal_translate`.
7. **Single tiling pass**: `gdal2tiles.py --resampling near` generates the XYZ
   tile pyramid. Nearest-neighbour resampling is required because averaging
   across Terrarium R-band boundaries (256 m per unit) produces large spurious
   elevation discontinuities at tile edges.
   - Worker count is configurable via `--terrain-gdal2tiles-processes`
     (defaults to all available CPUs).
8. **Direct PMTiles v3 packaging**: a pure-Go writer walks the tile directory,
   sorts tiles by Hilbert tile ID, deduplicates identical blobs using SHA-256,
   and writes the PMTiles v3 archive directly. No MBTiles intermediate and no
   `pmtiles convert` subprocess. The `pmtiles` binary is still required for
   the metadata-consistency validation gate (`pmtiles show`).
9. Compute PMTiles checksum and emit `terrain.manifest.json`.
10. Execute validation gates and emit optional build report.

Note: validation (step 10) runs before the PMTiles checksum is computed
(step 9) so that a failing quality gate does not pay the cost of hashing a
large file.

## 5. Validation Gates

- **Coverage**: expected AOI tile coordinates must exist for the configured
  zoom range.
- **Seams**: for each pair of adjacent tiles the seam delta is
  `max(0, cross - max(leftGrad, rightGrad))`, where `cross` is the elevation
  jump across the boundary and `leftGrad`/`rightGrad` are the one-pixel
  gradients on each side. The 95th-percentile of per-row deltas must not
  exceed `--terrain-seam-threshold`. Coverage and seam checks are performed
  in a single pass per zoom level with a two-row sliding cache (only the
  current and next row are held in memory at a time).
- **Elevation checks**: optional control-point RMSE threshold gate.
  `gdallocationinfo` calls are issued in parallel (up to 8 concurrent
  processes).
- **Raster sanity**: the warped DEM file must exist and be non-empty; the
  tile at the centre of the AOI at the middle zoom level is decoded and
  checked for at least one non-transparent pixel. A missing tile (edge AOI)
  is silently accepted; other open errors (permission, I/O) are hard failures.
- **Metadata consistency**: PMTiles header min/max zoom must match the
  manifest values (`pmtiles show --header-json`).

Validation failures are hard errors and block publication.

## 6. Determinism Notes

- Source files are sorted deterministically before mosaicking.
- AOI filtering is deterministic given a fixed filename convention.
- Build plan file naming is stable.
- Build timestamp can be pinned with `--terrain-build-timestamp`.
- Manifest and build report use canonical JSON serialization.

## 7. Artifacts

- `terrain.pmtiles`
- `terrain.manifest.json`
- optional `build-report.json`

## 8. Performance Notes

- Only DEM tiles intersecting the AOI are ingested and mosaicked.
- Crop before fill eliminates nodata filling on the full-mosaic extent.
- Single `gdal_calc.py` pass replaces the former three-pass R/G/B approach.
- One tiling pass (gdal2tiles) with direct PMTiles output replaces the former
  MBTiles + gdaladdo + pmtiles-convert chain.
- Tile data is streamed one tile at a time to a temp file; peak memory is
  O(one tile + directory entries), not O(entire pyramid).
- Tile deduplication uses SHA-256 (stdlib `crypto/sha256`); a hash hit implies
  identical content with no collision risk for any realistic tile count.
- gdal2tiles parallelism is unrestricted by default (all CPUs).
- Tile validation uses a two-row sliding cache per zoom level, bounding memory
  to O(2 × row width) decoded images rather than O(zoom level area).
- SHA-256 hashing of source files is skipped when no checksums file is
  provided.
- Non-Copernicus-named source files are resolved via parallel `gdalinfo`
  calls (up to 8 concurrent processes).
