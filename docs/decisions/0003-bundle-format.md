# ADR 0003: ZIP-Based Bundle Container Format (.ofpkg)

## Status

Accepted

## Context

The parser can produce up to six output files in a single invocation: NavSnapshot XML, parse report JSON, map PMTiles, terrain PMTiles, terrain manifest JSON, and terrain build report JSON. Distributing multiple loose files is error-prone and complicates integrity verification.

A single self-describing package is needed for portable, verifiable distribution of all artifacts from one pipeline run.

## Decision

Use a standard ZIP64 archive as the container format (extension `.ofpkg`):

1. Internal layout follows a fixed directory structure (`payload/`, `reports/`).
2. A required `manifest.json` (first entry) describes schema version, artifact metadata, checksums, and snapshot-level fields.
3. A required `checksums.sha256` (last entry) provides GNU sha256sum-compatible integrity lines.
4. Binary blobs (PMTiles) use `STORE` compression; text files (XML, JSON) use `DEFLATE`.
5. All ZIP entries use a fixed timestamp (`2001-01-01T00:00:00Z`), fixed permissions (`0644`), and stable entry order to guarantee byte-reproducible archives given identical inputs.

### Alternatives Considered

- **Custom binary format**: rejected because it would require dedicated tooling and lose compatibility with standard archive tools.
- **tar.gz**: rejected because tar does not support random access, and compression would prevent efficient extraction of large PMTiles blobs.
- **Modified ZIP (custom headers/magic)**: rejected because it would break standard ZIP readers for no practical benefit over a manifest-driven approach.

## Consequences

- Bundle is readable and extractable with any ZIP-compatible tool (unzip, 7-Zip, OS built-in extractors).
- Integrity can be verified using the embedded `checksums.sha256` without custom tooling.
- Schema evolution is handled via `manifest.schemaVersion` without binary format changes.
- When `--bundle-output` is specified, individual artifact files are not left on the filesystem; they are written to a staging directory, packed into the archive, and cleaned up.
- A verify/unpack subcommand is not implemented in this version; it is deferred as a future extension.
- No new external dependencies are introduced (uses Go stdlib `archive/zip`).
