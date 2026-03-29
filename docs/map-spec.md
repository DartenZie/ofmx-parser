# Map Layer Specification

## 1. Scope

This document specifies the vector tile layer contract for PMTiles output.
It includes selected OpenMapTiles-derived base layers and OFMX-derived aviation overlay layers.

This specification is normative for map export behavior.

Status: implemented through milestones M5.1-M5.8.

## 2. Tile Source Strategy

- Base layers are sourced from OSM PBF through tilemaker OpenMapTiles-compatible processing.
- Aviation layers are sourced from parser-generated GeoJSON files.
- Output container format: PMTiles.

## 3. OpenMapTiles-Derived Layer Contract

The following OpenMapTiles-compatible source layers are produced by the embedded
tilemaker Lua/JSON configuration. Each layer carries a `class` attribute (and
additional filter attributes where noted) so the map style can filter features
at render time.

### 3.1 landuse

- geometry: Polygon
- attributes: `class` (string)
- included classes: `residential`
- minzoom: 6
- simplification: geometry simplified below z10; tiny polygons filtered below z10

### 3.2 landcover

- geometry: Polygon
- attributes: `class` (string)
- included classes: `grass`, `wood`
- minzoom: 6
- simplification: geometry simplified below z10; tiny polygons filtered below z10

### 3.3 water

- geometry: Polygon
- attributes: `intermittent` (integer, 0 or 1), `brunnel` (string, optional)
- minzoom: 6
- simplification: geometry simplified below z10; tiny polygons filtered below z10

### 3.4 waterway

- geometry: LineString
- attributes: `intermittent` (integer, 0 or 1), `brunnel` (string, optional)
- minzoom: 8
- simplification: geometry simplified below z10

### 3.5 transportation

- geometry: LineString
- attributes: `class` (string)
- included classes: `motorway`, `trunk`, `primary`, `secondary`, `tertiary`
- minzoom: 5
- simplification: geometry simplified below z10
- per-feature MinZoom: `motorway` z5, `trunk`/`primary` z7, `secondary` z9, `tertiary` z10

### 3.6 place

- geometry: Point
- attributes: `class` (string), `name` (string)
- included classes: `city`, `town`, `village`, `hamlet`, `suburb`, `quarter`, `neighbourhood`, `locality`
- minzoom: 5
- per-feature MinZoom: `city` z5, `town` z7, `village`/`suburb` z9, `hamlet`/`quarter`/`neighbourhood`/`locality` z10

## 4. Aviation Overlay Layers

### 4.1 aviation_airports

- geometry: Point
- source: OFMX airport features
- mandatory attributes:
  - `id` (string)
  - `name` (string)
  - `type` (string)
  - `elev_m` (number)

### 4.2 aviation_zones

- geometry: Polygon
- source: OFMX airspace features + border geometry
- type filter: only OFMX airspaces with `codeType IN {ATZ, CTR, TMA, D, P, PR, R, TRA, TRA_GA, TSA}` are included
  - configurable replacement via `transform.airspace.allowed_types`
- lower-limit filter: only airspaces with lower limit `<= max_altitude_fl` (FL) are included
  - configurable via `transform.airspace.max_altitude_fl`
  - conversion: `SFC/AGL/HEI => FL0`, `STD/FL => FL value`, `MSL/AMSL/ALT/FT => floor(feet/100)`
- curved OFMX borders (`CWA`/`CCA` arcs and `Circle`) are exported as deterministic densified polygon rings
- `Abd` records linked to the same airspace id are stitched when endpoints connect;
  if multiple disconnected rings remain, the largest ring is used for `aviation_zones` polygon geometry
- polygon normalization removes only consecutive duplicate points and an explicit closing duplicate
- mandatory attributes:
  - `id` (string)
  - `name` (string)
  - `zone_type` (string)
  - `class` (string)
  - `low_m` (number)
  - `low_ref` (string)
  - `up_m` (number)
  - `up_ref` (string)

### 4.3 aviation_poi

- geometry: Point
- source: OFMX navaids + designated points + obstacles
- mandatory attributes:
  - `id` (string)
  - `kind` (string; e.g. `VOR`, `NDB`, `OBSTACLE`, `DESIGNATED`)
  - `name` (string)

### 4.4 aviation_airspace_borders

- geometry: LineString
- source: derived from filtered airspace polygon edges after deduplication
- mandatory attributes:
  - `edge_id` (string, deterministic)
  - `zone_a` (string)
  - `zone_b` (string, optional when edge is non-shared)
  - `zone_type` (string; type of `zone_a`)
  - `shared` (boolean)

### 4.5 countries_boundary

- geometry: LineString
- source: OFMX `Gbr` (geographical border) features
- includes all parsed `Gbr` borders with at least two valid `WGE` points
- mandatory attributes:
  - `uid` (string)
  - `name` (string)

## 5. Shared Border Rule (Normative)

Common borders of airspaces MUST be rendered as one logical line in the final map data.

Algorithmic requirements:

1. Polygon edges are treated as undirected segments.
2. Segment orientation MUST be normalized (`A-B` equivalent to `B-A`).
3. Segment keys MUST use deterministic coordinate quantization.
4. Equal normalized keys MUST produce exactly one output border feature.

Implementation detail:

- Quantization factor is `1e6` in both latitude and longitude before edge key construction.
- Edge IDs are deterministic canonical strings based on quantized endpoint coordinates.

## 6. Determinism Requirements

To support reproducible thesis outputs:

- feature ordering MUST be stable,
- generated IDs MUST be deterministic,
- border deduplication MUST be deterministic under equal input.

## 7. Versioning

Future changes to this specification should include:

- explicit version marker,
- updated mapping rules,
- regression fixtures for affected layers.
