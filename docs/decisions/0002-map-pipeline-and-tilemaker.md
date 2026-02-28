# ADR 0002: PMTiles Map Pipeline with tilemaker

## Status

Accepted

## Context

The project requires a map artifact for thesis workflows in addition to XML export.
The map artifact must include selected base-map content from OSM PBF and OFMX-derived aviation overlays.

The selected output container is PMTiles.

## Decision

Introduce a second export branch in the application:

1. OFMX ingest into canonical domain model.
2. Transform into map intermediate model.
3. Emit GeoJSON runtime artifacts for aviation layers.
4. Execute tilemaker to produce PMTiles from OSM PBF plus generated overlays.

Use strict-fail behavior when PMTiles mode is requested and tilemaker is unavailable.

Implement shared airspace border deduplication as normalized, quantized undirected segments,
so common borders render as a single line.

## Consequences

- PMTiles output is available in map-only and dual (XML + map) modes.
- Build/runtime requires tilemaker installation for PMTiles mode.
- Deterministic tests now cover map CLI wiring, artifact generation, and tilemaker invocation contract.
- The architecture remains aligned with package boundaries (`ingest`/`transform`/`output`/`pipeline`).
