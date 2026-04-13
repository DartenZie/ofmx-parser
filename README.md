# OFMX Parser (CLI)

`ofmx-parser` is a CLI-focused Go project for parsing OFMX data and producing:

- custom XML output for thesis-defined exchange,
- PMTiles map output from OSM PBF plus OFMX aviation overlays.
- terrain PMTiles output from Copernicus DEM inputs for hillshade/render-query workflows.

The initial structure is intentionally layered so the mapping logic can grow without refactoring core wiring.

## Project Goals

- Parse OFMX source files.
- Transform source data into an internal canonical model.
- Export deterministic custom XML output.
- Keep the architecture testable, extensible, and easy to document.

## Architecture

Pipeline flow:

XML branch:

1. Ingest OFMX input (`internal/ingest`)
2. Map into output model (`internal/transform`)
3. Validate output model against `configs/output.xsd` semantic constraints (`internal/output/schema.go`)
4. Serialize to XML (`internal/output/xml_writer.go`)

Map branch:

1. Ingest OFMX input (`internal/ingest`)
2. Map into map dataset (`internal/transform/map_mapper.go`)
3. Serialize aviation GeoJSON runtime sources (`internal/output/geojson_writer.go`)
4. Invoke tilemaker for PMTiles generation (`internal/output/tilemaker_runner.go`)

Terrain branch:

1. Ingest Copernicus DEM files (`internal/ingest/dem_source_inventory.go`)
2. Build deterministic preprocessing plan (`internal/transform/terrain_planner.go`)
3. Run GDAL pipeline + PMTiles packaging (`internal/output/terrain_runner.go`)
4. Validate coverage/seams/elevation checks/metadata consistency (`internal/output/terrain_validator.go`)
5. Emit `terrain.manifest.json` + optional build report (`internal/output/terrain_metadata.go`)

Main binary entrypoint: `cmd/ofmx-parser/main.go`

## Directory Layout

```text
cmd/ofmx-parser/           CLI entrypoint
internal/app/              Application orchestration
internal/config/           CLI and file config parsing
internal/ingest/           Input file readers/parsers
internal/domain/           Canonical models and typed errors
internal/transform/        Mapping logic and transformation rules
internal/output/           XML writer and schema validation hooks
internal/pipeline/         End-to-end parse -> map -> export flow
test/integration/          Integration tests
test/fixtures/             Input/output fixtures (kept empty initially)
docs/                      Architecture and mapping documentation
```

## Requirements

- Go 1.24+
- tilemaker available in PATH (required only for PMTiles mode)

## Quick Start

Build:

```bash
make build
```

Run tests:

```bash
make test
```

Run parser:

```bash
go run ./cmd/ofmx-parser --input path/to/input.ofmx --output path/to/output.xml
```

Run parser and emit M1 parse report (snapshot metadata + feature counts):

```bash
go run ./cmd/ofmx-parser --input path/to/input.ofmx --output path/to/output.xml --report path/to/report.json
```

Run map generation (PMTiles) with tilemaker (strict-fail if tilemaker is missing):

```bash
go run ./cmd/ofmx-parser \
  --input path/to/input.ofmx \
  --pbf-input path/to/base.osm.pbf \
  --pmtiles-output path/to/output.pmtiles
```

Run both XML and PMTiles in one invocation:

```bash
go run ./cmd/ofmx-parser \
  --input path/to/input.ofmx \
  --output path/to/output.xml \
  --pbf-input path/to/base.osm.pbf \
  --pmtiles-output path/to/output.pmtiles
```

Run terrain preprocessing and PMTiles packaging (Copernicus DEM):

```bash
go run ./cmd/ofmx-parser \
  --terrain-source-dir path/to/copernicus-dem \
  --terrain-aoi-bbox 12.10,48.50,18.90,51.10 \
  --terrain-version COPDEM_GLO30_2024_1 \
  --terrain-pmtiles-output path/to/terrain.pmtiles \
  --terrain-manifest-output path/to/terrain.manifest.json \
  --terrain-build-report-output path/to/build-report.json
```

Dual mode ingest behavior:

- OFMX input is parsed once and reused for both XML and PMTiles branches.

Optional config file path:

```bash
go run ./cmd/ofmx-parser --input path/to/input.ofmx --output path/to/output.xml --config configs/parser.example.yaml
```

Config auto-discovery (when `--config` is omitted):

- `configs/parser.yaml`
- `configs/parser.yml`
- `configs/parser.example.yaml`

The first existing file is loaded.

## Configuration

Example file: `configs/parser.example.yaml`

Supported config fields:

- `transform.airspace.allowed_types`: optional replacement list for exported `Ase/AseUid/codeType` values.
  - If omitted, built-in defaults are used.
  - If provided, it replaces defaults for both XML airspace export and map zone/border export.
  - Values are normalized to uppercase and deduplicated.
- `transform.airspace.max_altitude_fl`: optional maximum allowed lower airspace limit in flight levels.
  - Default is `95`.
  - Minimum allowed value is `95`.
  - Lower limit conversion rules:
    - `SFC`/`AGL`/`HEI` => `FL 0`
    - `STD`/`FL` => value treated as flight level
    - `MSL`/`AMSL`/`ALT`/`FT` => value treated as feet and converted to `floor(feet/100)`

Map-related flags:

- `--arc-max-chord-m`: maximum chord length (meters) for OFMX arc/circle densification (default: `750`)
- `--pbf-input`: OSM PBF input path for tilemaker
- `--pmtiles-output`: PMTiles output path
- `--tilemaker-bin`: tilemaker binary path/name (default: `tilemaker`)
- `--tilemaker-config`: optional custom tilemaker config override
- `--tilemaker-process`: optional custom tilemaker process.lua override
- `--geojson-output-dir`: optional directory where only generated GeoJSON layer files are copied for debugging
- `--map-temp-dir`: optional temp directory for generated GeoJSON/config/process files
  - when omitted, a temporary directory is created automatically and removed after map generation
  - when provided, generated runtime files are kept in the specified directory

Terrain-related flags:

- `--terrain-source-dir`: directory with Copernicus DEM `*.tif/*.tiff` source files
- `--terrain-source-checksums`: optional checksum file (`sha256 filename`) for source integrity checks
- `--terrain-aoi-bbox`: AOI bounds in WGS84 (`minLon,minLat,maxLon,maxLat`)
- `--terrain-version`: source/version identifier included in manifest/report
- `--terrain-pmtiles-output`: output PMTiles path
- `--terrain-manifest-output`: optional terrain manifest output path (default `terrain.manifest.json` next to PMTiles)
- `--terrain-build-report-output`: optional machine-readable build report path
- `--terrain-min-zoom`, `--terrain-max-zoom`: terrain pyramid zoom range (default fallback matches OSM map range: `5-10`)
- `--terrain-tile-size`: tile size (`256` or `512`)
- `--terrain-encoding`: encoding marker in manifest (default `terrarium`)
- `--terrain-vertical-datum`: vertical datum label in manifest (default `EGM2008`)
- `--terrain-build-timestamp`: optional RFC3339 timestamp for deterministic metadata
- `--terrain-control-points`: optional CSV (`lon,lat,elev_m`) for RMSE quality gate
- `--terrain-seam-threshold`: maximum allowed edge seam pixel delta

## Testing Strategy

- Unit tests for parsing, mapping, and writer components.
- Integration tests for full pipeline behavior.
- Fixtures in `test/fixtures/input` and `test/fixtures/expected`.

## Thesis-Oriented Documentation

- `docs/architecture.md` - system boundaries and component responsibilities
- `docs/specification.md` - formal output XML specification (thesis appendix candidate)
- `docs/mapping-spec.md` - mapping rules from OFMX to custom XML
- `docs/map-spec.md` - formal PMTiles layer and aviation overlay specification
- `docs/map-pipeline-design.md` - map pipeline runtime and CLI contract
- `docs/terrain-pipeline-design.md` - Copernicus DEM terrain preprocessing runtime contract
- `docs/appendix-index.md` - thesis appendix index with suggested citation text
- `docs/decisions/` - architecture decision records

## Roadmap

- Extend OFMX feature coverage beyond current mapped entities.
- Add fixture sets for OFMX variants and edge-case geometry.
- Add tilemaker profile variants for deployment-specific map styles.
- Improve diagnostics/reporting for unresolved references and skipped features.
