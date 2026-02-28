# Architecture

## Overview

The parser is a CLI-first pipeline that converts OFMX input into two optional outputs:

- custom XML (`NavSnapshot`),
- PMTiles map package (selected OpenMapTiles base layers plus aviation overlays).

The architecture isolates responsibilities so mapping growth does not force broad refactors.

## Components

- `cmd/ofmx-parser`: process entrypoint and exit behavior
- `internal/app`: top-level runtime composition
- `internal/config`: CLI argument parsing and optional config loading
- `internal/ingest`: input source reading/parsing
- `internal/domain`: canonical models and typed errors
- `internal/transform`: mapping rules from OFMX model to XML and map intermediate models
- `internal/output`: XML/JSON/GeoJSON serialization and tilemaker invocation
- `internal/pipeline`: orchestration for XML and map export branches

## Data Flow

XML branch:

1. User provides CLI arguments.
2. Input file is loaded by `ingest`.
3. `transform` maps input into XML domain representation.
4. `output` validator checks structural correctness.
5. XML is written to destination path.

Map branch:

1. Input file is loaded by `ingest`.
2. `transform` maps input to map dataset.
3. `output` writes aviation GeoJSON artifacts.
4. `output` invokes tilemaker with OSM PBF + runtime config/process.
5. PMTiles artifact is written to destination path.

## Design Principles

- Deterministic transformation output.
- Typed errors with contextual diagnostics.
- Clear package boundaries to support testing and extension.
- Fixture-driven validation of mapping behavior.
- Strict-fail policy for requested external map tooling (tilemaker).
