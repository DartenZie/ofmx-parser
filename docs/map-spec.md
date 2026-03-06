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

The following style-targeted layers are required.

### 3.1 Land and Water

- `landuse-residential`
  - source vector layer: `landuse`
  - filter: `class = residential`
  - geometry: Polygon

- `landcover_grass`
  - source vector layer: `landcover`
  - filter: `class = grass`
  - geometry: Polygon

- `landcover_wood`
  - source vector layer: `landcover`
  - filter: `class = wood`
  - geometry: Polygon

- `water`
  - source vector layer: `water`
  - filter: base water features (`class`-driven)
  - geometry: Polygon

- `water_intermittent`
  - source vector layer: `water`
  - filter: `intermittent = 1`
  - geometry: Polygon

- `waterway`
  - source vector layer: `waterway`
  - filter: all linear waterways
  - geometry: LineString

- `waterway-tunnel`
  - source vector layer: `waterway`
  - filter: `brunnel = tunnel`
  - geometry: LineString

- `waterway_intermittent`
  - source vector layer: `waterway`
  - filter: `intermittent = 1`
  - geometry: LineString

### 3.2 Roads

- `road_major_motorway`
  - source vector layer: `transportation`
  - filter: `class = motorway`
  - geometry: LineString

- `road_trunk_primary`
  - source vector layer: `transportation`
  - filter: `class IN {trunk, primary}`
  - geometry: LineString

- `road_secondary_tertiary`
  - source vector layer: `transportation`
  - filter: `class IN {secondary, tertiary}`
  - geometry: LineString

### 3.3 Place Labels

- `place_label_city`
  - source vector layer: `place`
  - filter: `class IN {city, town}`
  - geometry: Point

- `place_label_other`
  - source vector layer: `place`
  - filter: `class IN {village, hamlet, suburb, quarter, neighbourhood, locality}`
  - geometry: Point

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
- curved OFMX borders (`CWA`/`CCA` arcs and `Circle`) are exported as deterministic densified polygon rings
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
- source: derived from airspace polygon edges after deduplication
- mandatory attributes:
  - `edge_id` (string, deterministic)
  - `zone_a` (string)
  - `zone_b` (string, optional when edge is non-shared)
  - `shared` (boolean)

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
