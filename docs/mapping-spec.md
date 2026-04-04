# Mapping Specification

This document defines how OFMX data maps to the custom XML output model (`NavSnapshot`).

## Snapshot-Level Mapping (M2)

- `OFMX-Snapshot/@effective` -> `NavSnapshot/@cycle`
  - Rule: take `YYYY-MM-DD` from the effective timestamp and remove dashes (`2026-01-15` -> `20260115`).
- `OFMX-Snapshot/@regions` -> `NavSnapshot/@region`
  - Rule: first whitespace-separated token; fallback `GLOBAL`.
- `OFMX-Snapshot/@created` -> `NavSnapshot/@generatedAt`
  - Rule: pass through RFC3339 string.
- constant -> `NavSnapshot/@schema`
  - Rule: always `output.xsd`.
- `OFMX-Snapshot/@origin` -> `NavSnapshot/@source`.

## Airport Mapping (M2)

- `Ahp/AhpUid/codeId` -> `Airport/@id`
- `Ahp/codeType` -> `Airport/@d` (fallback `UNKNOWN`)
- `Ahp/txtName` -> `Airport/@n`
- `Ahp/geoLat` -> `Airport/@lat`
- `Ahp/geoLong` -> `Airport/@lon`
- `Ahp/valElev` -> `Airport/@elevM` (fallback `0`)

Coordinate rule for `geoLat` / `geoLong`:

- Primary format: parse OFMX schema-compliant decimal coordinates with hemisphere suffix
  (e.g. `49.123456N`, `014.123456E`).
- Compatibility fallback: parse legacy DMS-like compact format
  (e.g. `500602N`, `0141544E`).
- Apply negative sign for `S` and `W` hemispheres.
- Enforce coordinate bounds in ingest:
  - latitude `[-90, 90]`
  - longitude `[-180, 180]`

## Runway Mapping (M2)

- `Rwy` is attached to `Airport` by `Rwy/RwyUid/AhpUid/codeId`.
- `Rwy/RwyUid/txtDesig` -> `Runway/@n`
- `Rwy/valLen` -> `Runway/@lenM` (fallback `0`)
- `Rwy/valWid` -> `Runway/@widM` (fallback `0`)
- `Rwy/codeComposition` -> `Runway/@comp` (optional)
- `Rwy/codePreparation` -> `Runway/@prep` (optional)

Runway directions:

- Join `Rdn` to `Rwy` by `(airportID, runwayDesignation)` from `RdnUid/RwyUid`.
- Bearing source priority:
  1. `Rdn/valTrueBrg`
  2. `Rdn/valMagBrg`
  3. inferred from runway designator (e.g. `06` -> `60`)
- `Dir/@code` is derived from bearing using 16-point compass (`N`, `NNE`, ..., `NNW`).

## Navaid Mapping (M2)

Supported OFMX sources:

- `Vor` -> `Navaid/@t=VOR`
- `Ndb` -> `Navaid/@t=NDB`
- `Dme` -> `Navaid/@t=DME`
- `Tcn` -> `Navaid/@t=TACAN`
- `Mkr` -> `Navaid/@t=MARKER`
- `Dpn` -> `Navaid/@t=DESIGNATED`

Common target mapping:

- `Uid/codeId` -> `Navaid/@id`
- descriptive source field (type/class fallback) -> `Navaid/@d`
- `txtName` (fallback `codeId`) -> `Navaid/@n`
- `Uid/geoLat` -> `Navaid/@lat`
- `Uid/geoLong` -> `Navaid/@lon`

## Determinism Rules

- Airports sorted by `@id`.
- Runways sorted by `@n`.
- Runway directions sorted by bearing.
- Navaids sorted by `(@id, @t)`.

## Airspace Mapping (M4)

- `Ase/AseUid/codeId` -> `Airspace/@id`
- `Ase/AseUid/codeType` -> `Airspace/@t`
- `Ase/txtName` (fallback `codeId`) -> `Airspace/@n`
- `Ase/codeClass` (fallback `codeActivity`, `codeType`) -> `Airspace/@d`
- `Ase/valDistVerLower` -> `Airspace/@lowM`
- `Ase/codeDistVerLower` (fallback `Ase/uomDistVerLower`) -> `Airspace/@lowRef`
- `Ase/valDistVerUpper` -> `Airspace/@upM` (fallback lower value)
- `Ase/codeDistVerUpper` (fallback `Ase/uomDistVerUpper`) -> `Airspace/@upRef`
- `Ase/txtRmk` -> `Airspace/@rmk`
- Airspace type filter: only `ATZ`, `CTR`, `TMA`, `D`, `P`, `PR`, `R`, `TRA`, `TRA_GA`, `TSA` are exported.
  - Config override: `transform.airspace.allowed_types` replaces the default allowlist when provided.
- Airspace lower-limit filter: only airspaces with lower limit `<= max_altitude_fl` (FL) are exported.
  - Config key: `transform.airspace.max_altitude_fl`
  - Default: `95`; minimum allowed value: `95`
  - Lower limit conversion:
    - `SFC`/`AGL`/`HEI` => `FL 0`
    - `STD`/`FL` => value interpreted directly as FL
    - `MSL`/`AMSL`/`ALT`/`FT` => value interpreted as feet and converted to `floor(feet/100)`

Horizontal geometry:

- `Abd` records are joined to `Ase` via `Abd/AbdUid/AseUid/codeId`.
- `Abd/Avx/geoLat` + `Abd/Avx/geoLong` -> `Airspace/Poly/P` points.
- `Abd/Avx/codeType` determines segment geometry from vertex `i` to `i+1` in the closed ring.
  - `GRC`/`RHL`/`ABE` are treated as straight segments between vertices.
  - `CWA` (clockwise arc) and `CCA` (counter-clockwise arc) are densified into intermediate points using
    `geoLatArc`/`geoLongArc` as arc center.
  - Arc densification uses deterministic chord-based sampling with default `750 m` max chord length;
    CLI override: `--arc-max-chord-m`.
  - When provided, `valRadiusArc` + `uomRadiusArc` are used as arc radius (`NM`, `KM`, `M`, `FT`),
    otherwise radius is derived from center->start vertex distance.
- `Abd/Circle` is supported and expanded into a deterministic polygon ring using
  `geoLatCen`/`geoLongCen` + `valRadius`/`uomRadius`.
- `Abd/Avx/codeType=FNT` is treated as a frontier placeholder and resolved against `Gbr` geometry.
  - Border resolution key priority: `Avx/GbrUid/@mid`, fallback `Avx/GbrUid/txtName`.
  - `Gbr/Gbv` points are indexed once per dataset (`codeDatum=WGE`) and inserted into airspace borders.
  - Border copying starts at the FNT vertex coordinate and follows the referenced border toward the next `Avx` vertex.
  - If both start and stop anchors are within snap tolerance, only that border subpath is inserted.
  - If either anchor does not snap within tolerance, frontier expansion is skipped, a warning is emitted, and the original FNT coordinate is kept.
  - Consecutive duplicate coordinates are removed using coordinate epsilon.
  - Missing `Gbr` emits a structured warning and falls back to the original frontier coordinate.
- Polygon normalization removes only consecutive duplicate points and an explicit closing duplicate (`last == first`).
  Non-consecutive repeated points are preserved to avoid changing intended shape topology.
- `BBox` is computed from polygon min/max coordinates.
- Airspaces with fewer than 3 unique points are omitted from output.

Height reference mapping (`codeDistVer*` -> `HeightRefEnum`):

- contains `UNL` -> `UNL`
- contains `FL` -> `FL`
- contains `SFC` or `AGL` -> `AGL`
- contains `MSL` or `AMSL` -> `MSL`
- fallback -> `STD`

## Obstacle Mapping (M4)

- `Obs/ObsUid/@mid` (fallback `ObsUid/OgrUid/txtName`, then `Obs/txtName`) -> `Obstacle/@id`
- `Obs/codeType` -> `Obstacle/@t` (fallback `UNKNOWN`)
- `Obs/txtName` (fallback id) -> `Obstacle/@n`
- `Obs/ObsUid/geoLat` -> `Obstacle/@lat`
- `Obs/ObsUid/geoLong` -> `Obstacle/@lon`
- `Obs/valHgt` -> `Obstacle/@hM` (fallback `0`)
- `Obs/valElev` -> `Obstacle/@elevM` (fallback `0`)

Obstacle ordering:

- Obstacles sorted by `@id`.

## Map Export Mapping (M5)

Map export mapping (`OFMX -> map dataset -> GeoJSON -> PMTiles`) is specified in:

- `docs/map-spec.md` (normative layer contract)
- `docs/map-pipeline-design.md` (runtime and CLI contract)

Implemented map dataset entities:

- airports point layer,
- zones polygon layer (stitches connected `Abd` border parts per airspace; when disconnected rings exist, selects the largest ring for polygon export),
- points-of-interest point layer (navaids, designated points, obstacles),
  - navaid POI vocalic rule: if `codeId` consists only of letters `A-Z`, `name` is replaced with NATO phonetic words
    (`A` -> `Alpha`, `B` -> `Bravo`, ..., `Z` -> `Zulu`) and POI property `type` is set to `vocalic`,
- deduplicated + stitched airspace border line layer (includes `zone_type` derived from `zone_a`; contiguous edges with unchanged zone ownership are emitted as continuous LineStrings; shared edges are kept as one line only when adjacent zones have the same `zone_type`, otherwise the same geometry is emitted once per zone type),
- country boundary line layer from parsed `Gbr` features (all parsed borders with at least two valid points).
