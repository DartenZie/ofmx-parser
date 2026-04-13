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
	TerrainGDALDEMBin          string
	TerrainGDALInfoBin         string
	TerrainGDALLocationInfoBin string
	TerrainPMTilesBin          string
}

// ParseArgs parses CLI flags into CLIConfig and validates required arguments.
func ParseArgs(args []string) (CLIConfig, error) {
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
	terrainGDALDEM := fs.String("terrain-gdaldem-bin", "gdaldem", "gdaldem binary path/name")
	terrainGDALInfo := fs.String("terrain-gdalinfo-bin", "gdalinfo", "gdalinfo binary path/name")
	terrainGDALLocationInfo := fs.String("terrain-gdallocationinfo-bin", "gdallocationinfo", "gdallocationinfo binary path/name")
	terrainPMTilesBin := fs.String("terrain-pmtiles-bin", "pmtiles", "pmtiles binary path/name")

	if err := fs.Parse(args); err != nil {
		return CLIConfig{}, domain.NewError(domain.ErrConfig, "invalid CLI arguments", err)
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
		TerrainGDALDEMBin:          *terrainGDALDEM,
		TerrainGDALInfoBin:         *terrainGDALInfo,
		TerrainGDALLocationInfoBin: *terrainGDALLocationInfo,
		TerrainPMTilesBin:          *terrainPMTilesBin,
	}

	if err := (&cfg).Validate(); err != nil {
		return CLIConfig{}, err
	}

	return cfg, nil
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
	Transform TransformConfig `yaml:"transform" json:"transform"`
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
