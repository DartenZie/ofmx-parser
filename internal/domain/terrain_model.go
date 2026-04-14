// Package domain defines shared domain models and typed errors.
//
// Author: Miroslav Pasek
package domain

import "time"

// TerrainExportRequest defines terrain export inputs independent of CLI flags.
type TerrainExportRequest struct {
	AOIBounds               BoundingBox
	Version                 string
	SourceDir               string
	SourceChecksumsPath     string
	PMTilesOutputPath       string
	ManifestOutputPath      string
	BuildReportOutputPath   string
	BuildDir                string
	Encoding                string
	TileSize                int
	MinZoom                 int
	MaxZoom                 int
	VerticalDatum           string
	SchemaVersion           string
	NodataFillMaxDistance   int
	NodataFillSmoothingIter int
	SeamPixelThreshold      uint8
	RMSEThresholdM          float64
	ControlPointsPath       string
	BuildTimestamp          time.Time
	GDAL2TilesProcesses     int
	// ElevationQuantizationM rounds elevation values to the nearest multiple of
	// this value (in metres) before Terrarium RGB encoding. This reduces blue-
	// channel entropy and improves PNG compression. Set to 0 to disable
	// quantization (default). Typical values: 0.5, 1.0, 2.0.
	ElevationQuantizationM float64
	// ClipPolygonPath is an optional path to a GeoJSON or Shapefile polygon
	// used to mask tiles outside the AOI border (e.g. exact country outline).
	// When empty, all tiles within the AOI bounding box are included.
	// If the file contains LineString geometry (e.g. the countries_boundary.geojson
	// produced by the map pipeline) it is automatically converted to a convex-hull
	// polygon before use; see ClipPolygonCountryName.
	ClipPolygonPath string
	// ClipPolygonCountryName is an optional filter applied when converting a
	// LineString border file to a clip polygon. Only features whose "name"
	// property contains this string (case-sensitive) are included in the convex
	// hull. When empty, all features in the file are used.
	// Typical value when auto-wired from the map pipeline: "CZECHREPUBLIC".
	ClipPolygonCountryName string
	Toolchain              TerrainToolchain
}

// BoundingBox stores WGS84 AOI bounds.
type BoundingBox struct {
	MinLon float64
	MinLat float64
	MaxLon float64
	MaxLat float64
}

// TerrainToolchain stores binary names/paths for external terrain tooling.
type TerrainToolchain struct {
	GDALBuildVRTBin     string
	GDALFillNodataBin   string
	GDALWarpBin         string
	GDALTranslateBin    string
	GDALAddoBin         string
	GDALCalcBin         string
	GDALMergeBin        string
	GDAL2TilesBin       string
	GDALDEMBin          string
	GDALInfoBin         string
	GDALLocationInfoBin string
	PMTilesBin          string
}

// DEMSourceFile describes one source DEM input file.
type DEMSourceFile struct {
	Path           string
	RelativePath   string
	SizeBytes      int64
	SHA256Checksum string
}

// DEMSourceInventory contains validated source file metadata.
type DEMSourceInventory struct {
	Files []DEMSourceFile
}

// TerrainBuildPlan captures deterministic preprocessing choices.
type TerrainBuildPlan struct {
	MosaicVRTPath          string
	FilledDEMPath          string
	WarpedDEMPath          string
	QuantizedDEMPath       string // set only when ElevationQuantizationM > 0
	TilesDir               string
	AOIBounds              BoundingBox
	Encoding               string
	TileSize               int
	MinZoom                int
	MaxZoom                int
	NodataDistance         int
	NodataSmoothing        int
	BuildTimestamp         time.Time
	VerticalDatum          string
	SchemaVersion          string
	SourceVersion          string
	ElevationQuantizationM float64
	ClipPolygonPath        string
	ClipPolygonCountryName string
}

// TerrainBuildArtifacts contains runtime artifacts produced by the runner.
type TerrainBuildArtifacts struct {
	PMTilesPath   string
	TilesDir      string
	FilledDEMPath string
	WarpedDEMPath string
}

// TerrainManifest is the machine-readable terrain release metadata.
type TerrainManifest struct {
	SchemaVersion  string     `json:"schema_version"`
	Version        string     `json:"source_version"`
	BuildTimestamp string     `json:"build_timestamp"`
	Bounds         [4]float64 `json:"bounds"`
	MinZoom        int        `json:"min_zoom"`
	MaxZoom        int        `json:"max_zoom"`
	Encoding       string     `json:"encoding"`
	TileSize       int        `json:"tile_size"`
	VerticalDatum  string     `json:"vertical_datum"`
	// QuantizationM is the elevation quantization step used during build (0 = none).
	QuantizationM   float64  `json:"quantization_m,omitempty"`
	PMTilesChecksum string   `json:"pmtiles_sha256"`
	SourceFileCount int      `json:"source_file_count"`
	SourceChecksums []string `json:"source_checksums"`
}

// TerrainValidationResult contains quality gate outputs.
type TerrainValidationResult struct {
	CoverageOK            bool    `json:"coverage_ok"`
	MissingTiles          int     `json:"missing_tiles"`
	MaxSeamDelta          uint8   `json:"max_seam_delta"`
	SeamsOK               bool    `json:"seams_ok"`
	RMSEm                 float64 `json:"rmse_m"`
	ControlPointsCompared int     `json:"control_points_compared"`
	ElevationChecksOK     bool    `json:"elevation_checks_ok"`
	RasterSanityOK        bool    `json:"raster_sanity_ok"`
	MetadataConsistencyOK bool    `json:"metadata_consistency_ok"`
}

// TerrainBuildReport stores final machine-readable build diagnostics.
type TerrainBuildReport struct {
	Version        string                  `json:"version"`
	BuildTimestamp string                  `json:"build_timestamp"`
	ManifestPath   string                  `json:"manifest_path"`
	PMTilesPath    string                  `json:"pmtiles_path"`
	Validation     TerrainValidationResult `json:"validation"`
	SourceFiles    []DEMSourceFile         `json:"source_files"`
}
