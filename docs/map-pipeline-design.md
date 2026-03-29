# Map Pipeline Design

## 1. Purpose

This document defines the architectural contract and implemented behavior for PMTiles generation from OSM PBF and OFMX-derived aeronautical data.

Status: implemented through milestones M5.1-M5.8.

## 2. Design Goals

- Preserve current XML conversion pipeline behavior without regressions.
- Introduce map generation as an optional and independent output branch.
- Keep package boundaries aligned with repository rules:
  - `ingest`: input parsing only,
  - `transform`: model mapping only,
  - `output`: serialization/external tool execution only,
  - `pipeline`: orchestration only.
- Ensure deterministic map feature generation (ordering and stable IDs).

## 3. Pipeline Topology

Map generation reuses OFMX ingest and transform input models but writes a different target artifact.

Logical flow:

1. Parse OFMX input into canonical domain model.
2. Build map-oriented intermediate model.
3. Export intermediate model to GeoJSON sources.
4. Invoke tilemaker against OSM PBF plus generated sources.
5. Produce `.pmtiles` artifact.

XML and map branches may run independently or together. In dual mode, OFMX ingest is executed once and reused by both branches.

## 4. CLI Contract (Normative)

### 4.1 Inputs

Map mode uses these flags:

- `--pbf-input <path>`: OSM PBF input for tilemaker.
- `--pmtiles-output <path>`: target PMTiles output path.
- `--arc-max-chord-m <meters>`: optional arc/circle densification chord limit (default `750`).
- `--tilemaker-bin <path-or-name>`: tilemaker executable (default `tilemaker`).
- `--tilemaker-config <path>`: optional custom config override.
- `--tilemaker-process <path>`: optional custom process.lua override.
- `--map-temp-dir <path>`: optional stable working directory for generated runtime files.

### 4.2 Execution Modes

- XML-only mode: no PMTiles flags supplied.
- Map-only mode: PMTiles flags supplied, XML `--output` omitted.
- Dual mode: both XML and PMTiles outputs requested.

### 4.3 Failure Semantics

Strict-fail policy is required:

- If PMTiles generation is requested and tilemaker is unavailable, execution MUST fail.
- Missing tilemaker is treated as a hard error, not warning.
- Error category MUST be typed for CLI diagnostics.

## 5. Runtime Artifacts

Generated transient assets (in temp directory):

- `aviation_airports.geojson`
- `aviation_zones.geojson`
- `aviation_poi.geojson`
- `aviation_airspace_borders.geojson`
- `countries_boundary.geojson`
- generated tilemaker `tilemaker.generated.config.json`
- generated tilemaker `tilemaker.generated.process.lua`

Temp directory lifecycle:

- if `--map-temp-dir` is provided, generated runtime artifacts are preserved in that directory,
- if omitted, an auto-created temporary directory is removed after map pipeline completion (success or failure).

Final artifact:

- `<output>.pmtiles`

## 6. Error Model

Typed errors in the implemented map branch include:

- map config/CLI contract failures,
- tilemaker executable lookup failures,
- runtime file generation failures,
- tilemaker process execution failures (non-zero exit),
- expected PMTiles output not produced.

## 7. Determinism and Validation

- GeoJSON feature ordering is stable and deterministic.
- Shared airspace borders are deduplicated deterministically using normalized, quantized undirected segments.
- Integration tests validate map mode CLI wiring, strict-fail behavior, runtime artifact generation, and tilemaker argument propagation.
