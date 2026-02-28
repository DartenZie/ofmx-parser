# AGENTS.md

This repository is structured for predictable, testable OFMX -> custom XML conversion work.

## Core Rules

1. Keep package boundaries strict:
   - `ingest` only reads/parses input.
   - `transform` only maps domain models.
   - `output` only validates/serializes output.
   - `pipeline` composes the flow; it should not hold mapping rules.
2. Add or update fixtures for every non-trivial mapping change.
3. Any mapping behavior change must update `docs/mapping-spec.md`.
4. Favor deterministic output (stable order, explicit defaults, no hidden randomness).
5. Return typed errors with useful context for CLI diagnostics.

## Contribution Workflow

1. Update domain model if new OFMX concepts are introduced.
2. Implement parsing in `internal/ingest`.
3. Implement mapping in `internal/transform`.
4. Implement/adjust XML serialization in `internal/output`.
5. Add tests (unit + integration as appropriate).
6. Update documentation (`README.md`, `docs/mapping-spec.md`, ADRs).

## Testing Expectations

- `go test ./...` must pass.
- New mapping logic should include at least one fixture-driven test.
- Keep tests readable and explicit; prefer table-driven tests when practical.

## Documentation Discipline

- Keep architecture docs aligned with code structure.
- Record major structural decisions in `docs/decisions`.
- Document assumptions and limitations, especially around OFMX variants.
