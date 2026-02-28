# Architecture

## Overview

The parser is a CLI-first pipeline that converts OFMX input into custom XML output.

The architecture isolates responsibilities so mapping growth does not force broad refactors.

## Components

- `cmd/ofmx-parser`: process entrypoint and exit behavior
- `internal/app`: top-level runtime composition
- `internal/config`: CLI argument parsing and optional config loading
- `internal/ingest`: input source reading/parsing
- `internal/domain`: canonical models and typed errors
- `internal/transform`: mapping rules from OFMX model to output model
- `internal/output`: schema validation hooks and XML serialization
- `internal/pipeline`: ordered orchestration of read -> map -> validate -> write

## Data Flow

1. User provides CLI arguments.
2. Input file is loaded by `ingest`.
3. `transform` maps input into output domain representation.
4. `output` validator checks structural correctness.
5. XML is written to destination path.

## Design Principles

- Deterministic transformation output.
- Typed errors with contextual diagnostics.
- Clear package boundaries to support testing and extension.
- Fixture-driven validation of mapping behavior.
