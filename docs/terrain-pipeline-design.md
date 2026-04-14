# Terrain Pipeline Design

## 1. Purpose

This document defines the runtime contract for deterministic Copernicus DEM preprocessing into PMTiles terrain artifacts.

## 2. Scope

- Input: Copernicus DEM source rasters for a specified AOI.
- Output: PMTiles terrain archive plus machine-readable manifest/report.
- Runtime target: MapLibre hillshade rendering and terrain-aware elevation queries.

## 3. CLI Contract (Normative)

Required terrain flags:

The same runtime settings may also be loaded from the grouped YAML config file
under `terrain`. When both sources are present, explicit CLI flags override
YAML values.

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
- `--terrain-elevation-quantization-m` (default 0 = disabled; round elevation to nearest N m before Terrarium encoding to reduce PNG size)
- `--terrain-clip-polygon` (path to GeoJSON/Shapefile used to clip tiles outside the AOI shape; see step 9)
- `--terrain-clip-country-name` (optional country name filter applied when converting a LineString border file to a clip polygon; e.g. `CZECHREPUBLIC`; default = use all features in file)

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
6. **(Optional) Elevation quantization**: when `--terrain-elevation-quantization-m`
   is set, a `gdal_calc.py` pass rounds each pixel to the nearest multiple of
   the specified step (e.g. 1 m). This eliminates sub-metre noise introduced
   by bilinear resampling, reducing Terrarium blue-channel entropy and improving
   PNG DEFLATE compression ratios significantly.
7. **Single-pass Terrarium RGB encoding**: one `gdal_calc.py` call with three
   `--calc` expressions writes all three bands to a GeoTIFF in a single raster
   read. No intermediate per-band files, no `gdalbuildvrt`, no second
   `gdal_translate`.
8. **Single tiling pass**: `gdal2tiles.py --resampling near` generates the XYZ
   tile pyramid. Nearest-neighbour resampling is required because averaging
   across Terrarium R-band boundaries (256 m per unit) produces large spurious
   elevation discontinuities at tile edges.
   - Worker count is configurable via `--terrain-gdal2tiles-processes`
     (defaults to all available CPUs).
9. **(Optional) Polygon tile clipping**: when `--terrain-clip-polygon` is set,
   the clip file is first prepared by `prepareClipPolygon`:
   - If the file already contains Polygon/MultiPolygon geometry it is used as-is.
   - If the file contains LineString geometry (e.g. the `countries_boundary.geojson`
     produced by the map pipeline, which stores border segments as LineStrings),
     `ogr2ogr` with the SQLite dialect builds a convex-hull polygon from the
     collected features. When `--terrain-clip-country-name` is set only features
     whose `name` property contains that string are included in the hull (e.g.
     `CZECHREPUBLIC` selects the four CZ border segments from a multi-country file).
     The result is written to `buildDir/clip_polygon.geojson`.
   - If `ogrinfo` or `ogr2ogr` are not in PATH, the step is silently skipped and
     all tiles are kept.
   After polygon preparation, every tile in the XYZ directory is tested against
   the polygon using `ogrinfo -al -so -spat`. Tiles whose geographic extent does
   not intersect the polygon are deleted from disk before PMTiles packaging,
   reducing tile count for irregular AOI shapes such as country outlines.
10. **Direct PMTiles v3 packaging**: a pure-Go writer walks the tile directory,
    sorts tiles by Hilbert tile ID, deduplicates identical blobs using FNV-1a
    128-bit, and writes the PMTiles v3 archive directly. No MBTiles intermediate
    and no `pmtiles convert` subprocess. The `pmtiles` binary is still required
    for the metadata-consistency validation gate (`pmtiles show`).
11. Compute PMTiles checksum and emit `terrain.manifest.json`.
12. Execute validation gates and emit optional build report.

Note: validation (step 10) runs before the PMTiles checksum is computed
(step 9) so that a failing quality gate does not pay the cost of hashing a
large file.

## 5. Validation Gates

- **Coverage**: expected AOI tile coordinates must exist for the configured
  zoom range. When `--terrain-clip-polygon` is used, tiles outside the polygon
  are intentionally absent; the coverage check tests only tiles that are
  present in the tile directory (missing tile count is the gap between expected
  bbox tiles and actual tile count).
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
- Elevation quantization (optional) reduces Terrarium blue-channel entropy,
  improving PNG compression with a typical 1 m vertical precision trade-off.
- Single `gdal_calc.py` pass replaces the former three-pass R/G/B approach.
- Polygon clip (optional) removes out-of-border tiles before packaging,
  reducing tile count for irregular AOI shapes without changing zoom range.
- One tiling pass (gdal2tiles) with direct PMTiles output replaces the former
  MBTiles + gdaladdo + pmtiles-convert chain.
- Tile data is streamed one tile at a time to a temp file; peak memory is
  O(one tile + directory entries), not O(entire pyramid).
- Tile deduplication uses FNV-1a 128-bit; a hash hit implies identical content
  with negligible collision risk for any realistic tile count.
- gdal2tiles parallelism is unrestricted by default (all CPUs).
- Tile validation uses a two-row sliding cache per zoom level, bounding memory
  to O(2 × row width) decoded images rather than O(zoom level area).
- SHA-256 hashing of source files is skipped when no checksums file is
  provided.
- Non-Copernicus-named source files are resolved via parallel `gdalinfo`
  calls (up to 8 concurrent processes).
