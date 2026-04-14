// Package config parses and validates CLI and file-based configuration.
//
// Author: Miroslav Pašek
package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DartenZie/ofmx-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

const (
	DefaultAirspaceMaxAltitudeFL = 95
	MinAirspaceMaxAltitudeFL     = 95
)

type CLIConfig struct {
	InputPath         string
	OutputPath        string
	ConfigPath        string
	ReportPath        string
	ArcMaxChordM      float64
	PBFInputPath      string
	PMTilesOutputPath string
	GeoJSONOutputDir  string
	TilemakerBin      string
	TilemakerConfig   string
	TilemakerProcess  string
	MapTempDir        string

	TerrainSourceDir               string
	TerrainSourceChecksumsPath     string
	TerrainAOIBBox                 string
	TerrainVersion                 string
	TerrainPMTilesOutputPath       string
	TerrainManifestOutputPath      string
	TerrainBuildReportOutputPath   string
	TerrainBuildDir                string
	TerrainMinZoom                 int
	TerrainMaxZoom                 int
	TerrainTileSize                int
	TerrainEncoding                string
	TerrainVerticalDatum           string
	TerrainSchemaVersion           string
	TerrainNodataFillMaxDistance   int
	TerrainNodataFillSmoothingIter int
	TerrainSeamPixelThreshold      int
	TerrainRMSEThresholdM          float64
	TerrainControlPointsPath       string
	TerrainBuildTimestampRaw       string
	TerrainBuildTimestamp          time.Time

	TerrainGDALBuildVRTBin     string
	TerrainGDALFillNodataBin   string
	TerrainGDALWarpBin         string
	TerrainGDALTranslateBin    string
	TerrainGDALAddoBin         string
	TerrainGDALCalcBin         string
	TerrainGDALMergeBin        string
	TerrainGDAL2TilesBin       string
	TerrainGDAL2TilesProcesses int
	TerrainGDALDEMBin          string
	TerrainGDALInfoBin         string
	TerrainGDALLocationInfoBin string
	TerrainPMTilesBin          string

	// Size-reduction options.
	TerrainElevationQuantizationM float64
	TerrainClipPolygonPath        string
	// TerrainClipPolygonCountryName filters border-line features by country name
	// when converting a LineString border file to a clip polygon. When empty all
	// features in the file are used. Ignored when the clip polygon already
	// contains polygon geometry.
	TerrainClipPolygonCountryName string
}

type ParsedArgs struct {
	Config        CLIConfig
	ExplicitFlags map[string]struct{}
}

// ParseArgs parses CLI flags and records which flags were explicitly provided.
func ParseArgs(args []string) (ParsedArgs, error) {
	fs := flag.NewFlagSet("ofmx-parser", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	input := fs.String("input", "", "Path to OFMX input file")
	output := fs.String("output", "", "Path to output XML file")
	configPath := fs.String("config", "", "Path to optional config file")
	reportPath := fs.String("report", "", "Path to optional parse report JSON output")
	arcMaxChord := fs.Float64("arc-max-chord-m", 750, "Maximum arc chord length in meters used when densifying OFMX arc/circle borders")
	pbfInput := fs.String("pbf-input", "", "Path to OSM PBF input for PMTiles generation")
	pmtilesOutput := fs.String("pmtiles-output", "", "Path to output PMTiles file")
	geojsonOutputDir := fs.String("geojson-output-dir", "", "Optional directory to persist only generated GeoJSON layer files for debugging")
	tilemakerBin := fs.String("tilemaker-bin", "tilemaker", "Tilemaker executable path/name")
	tilemakerConfig := fs.String("tilemaker-config", "", "Optional tilemaker config override")
	tilemakerProcess := fs.String("tilemaker-process", "", "Optional tilemaker process.lua override")
	mapTempDir := fs.String("map-temp-dir", "", "Optional map generation temporary directory")

	terrainSourceDir := fs.String("terrain-source-dir", "", "Directory containing Copernicus DEM source files (*.tif/*.tiff)")
	terrainSourceChecksums := fs.String("terrain-source-checksums", "", "Optional checksum file for source DEM integrity validation")
	terrainAOIBBox := fs.String("terrain-aoi-bbox", "", "AOI bbox in WGS84: minLon,minLat,maxLon,maxLat")
	terrainVersion := fs.String("terrain-version", "", "Terrain source/version identifier")
	terrainPMTilesOutput := fs.String("terrain-pmtiles-output", "", "Output PMTiles v3 path for terrain pyramid")
	terrainManifestOutput := fs.String("terrain-manifest-output", "", "Optional terrain manifest output path")
	terrainBuildReportOutput := fs.String("terrain-build-report-output", "", "Optional terrain build report JSON output path")
	terrainBuildDir := fs.String("terrain-build-dir", "", "Optional terrain build working directory")
	terrainMinZoom := fs.Int("terrain-min-zoom", 5, "Terrain tile pyramid minimum zoom (defaults to OSM map min zoom)")
	terrainMaxZoom := fs.Int("terrain-max-zoom", 10, "Terrain tile pyramid maximum zoom (defaults to OSM map max zoom)")
	terrainTileSize := fs.Int("terrain-tile-size", 256, "Terrain tile size (256 or 512)")
	terrainEncoding := fs.String("terrain-encoding", "terrarium", "Terrain tile encoding")
	terrainVerticalDatum := fs.String("terrain-vertical-datum", "EGM2008", "Terrain vertical datum label for manifest")
	terrainSchemaVersion := fs.String("terrain-schema-version", "1.0.0", "Terrain manifest schema version")
	terrainNodataFillDistance := fs.Int("terrain-nodata-fill-distance", 100, "Maximum pixel distance for deterministic nodata fill")
	terrainNodataFillSmoothing := fs.Int("terrain-nodata-fill-smoothing", 0, "Smoothing iterations for nodata fill")
	terrainSeamThreshold := fs.Int("terrain-seam-threshold", 8, "Max allowed edge seam pixel delta")
	terrainRMSEThreshold := fs.Float64("terrain-rmse-threshold-m", 25.0, "Maximum allowed RMSE for elevation control point checks (meters)")
	terrainControlPoints := fs.String("terrain-control-points", "", "Optional CSV with control points: lon,lat,elev_m")
	terrainBuildTimestamp := fs.String("terrain-build-timestamp", "", "Optional RFC3339 build timestamp for deterministic metadata")
	terrainGDALBuildVRT := fs.String("terrain-gdalbuildvrt-bin", "gdalbuildvrt", "gdalbuildvrt binary path/name")
	terrainGDALFillNodata := fs.String("terrain-gdal-fillnodata-bin", "gdal_fillnodata.py", "gdal_fillnodata.py binary path/name")
	terrainGDALWarp := fs.String("terrain-gdalwarp-bin", "gdalwarp", "gdalwarp binary path/name")
	terrainGDALTranslate := fs.String("terrain-gdal-translate-bin", "gdal_translate", "gdal_translate binary path/name")
	terrainGDALAddo := fs.String("terrain-gdaladdo-bin", "gdaladdo", "gdaladdo binary path/name")
	terrainGDALCalc := fs.String("terrain-gdal-calc-bin", "gdal_calc.py", "gdal_calc.py binary path/name")
	terrainGDALMerge := fs.String("terrain-gdal-merge-bin", "gdal_merge.py", "gdal_merge.py binary path/name")
	terrainGDAL2Tiles := fs.String("terrain-gdal2tiles-bin", "gdal2tiles.py", "gdal2tiles.py binary path/name")
	terrainGDAL2TilesProcesses := fs.Int("terrain-gdal2tiles-processes", 0, "Number of gdal2tiles.py worker processes (0 = use all CPUs)")
	terrainGDALDEM := fs.String("terrain-gdaldem-bin", "gdaldem", "gdaldem binary path/name")
	terrainGDALInfo := fs.String("terrain-gdalinfo-bin", "gdalinfo", "gdalinfo binary path/name")
	terrainGDALLocationInfo := fs.String("terrain-gdallocationinfo-bin", "gdallocationinfo", "gdallocationinfo binary path/name")
	terrainPMTilesBin := fs.String("terrain-pmtiles-bin", "pmtiles", "pmtiles binary path/name")
	terrainElevQuantization := fs.Float64("terrain-elevation-quantization-m", 0, "Round elevation to nearest N metres before Terrarium encoding to reduce tile size (0 = disabled)")
	terrainClipPolygon := fs.String("terrain-clip-polygon", "", "Path to GeoJSON/Shapefile polygon for clipping tiles outside AOI shape (e.g. country border)")
	terrainClipCountryName := fs.String("terrain-clip-country-name", "", "Country name filter when converting LineString border file to clip polygon (e.g. CZECHREPUBLIC)")

	if err := fs.Parse(args); err != nil {
		return ParsedArgs{}, domain.NewError(domain.ErrConfig, "invalid CLI arguments", err)
	}

	cfg := CLIConfig{
		InputPath:         *input,
		OutputPath:        *output,
		ConfigPath:        *configPath,
		ReportPath:        *reportPath,
		ArcMaxChordM:      *arcMaxChord,
		PBFInputPath:      *pbfInput,
		PMTilesOutputPath: *pmtilesOutput,
		GeoJSONOutputDir:  *geojsonOutputDir,
		TilemakerBin:      *tilemakerBin,
		TilemakerConfig:   *tilemakerConfig,
		TilemakerProcess:  *tilemakerProcess,
		MapTempDir:        *mapTempDir,

		TerrainSourceDir:               *terrainSourceDir,
		TerrainSourceChecksumsPath:     *terrainSourceChecksums,
		TerrainAOIBBox:                 *terrainAOIBBox,
		TerrainVersion:                 *terrainVersion,
		TerrainPMTilesOutputPath:       *terrainPMTilesOutput,
		TerrainManifestOutputPath:      *terrainManifestOutput,
		TerrainBuildReportOutputPath:   *terrainBuildReportOutput,
		TerrainBuildDir:                *terrainBuildDir,
		TerrainMinZoom:                 *terrainMinZoom,
		TerrainMaxZoom:                 *terrainMaxZoom,
		TerrainTileSize:                *terrainTileSize,
		TerrainEncoding:                *terrainEncoding,
		TerrainVerticalDatum:           *terrainVerticalDatum,
		TerrainSchemaVersion:           *terrainSchemaVersion,
		TerrainNodataFillMaxDistance:   *terrainNodataFillDistance,
		TerrainNodataFillSmoothingIter: *terrainNodataFillSmoothing,
		TerrainSeamPixelThreshold:      *terrainSeamThreshold,
		TerrainRMSEThresholdM:          *terrainRMSEThreshold,
		TerrainControlPointsPath:       *terrainControlPoints,
		TerrainBuildTimestampRaw:       *terrainBuildTimestamp,

		TerrainGDALBuildVRTBin:     *terrainGDALBuildVRT,
		TerrainGDALFillNodataBin:   *terrainGDALFillNodata,
		TerrainGDALWarpBin:         *terrainGDALWarp,
		TerrainGDALTranslateBin:    *terrainGDALTranslate,
		TerrainGDALAddoBin:         *terrainGDALAddo,
		TerrainGDALCalcBin:         *terrainGDALCalc,
		TerrainGDALMergeBin:        *terrainGDALMerge,
		TerrainGDAL2TilesBin:       *terrainGDAL2Tiles,
		TerrainGDAL2TilesProcesses: *terrainGDAL2TilesProcesses,
		TerrainGDALDEMBin:          *terrainGDALDEM,
		TerrainGDALInfoBin:         *terrainGDALInfo,
		TerrainGDALLocationInfoBin: *terrainGDALLocationInfo,
		TerrainPMTilesBin:          *terrainPMTilesBin,

		TerrainElevationQuantizationM: *terrainElevQuantization,
		TerrainClipPolygonPath:        *terrainClipPolygon,
		TerrainClipPolygonCountryName: *terrainClipCountryName,
	}

	explicitFlags := make(map[string]struct{})
	fs.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = struct{}{}
	})

	return ParsedArgs{Config: cfg, ExplicitFlags: explicitFlags}, nil
}

// Validate validates required CLI configuration fields.
func (c *CLIConfig) Validate() error {
	if c.InputPath == "" {
		if c.OutputPath != "" || c.PMTilesOutputPath != "" {
			return domain.NewError(domain.ErrConfig, "--input is required when XML/map OFMX outputs are requested", nil)
		}
	}

	if c.OutputPath == "" && c.PMTilesOutputPath == "" && c.TerrainPMTilesOutputPath == "" {
		return domain.NewError(domain.ErrConfig, "at least one output is required: --output, --pmtiles-output, or --terrain-pmtiles-output", nil)
	}

	if c.ArcMaxChordM <= 0 {
		return domain.NewError(domain.ErrConfig, "--arc-max-chord-m must be > 0", nil)
	}

	mapRequested := c.PBFInputPath != "" || c.PMTilesOutputPath != "" || c.GeoJSONOutputDir != "" || c.TilemakerConfig != "" || c.TilemakerProcess != "" || c.MapTempDir != ""
	if mapRequested {
		if c.PBFInputPath == "" {
			return domain.NewError(domain.ErrConfig, "--pbf-input is required when map generation is enabled", nil)
		}
		if c.PMTilesOutputPath == "" {
			return domain.NewError(domain.ErrConfig, "--pmtiles-output is required when map generation is enabled", nil)
		}
	}

	terrainRequested := c.TerrainPMTilesOutputPath != "" || c.TerrainSourceDir != "" || c.TerrainAOIBBox != "" || c.TerrainVersion != "" || c.TerrainManifestOutputPath != "" || c.TerrainBuildReportOutputPath != "" || c.TerrainBuildDir != "" || c.TerrainControlPointsPath != ""
	if terrainRequested {
		if c.TerrainSourceDir == "" {
			return domain.NewError(domain.ErrConfig, "--terrain-source-dir is required when terrain mode is enabled", nil)
		}
		if c.TerrainAOIBBox == "" {
			return domain.NewError(domain.ErrConfig, "--terrain-aoi-bbox is required when terrain mode is enabled", nil)
		}
		if _, err := ParseBoundingBox(c.TerrainAOIBBox); err != nil {
			return domain.NewError(domain.ErrConfig, "invalid --terrain-aoi-bbox", err)
		}
		if c.TerrainVersion == "" {
			return domain.NewError(domain.ErrConfig, "--terrain-version is required when terrain mode is enabled", nil)
		}
		if c.TerrainPMTilesOutputPath == "" {
			return domain.NewError(domain.ErrConfig, "--terrain-pmtiles-output is required when terrain mode is enabled", nil)
		}
		if c.TerrainMinZoom < 0 || c.TerrainMaxZoom < 0 || c.TerrainMinZoom > c.TerrainMaxZoom {
			return domain.NewError(domain.ErrConfig, "terrain zoom range is invalid", nil)
		}
		if c.TerrainTileSize != 256 && c.TerrainTileSize != 512 {
			return domain.NewError(domain.ErrConfig, "--terrain-tile-size must be 256 or 512", nil)
		}
		if c.TerrainNodataFillMaxDistance < 0 || c.TerrainNodataFillSmoothingIter < 0 {
			return domain.NewError(domain.ErrConfig, "terrain nodata fill parameters must be >= 0", nil)
		}
		if c.TerrainSeamPixelThreshold < 0 || c.TerrainSeamPixelThreshold > 255 {
			return domain.NewError(domain.ErrConfig, "--terrain-seam-threshold must be in range 0..255", nil)
		}
		if c.TerrainRMSEThresholdM < 0 {
			return domain.NewError(domain.ErrConfig, "--terrain-rmse-threshold-m must be >= 0", nil)
		}
		if c.TerrainElevationQuantizationM < 0 {
			return domain.NewError(domain.ErrConfig, "--terrain-elevation-quantization-m must be >= 0 (0 = disabled)", nil)
		}

		if strings.TrimSpace(c.TerrainBuildTimestampRaw) != "" {
			ts, err := time.Parse(time.RFC3339, c.TerrainBuildTimestampRaw)
			if err != nil {
				return domain.NewError(domain.ErrConfig, "--terrain-build-timestamp must be RFC3339", err)
			}
			c.TerrainBuildTimestamp = ts.UTC()
		}
	}

	return nil
}

// ParseBoundingBox parses bbox format minLon,minLat,maxLon,maxLat.
func ParseBoundingBox(raw string) (domain.BoundingBox, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return domain.BoundingBox{}, fmt.Errorf("expected 4 comma-separated values")
	}

	vals := [4]float64{}
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return domain.BoundingBox{}, fmt.Errorf("invalid bbox value %q: %w", p, err)
		}
		vals[i] = v
	}

	bbox := domain.BoundingBox{MinLon: vals[0], MinLat: vals[1], MaxLon: vals[2], MaxLat: vals[3]}
	if bbox.MinLon >= bbox.MaxLon || bbox.MinLat >= bbox.MaxLat {
		return domain.BoundingBox{}, fmt.Errorf("bbox min values must be less than max values")
	}
	if bbox.MinLon < -180 || bbox.MaxLon > 180 || bbox.MinLat < -90 || bbox.MaxLat > 90 {
		return domain.BoundingBox{}, fmt.Errorf("bbox out of WGS84 range")
	}

	return bbox, nil
}

// FileConfig stores optional file-based configuration.
type FileConfig struct {
	OFMX      OFMXFileConfig    `yaml:"ofmx" json:"ofmx"`
	XML       XMLFileConfig     `yaml:"xml" json:"xml"`
	Map       MapFileConfig     `yaml:"map" json:"map"`
	Terrain   TerrainFileConfig `yaml:"terrain" json:"terrain"`
	Transform TransformConfig   `yaml:"transform" json:"transform"`
}

type OFMXFileConfig struct {
	InputPath    *string  `yaml:"input" json:"input"`
	ArcMaxChordM *float64 `yaml:"arc_max_chord_m" json:"arc_max_chord_m"`
}

type XMLFileConfig struct {
	OutputPath *string `yaml:"output" json:"output"`
	ReportPath *string `yaml:"report" json:"report"`
}

type MapFileConfig struct {
	PBFInputPath      *string             `yaml:"pbf_input" json:"pbf_input"`
	PMTilesOutputPath *string             `yaml:"pmtiles_output" json:"pmtiles_output"`
	GeoJSONOutputDir  *string             `yaml:"geojson_output_dir" json:"geojson_output_dir"`
	TempDir           *string             `yaml:"temp_dir" json:"temp_dir"`
	Tilemaker         TilemakerFileConfig `yaml:"tilemaker" json:"tilemaker"`
}

type TilemakerFileConfig struct {
	Bin     *string `yaml:"bin" json:"bin"`
	Config  *string `yaml:"config" json:"config"`
	Process *string `yaml:"process" json:"process"`
}

type TerrainFileConfig struct {
	SourceDir               *string                    `yaml:"source_dir" json:"source_dir"`
	SourceChecksumsPath     *string                    `yaml:"source_checksums" json:"source_checksums"`
	AOIBBox                 *string                    `yaml:"aoi_bbox" json:"aoi_bbox"`
	Version                 *string                    `yaml:"version" json:"version"`
	PMTilesOutputPath       *string                    `yaml:"pmtiles_output" json:"pmtiles_output"`
	ManifestOutputPath      *string                    `yaml:"manifest_output" json:"manifest_output"`
	BuildReportOutputPath   *string                    `yaml:"build_report_output" json:"build_report_output"`
	BuildDir                *string                    `yaml:"build_dir" json:"build_dir"`
	MinZoom                 *int                       `yaml:"min_zoom" json:"min_zoom"`
	MaxZoom                 *int                       `yaml:"max_zoom" json:"max_zoom"`
	TileSize                *int                       `yaml:"tile_size" json:"tile_size"`
	Encoding                *string                    `yaml:"encoding" json:"encoding"`
	VerticalDatum           *string                    `yaml:"vertical_datum" json:"vertical_datum"`
	SchemaVersion           *string                    `yaml:"schema_version" json:"schema_version"`
	NodataFillMaxDistance   *int                       `yaml:"nodata_fill_distance" json:"nodata_fill_distance"`
	NodataFillSmoothingIter *int                       `yaml:"nodata_fill_smoothing" json:"nodata_fill_smoothing"`
	SeamPixelThreshold      *int                       `yaml:"seam_threshold" json:"seam_threshold"`
	RMSEThresholdM          *float64                   `yaml:"rmse_threshold_m" json:"rmse_threshold_m"`
	ControlPointsPath       *string                    `yaml:"control_points" json:"control_points"`
	BuildTimestampRaw       *string                    `yaml:"build_timestamp" json:"build_timestamp"`
	GDAL2TilesProcesses     *int                       `yaml:"gdal2tiles_processes" json:"gdal2tiles_processes"`
	ElevationQuantizationM  *float64                   `yaml:"elevation_quantization_m" json:"elevation_quantization_m"`
	ClipPolygonPath         *string                    `yaml:"clip_polygon" json:"clip_polygon"`
	ClipPolygonCountryName  *string                    `yaml:"clip_country_name" json:"clip_country_name"`
	Toolchain               TerrainToolchainFileConfig `yaml:"toolchain" json:"toolchain"`
}

type TerrainToolchainFileConfig struct {
	GDALBuildVRTBin     *string `yaml:"gdalbuildvrt_bin" json:"gdalbuildvrt_bin"`
	GDALFillNodataBin   *string `yaml:"gdal_fillnodata_bin" json:"gdal_fillnodata_bin"`
	GDALWarpBin         *string `yaml:"gdalwarp_bin" json:"gdalwarp_bin"`
	GDALTranslateBin    *string `yaml:"gdal_translate_bin" json:"gdal_translate_bin"`
	GDALAddoBin         *string `yaml:"gdaladdo_bin" json:"gdaladdo_bin"`
	GDALCalcBin         *string `yaml:"gdal_calc_bin" json:"gdal_calc_bin"`
	GDALMergeBin        *string `yaml:"gdal_merge_bin" json:"gdal_merge_bin"`
	GDAL2TilesBin       *string `yaml:"gdal2tiles_bin" json:"gdal2tiles_bin"`
	GDALDEMBin          *string `yaml:"gdaldem_bin" json:"gdaldem_bin"`
	GDALInfoBin         *string `yaml:"gdalinfo_bin" json:"gdalinfo_bin"`
	GDALLocationInfoBin *string `yaml:"gdallocationinfo_bin" json:"gdallocationinfo_bin"`
	PMTilesBin          *string `yaml:"pmtiles_bin" json:"pmtiles_bin"`
}

type TransformConfig struct {
	Airspace AirspaceTransformConfig `yaml:"airspace" json:"airspace"`
}

type AirspaceTransformConfig struct {
	AllowedTypes  []string `yaml:"allowed_types" json:"allowed_types"`
	MaxAltitudeFL *int     `yaml:"max_altitude_fl" json:"max_altitude_fl"`
}

func (c *FileConfig) normalize() {
	if len(c.Transform.Airspace.AllowedTypes) == 0 {
		return
	}

	normalized := make([]string, 0, len(c.Transform.Airspace.AllowedTypes))
	seen := make(map[string]struct{}, len(c.Transform.Airspace.AllowedTypes))
	for _, raw := range c.Transform.Airspace.AllowedTypes {
		v := strings.ToUpper(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		normalized = append(normalized, v)
	}

	c.Transform.Airspace.AllowedTypes = normalized
}

// ApplyTo merges file-based settings into CLI config unless the matching CLI
// flag was explicitly provided.
func (c FileConfig) ApplyTo(dst *CLIConfig, explicitFlags map[string]struct{}) {
	applyString(dst, c.OFMX.InputPath, explicitFlags, "input", func(cfg *CLIConfig, v string) { cfg.InputPath = v })
	applyFloat64(dst, c.OFMX.ArcMaxChordM, explicitFlags, "arc-max-chord-m", func(cfg *CLIConfig, v float64) { cfg.ArcMaxChordM = v })

	applyString(dst, c.XML.OutputPath, explicitFlags, "output", func(cfg *CLIConfig, v string) { cfg.OutputPath = v })
	applyString(dst, c.XML.ReportPath, explicitFlags, "report", func(cfg *CLIConfig, v string) { cfg.ReportPath = v })

	applyString(dst, c.Map.PBFInputPath, explicitFlags, "pbf-input", func(cfg *CLIConfig, v string) { cfg.PBFInputPath = v })
	applyString(dst, c.Map.PMTilesOutputPath, explicitFlags, "pmtiles-output", func(cfg *CLIConfig, v string) { cfg.PMTilesOutputPath = v })
	applyString(dst, c.Map.GeoJSONOutputDir, explicitFlags, "geojson-output-dir", func(cfg *CLIConfig, v string) { cfg.GeoJSONOutputDir = v })
	applyString(dst, c.Map.TempDir, explicitFlags, "map-temp-dir", func(cfg *CLIConfig, v string) { cfg.MapTempDir = v })
	applyString(dst, c.Map.Tilemaker.Bin, explicitFlags, "tilemaker-bin", func(cfg *CLIConfig, v string) { cfg.TilemakerBin = v })
	applyString(dst, c.Map.Tilemaker.Config, explicitFlags, "tilemaker-config", func(cfg *CLIConfig, v string) { cfg.TilemakerConfig = v })
	applyString(dst, c.Map.Tilemaker.Process, explicitFlags, "tilemaker-process", func(cfg *CLIConfig, v string) { cfg.TilemakerProcess = v })

	applyString(dst, c.Terrain.SourceDir, explicitFlags, "terrain-source-dir", func(cfg *CLIConfig, v string) { cfg.TerrainSourceDir = v })
	applyString(dst, c.Terrain.SourceChecksumsPath, explicitFlags, "terrain-source-checksums", func(cfg *CLIConfig, v string) { cfg.TerrainSourceChecksumsPath = v })
	applyString(dst, c.Terrain.AOIBBox, explicitFlags, "terrain-aoi-bbox", func(cfg *CLIConfig, v string) { cfg.TerrainAOIBBox = v })
	applyString(dst, c.Terrain.Version, explicitFlags, "terrain-version", func(cfg *CLIConfig, v string) { cfg.TerrainVersion = v })
	applyString(dst, c.Terrain.PMTilesOutputPath, explicitFlags, "terrain-pmtiles-output", func(cfg *CLIConfig, v string) { cfg.TerrainPMTilesOutputPath = v })
	applyString(dst, c.Terrain.ManifestOutputPath, explicitFlags, "terrain-manifest-output", func(cfg *CLIConfig, v string) { cfg.TerrainManifestOutputPath = v })
	applyString(dst, c.Terrain.BuildReportOutputPath, explicitFlags, "terrain-build-report-output", func(cfg *CLIConfig, v string) { cfg.TerrainBuildReportOutputPath = v })
	applyString(dst, c.Terrain.BuildDir, explicitFlags, "terrain-build-dir", func(cfg *CLIConfig, v string) { cfg.TerrainBuildDir = v })
	applyInt(dst, c.Terrain.MinZoom, explicitFlags, "terrain-min-zoom", func(cfg *CLIConfig, v int) { cfg.TerrainMinZoom = v })
	applyInt(dst, c.Terrain.MaxZoom, explicitFlags, "terrain-max-zoom", func(cfg *CLIConfig, v int) { cfg.TerrainMaxZoom = v })
	applyInt(dst, c.Terrain.TileSize, explicitFlags, "terrain-tile-size", func(cfg *CLIConfig, v int) { cfg.TerrainTileSize = v })
	applyString(dst, c.Terrain.Encoding, explicitFlags, "terrain-encoding", func(cfg *CLIConfig, v string) { cfg.TerrainEncoding = v })
	applyString(dst, c.Terrain.VerticalDatum, explicitFlags, "terrain-vertical-datum", func(cfg *CLIConfig, v string) { cfg.TerrainVerticalDatum = v })
	applyString(dst, c.Terrain.SchemaVersion, explicitFlags, "terrain-schema-version", func(cfg *CLIConfig, v string) { cfg.TerrainSchemaVersion = v })
	applyInt(dst, c.Terrain.NodataFillMaxDistance, explicitFlags, "terrain-nodata-fill-distance", func(cfg *CLIConfig, v int) { cfg.TerrainNodataFillMaxDistance = v })
	applyInt(dst, c.Terrain.NodataFillSmoothingIter, explicitFlags, "terrain-nodata-fill-smoothing", func(cfg *CLIConfig, v int) { cfg.TerrainNodataFillSmoothingIter = v })
	applyInt(dst, c.Terrain.SeamPixelThreshold, explicitFlags, "terrain-seam-threshold", func(cfg *CLIConfig, v int) { cfg.TerrainSeamPixelThreshold = v })
	applyFloat64(dst, c.Terrain.RMSEThresholdM, explicitFlags, "terrain-rmse-threshold-m", func(cfg *CLIConfig, v float64) { cfg.TerrainRMSEThresholdM = v })
	applyString(dst, c.Terrain.ControlPointsPath, explicitFlags, "terrain-control-points", func(cfg *CLIConfig, v string) { cfg.TerrainControlPointsPath = v })
	applyString(dst, c.Terrain.BuildTimestampRaw, explicitFlags, "terrain-build-timestamp", func(cfg *CLIConfig, v string) { cfg.TerrainBuildTimestampRaw = v })
	applyInt(dst, c.Terrain.GDAL2TilesProcesses, explicitFlags, "terrain-gdal2tiles-processes", func(cfg *CLIConfig, v int) { cfg.TerrainGDAL2TilesProcesses = v })
	applyFloat64(dst, c.Terrain.ElevationQuantizationM, explicitFlags, "terrain-elevation-quantization-m", func(cfg *CLIConfig, v float64) { cfg.TerrainElevationQuantizationM = v })
	applyString(dst, c.Terrain.ClipPolygonPath, explicitFlags, "terrain-clip-polygon", func(cfg *CLIConfig, v string) { cfg.TerrainClipPolygonPath = v })
	applyString(dst, c.Terrain.ClipPolygonCountryName, explicitFlags, "terrain-clip-country-name", func(cfg *CLIConfig, v string) { cfg.TerrainClipPolygonCountryName = v })

	applyString(dst, c.Terrain.Toolchain.GDALBuildVRTBin, explicitFlags, "terrain-gdalbuildvrt-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDALBuildVRTBin = v })
	applyString(dst, c.Terrain.Toolchain.GDALFillNodataBin, explicitFlags, "terrain-gdal-fillnodata-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDALFillNodataBin = v })
	applyString(dst, c.Terrain.Toolchain.GDALWarpBin, explicitFlags, "terrain-gdalwarp-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDALWarpBin = v })
	applyString(dst, c.Terrain.Toolchain.GDALTranslateBin, explicitFlags, "terrain-gdal-translate-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDALTranslateBin = v })
	applyString(dst, c.Terrain.Toolchain.GDALAddoBin, explicitFlags, "terrain-gdaladdo-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDALAddoBin = v })
	applyString(dst, c.Terrain.Toolchain.GDALCalcBin, explicitFlags, "terrain-gdal-calc-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDALCalcBin = v })
	applyString(dst, c.Terrain.Toolchain.GDALMergeBin, explicitFlags, "terrain-gdal-merge-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDALMergeBin = v })
	applyString(dst, c.Terrain.Toolchain.GDAL2TilesBin, explicitFlags, "terrain-gdal2tiles-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDAL2TilesBin = v })
	applyString(dst, c.Terrain.Toolchain.GDALDEMBin, explicitFlags, "terrain-gdaldem-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDALDEMBin = v })
	applyString(dst, c.Terrain.Toolchain.GDALInfoBin, explicitFlags, "terrain-gdalinfo-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDALInfoBin = v })
	applyString(dst, c.Terrain.Toolchain.GDALLocationInfoBin, explicitFlags, "terrain-gdallocationinfo-bin", func(cfg *CLIConfig, v string) { cfg.TerrainGDALLocationInfoBin = v })
	applyString(dst, c.Terrain.Toolchain.PMTilesBin, explicitFlags, "terrain-pmtiles-bin", func(cfg *CLIConfig, v string) { cfg.TerrainPMTilesBin = v })
}

func applyString(dst *CLIConfig, src *string, explicitFlags map[string]struct{}, flagName string, assign func(*CLIConfig, string)) {
	if dst == nil || src == nil || flagExplicit(explicitFlags, flagName) {
		return
	}
	assign(dst, *src)
}

func applyInt(dst *CLIConfig, src *int, explicitFlags map[string]struct{}, flagName string, assign func(*CLIConfig, int)) {
	if dst == nil || src == nil || flagExplicit(explicitFlags, flagName) {
		return
	}
	assign(dst, *src)
}

func applyFloat64(dst *CLIConfig, src *float64, explicitFlags map[string]struct{}, flagName string, assign func(*CLIConfig, float64)) {
	if dst == nil || src == nil || flagExplicit(explicitFlags, flagName) {
		return
	}
	assign(dst, *src)
}

func flagExplicit(explicitFlags map[string]struct{}, flagName string) bool {
	_, ok := explicitFlags[flagName]
	return ok
}

func (c FileConfig) EffectiveAirspaceMaxAltitudeFL() int {
	if c.Transform.Airspace.MaxAltitudeFL == nil {
		return DefaultAirspaceMaxAltitudeFL
	}
	return *c.Transform.Airspace.MaxAltitudeFL
}

func (c FileConfig) validate() error {
	if c.Transform.Airspace.MaxAltitudeFL != nil && *c.Transform.Airspace.MaxAltitudeFL < MinAirspaceMaxAltitudeFL {
		return fmt.Errorf("transform.airspace.max_altitude_fl must be >= %d", MinAirspaceMaxAltitudeFL)
	}
	return nil
}

// LoadFile loads a config file from disk.
func LoadFile(path string) (FileConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, domain.NewError(domain.ErrConfig, fmt.Sprintf("failed to read config file %q", path), err)
	}

	var cfg FileConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return FileConfig{}, domain.NewError(domain.ErrConfig, fmt.Sprintf("failed to parse config file %q", path), err)
	}
	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return FileConfig{}, domain.NewError(domain.ErrConfig, fmt.Sprintf("invalid config file %q", path), err)
	}

	return cfg, nil
}
