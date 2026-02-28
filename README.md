# OFMX Parser (CLI)

`ofmx-parser` is a CLI-focused Go project for parsing OFMX data and producing a custom XML output format for bachelor thesis work.

The initial structure is intentionally layered so the mapping logic can grow without refactoring core wiring.

## Project Goals

- Parse OFMX source files.
- Transform source data into an internal canonical model.
- Export deterministic custom XML output.
- Keep the architecture testable, extensible, and easy to document.

## Architecture

Pipeline flow:

1. Ingest OFMX input (`internal/ingest`)
2. Map into output model (`internal/transform`)
3. Validate output model against `configs/output.xsd` semantic constraints (`internal/output/schema.go`)
4. Serialize to XML (`internal/output/xml_writer.go`)

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

Optional config file path (reserved for extended behavior):

```bash
go run ./cmd/ofmx-parser --input path/to/input.ofmx --output path/to/output.xml --config configs/parser.example.yaml
```

## Configuration

Example file: `configs/parser.example.yaml`

The config file is available in the project structure for future parser/mapping options. Current CLI behavior is driven primarily by flags.

## Testing Strategy

- Unit tests for parsing, mapping, and writer components.
- Integration tests for full pipeline behavior.
- Fixtures in `test/fixtures/input` and `test/fixtures/expected`.

## Thesis-Oriented Documentation

- `docs/architecture.md` - system boundaries and component responsibilities
- `docs/specification.md` - formal output XML specification (thesis appendix candidate)
- `docs/mapping-spec.md` - mapping rules from OFMX to custom XML
- `docs/decisions/` - architecture decision records

## Roadmap

- Extend OFMX feature coverage beyond current mapped entities.
- Add fixture sets for OFMX variants and edge-case geometry.
- Add strict and profile-based validation modes.
- Improve diagnostics/reporting for unresolved references and skipped features.
