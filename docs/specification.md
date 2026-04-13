# Output XML Specification

## 1. Purpose and Scope

This document specifies the target XML format produced by the OFMX parser for the bachelor thesis project.
The format is intended for deterministic export of selected aeronautical navigation data into a compact, machine-readable snapshot.

This specification is normative for output generation and validation.
Unless stated otherwise, requirements in this document use the terms MUST, SHOULD, and MAY in the RFC 2119 sense.

## 2. Normative Schema Reference

The normative XML Schema Definition (XSD) for this format is:

- `configs/output.xsd`

Any XML instance conforming to this specification MUST validate against that schema.

## 3. Document Model Overview

The root element is `NavSnapshot`.

Top-level content model:

- `NavSnapshot`
  - optional `Airports`
    - zero or more `Airport`
  - optional `Navaids`
    - zero or more `Navaid`
  - optional `Airspaces`
    - zero or more `Airspace`
  - optional `Obstacles`
    - zero or more `Obstacle`

Root attributes:

- `cycle` (required)
- `region` (required)
- `generatedAt` (required, `xs:dateTime`)
- `schema` (required)
- `source` (optional)

## 4. Primitive and Enumeration Types

### 4.1 Coordinate and Numeric Types

- `LatitudeType`: decimal in `[-90, 90]`
- `LongitudeType`: decimal in `[-180, 180]`
- `BearingType`: decimal in `[0, 360]`
- `PositiveDecimalType`: decimal `>= 0`
- `NonEmptyStringType`: non-empty string (`minLength = 1`)

### 4.2 Enumerations

- `RadioTypeEnum`: `COM | TWR`
- `FreqUnitEnum`: `MHZ | KHZ | GHZ`
- `NavaidTypeEnum`: `VOR | NDB | DME | DESIGNATED | MARKER | TACAN | UNKNOWN`
- `HeightRefEnum`: `AGL | MSL | FL | UNL | STD`
- `RunwayDirectionCodeEnum`: `N | NNE | NE | ENE | E | ESE | SE | SSE | S | SSW | SW | WSW | W | WNW | NW | NNW`

## 5. Element Specifications

### 5.1 NavSnapshot (root)

Attributes:

- `cycle` (required): non-empty cycle identifier string
- `region` (required): non-empty region identifier
- `generatedAt` (required): UTC timestamp in XML `dateTime` lexical format
- `schema` (required): schema identifier/version string
- `source` (optional): origin descriptor

Child containers are optional to support partial data snapshots.

### 5.2 Airports

`Airports` contains `Airport*`.

#### 5.2.1 Airport

Attributes:

- `id` (required): unique airport identifier in snapshot scope
- `d` (required): airport descriptor/type
- `n` (required): airport display name
- `lat` (required): latitude (`LatitudeType`)
- `lon` (required): longitude (`LongitudeType`)
- `elevM` (required): airport elevation in meters (`xs:decimal`)

Children:

- optional `Radios`
  - zero or more `Radio`
- optional `Runways`
  - zero or more `Runway`

#### 5.2.2 Radio

Attributes:

- `t` (required): radio category (`RadioTypeEnum`)
- `f` (required): frequency value (`PositiveDecimalType`)
- `u` (required): frequency unit (`FreqUnitEnum`)
- `c` (optional): channel value
- `l` (optional): language/label/local descriptor

#### 5.2.3 Runway

Attributes:

- `n` (required): runway designator
- `lenM` (required): length in meters (`PositiveDecimalType`)
- `widM` (required): width in meters (`PositiveDecimalType`)
- `comp` (optional): composition/surface descriptor
- `prep` (optional): preparation/treatment descriptor

Children:

- one or more `Dir`

#### 5.2.4 Dir (RunwayDirection)

Attributes:

- `brg` (required): runway direction bearing (`BearingType`)
- `code` (required): cardinal/ordinal code (`RunwayDirectionCodeEnum`)

### 5.3 Navaids

`Navaids` contains `Navaid*`.

#### 5.3.1 Navaid

Attributes:

- `id` (required): unique navaid identifier in snapshot scope
- `t` (required): navaid type (`NavaidTypeEnum`)
- `d` (required): descriptor/class/type detail
- `n` (required): display name
- `lat` (required): latitude (`LatitudeType`)
- `lon` (required): longitude (`LongitudeType`)

### 5.4 Airspaces

`Airspaces` contains `Airspace*`.

#### 5.4.1 Airspace

Attributes:

- `id` (required): unique airspace identifier in snapshot scope
- `d` (required): descriptor/class
- `n` (required): display name
- `t` (required): airspace type code
- `lowM` (required): lower vertical limit value in meters (`xs:decimal`)
- `lowRef` (required): lower vertical reference (`HeightRefEnum`)
- `upM` (required): upper vertical limit value in meters (`xs:decimal`)
- `upRef` (required): upper vertical reference (`HeightRefEnum`)
- `rmk` (optional): free-text remark

Children:

- `Poly` (required)
  - `P` (point) repeated, minimum 3
- `BBox` (required)

#### 5.4.2 Poly / P

`Poly` defines the horizontal boundary polygon.

Each `P` has:

- `lat` (required): latitude (`LatitudeType`)
- `lon` (required): longitude (`LongitudeType`)

At least three points are required.

#### 5.4.3 BBox

`BBox` is the axis-aligned bounding box for the polygon.

Attributes:

- `minLat`, `minLon`, `maxLat`, `maxLon` (all required)

### 5.5 Obstacles

`Obstacles` contains `Obstacle*`.

#### 5.5.1 Obstacle

Attributes:

- `id` (required): unique obstacle identifier in snapshot scope
- `t` (required): obstacle type/category
- `n` (required): obstacle display name
- `lat` (required): latitude (`LatitudeType`)
- `lon` (required): longitude (`LongitudeType`)
- `hM` (required): obstacle height in meters (`PositiveDecimalType`)
- `elevM` (required): obstacle elevation in meters (`xs:decimal`)

## 6. Conformance Requirements

An output document is conformant if all of the following hold:

1. It validates against `configs/output.xsd`.
2. Required attributes and required child elements are present.
3. All values satisfy declared numeric ranges and enumeration domains.
4. Geometry constraints are respected (for example `Airspace/Poly/P` count >= 3).

## 7. Determinism and Reproducibility

To support reproducible scientific/engineering evaluation, serialization SHOULD be deterministic.
The reference implementation applies stable ordering of major collections and deterministic derived values
(for example cardinal direction codes and airspace bounding boxes).

## 8. Example (Informative)

```xml
<NavSnapshot cycle="20260115" region="CZ" generatedAt="2026-01-15T00:00:00Z" schema="output.xsd" source="unit-test">
  <Airports>
    <Airport id="LKPR" d="AD" n="Prague Airport" lat="50.100556" lon="14.262222" elevM="380">
      <Runways>
        <Runway n="06/24" lenM="3715" widM="45">
          <Dir brg="58" code="ENE"/>
        </Runway>
      </Runways>
    </Airport>
  </Airports>
  <Navaids>
    <Navaid id="VLM" t="VOR" d="VOR" n="VLM VOR" lat="50.116667" lon="14.5"/>
  </Navaids>
</NavSnapshot>
```

## 9. Versioning and Evolution

Schema evolution SHOULD preserve backward compatibility where feasible.
Breaking changes (such as required attribute additions, enum contractions, or structural reorganization)
SHOULD be accompanied by:

- schema version increment,
- migration notes,
- updated mapping specification,
- regression fixtures.

## 10. Bundle Format (.ofpkg)

When the `--bundle-output` flag is provided, all produced artifacts are packaged into a single ZIP64-compliant archive with extension `.ofpkg`.

### 10.1 Internal Layout

```text
manifest.json            (required, first entry, DEFLATE)
payload/
  navsnapshot.xml        (present if XML branch ran, DEFLATE)
  map.pmtiles            (present if map branch ran, STORE)
  terrain.pmtiles        (present if terrain branch ran, STORE)
reports/
  report.json            (present if --report was given, DEFLATE)
  terrain.manifest.json  (present if terrain branch ran, DEFLATE)
  terrain.build-report.json (present if terrain build report path was given, DEFLATE)
checksums.sha256         (required, last entry, DEFLATE)
```

Binary blobs (`.pmtiles`) use `STORE` compression; all text files use `DEFLATE`.
Entry order is fixed. All entries use timestamp `2001-01-01T00:00:00Z` and permissions `0644`.

### 10.2 manifest.json

Required attributes:

- `schemaVersion` (string): manifest schema version (currently `"1.0.0"`)
- `generatedAt` (string): RFC 3339 UTC timestamp of archive creation
- `source` (string): always `"ofmx-parser"`
- `artifacts` (array of objects): one entry per packaged file
- `metadata` (object): snapshot-level fields (`cycle`, `region`)

Each artifact object:

- `role` (string): one of `navsnapshot`, `map-pmtiles`, `terrain-pmtiles`, `parse-report`, `terrain-manifest`, `terrain-build-report`
- `path` (string): archive-relative path
- `mediaType` (string): MIME type
- `compression` (string): `"deflate"` or `"store"`
- `sizeBytes` (integer): uncompressed file size in bytes
- `sha256` (string): hex-encoded SHA-256 of the uncompressed file content

### 10.3 checksums.sha256

GNU sha256sum-compatible format (`hash  path`), one line per artifact, sorted alphabetically by path.
Does not include `manifest.json` or `checksums.sha256` itself.

### 10.4 Determinism

When given identical input files and configuration, the archive SHOULD be byte-reproducible.
This is achieved through fixed timestamps, stable entry ordering, and deterministic JSON formatting.

### 10.5 Design Decision

See `docs/decisions/0003-bundle-format.md` for rationale and alternatives considered.
