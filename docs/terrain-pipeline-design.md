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

## 4. Processing Stages

1. Ingest source raster files from `--terrain-source-dir`.
2. Validate source integrity (checksum file if provided).
3. Build deterministic preprocessing plan in build directory.
4. Execute GDAL-based mosaic, nodata fill, AOI crop/reproject to EPSG:3857.
5. Generate tile pyramid and package to PMTiles v3.
6. Compute PMTiles checksum and emit `terrain.manifest.json`.
7. Execute validation gates and emit optional build report.

## 5. Validation Gates

- Coverage: expected AOI tile coordinates must exist for configured zoom range.
- Seams: adjacent tile edge pixel deltas must not exceed threshold.
- Elevation checks: optional control-point RMSE threshold gate.
- Hillshade sanity: raster stats must be present on generated hillshade.
- Metadata consistency: PMTiles header min/max zoom must match manifest.

Validation failures are hard errors and block publication.

## 6. Determinism Notes

- Source files are sorted deterministically.
- Build plan file naming is stable.
- Build timestamp can be pinned with `--terrain-build-timestamp`.
- Manifest and build report use canonical JSON serialization.

## 7. Artifacts

- `terrain.pmtiles`
- `terrain.manifest.json`
- optional `build-report.json`
