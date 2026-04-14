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
go run ./cmd/ofmx-parser --config configs/parser.example.yaml --output path/to/output.xml
```

Config auto-discovery (when `--config` is omitted):

- `configs/parser.yaml`
- `configs/parser.yml`
- `configs/parser.example.yaml`

The first existing file is loaded.

## Configuration

Example file: `configs/parser.example.yaml`

The file is grouped into these sections:

- `ofmx`: shared OFMX input settings, including `input` and `arc_max_chord_m`.
- `xml`: XML output settings such as `output` and `report`.
- `map`: PMTiles map settings including `pbf_input`, `pmtiles_output`, `geojson_output_dir`, `temp_dir`, and nested `tilemaker` overrides.
- `terrain`: Copernicus DEM terrain settings, including output paths, zoom/tile parameters, quality gates, clipping options, and nested `toolchain` binary paths.
- `transform`: transformation rules such as airspace allowlist filtering and lower-limit thresholding.

All current defaults are written explicitly in `configs/parser.example.yaml` together with inline comments.

Config precedence is:

1. Built-in defaults.
2. Values loaded from the YAML config file.
3. Explicit CLI flags.

This means the config file can fully drive a run by itself, while any flag you pass on the command line still overrides the matching YAML field.

Example config-driven XML run:

```bash
go run ./cmd/ofmx-parser --config configs/parser.example.yaml
```

Example overriding just one value from the config file:

```bash
go run ./cmd/ofmx-parser --config configs/parser.example.yaml --output path/to/output.xml
```

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
