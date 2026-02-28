# ADR 0001: Project Structure

## Status

Accepted

## Context

The project targets thesis-grade OFMX -> custom XML conversion where mapping rules are expected to grow over time.

Without strict boundaries, parsing, mapping, and serialization logic can become tightly coupled and harder to test.

## Decision

Use a CLI-only architecture with package boundaries inside `internal/`:

- `ingest` for input reading/parsing
- `transform` for mapping rules
- `output` for validation/serialization
- `pipeline` for orchestration only

Keep external surface minimal by exposing only CLI binary in `cmd/ofmx-parser`.

## Consequences

- Better testability and maintainability.
- Easier reasoning about mapping changes.
- Clear location for future schema validation and fixture expansion.
- No public package API at this stage (intentional).
