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

- Parse OFMX DMS strings (e.g. `500602N`, `0141544E`) into decimal degrees.
- Apply negative sign for `S` and `W` hemispheres.

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
- `Ase/codeDistVerLower` -> `Airspace/@lowRef`
- `Ase/valDistVerUpper` -> `Airspace/@upM` (fallback lower value)
- `Ase/codeDistVerUpper` -> `Airspace/@upRef`
- `Ase/txtRmk` -> `Airspace/@rmk`

Horizontal geometry:

- `Abd` records are joined to `Ase` via `Abd/AbdUid/AseUid/codeId`.
- `Abd/Avx/geoLat` + `Abd/Avx/geoLong` -> `Airspace/Poly/P` points.
- Duplicate points are removed.
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
