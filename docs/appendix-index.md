# Thesis Appendix Index

This index provides a concise entry point to the technical artifacts intended for the bachelor thesis appendices.

## Appendix A - Output XML Format

- Document: `docs/specification.md`
- Scope: formal specification of the `NavSnapshot` XML format.
- Normative schema: `configs/output.xsd`.

Suggested citation text:

"Appendix A defines the formal XML output contract (`NavSnapshot`) used by the conversion pipeline. All produced XML instances are expected to conform to the schema in `configs/output.xsd`."

## Appendix B - OFMX to XML Mapping Rules

- Document: `docs/mapping-spec.md`
- Scope: deterministic mapping rules from OFMX source structures into XML output fields.

Suggested citation text:

"Appendix B documents the transformation logic from OFMX entities to the custom XML model, including defaulting behavior, coordinate handling, and deterministic ordering constraints."

## Appendix C - PMTiles Layer Specification

- Document: `docs/map-spec.md`
- Scope: vector tile layer contract for PMTiles output (selected base layers + aviation overlays).

Suggested citation text:

"Appendix C specifies the PMTiles layer model, including source layer filters, aviation overlay attributes, and the normative shared-airspace-border deduplication rule."

## Appendix D - Map Pipeline Runtime Contract

- Document: `docs/map-pipeline-design.md`
- Scope: operational contract for OFMX + OSM PBF -> PMTiles generation with tilemaker.

Suggested citation text:

"Appendix D describes the implemented map-generation architecture, runtime artifacts, CLI contracts, and strict-fail behavior for missing external tooling."

## Appendix E - Terrain Preprocessing Pipeline Contract

- Document: `docs/terrain-pipeline-design.md`
- Scope: reproducible Copernicus DEM preprocessing into PMTiles terrain artifacts.

Suggested citation text:

"Appendix E defines the deterministic terrain preprocessing flow for Copernicus DEM, including integrity checks, quality gates (coverage/seams/elevation), and release metadata contracts."
