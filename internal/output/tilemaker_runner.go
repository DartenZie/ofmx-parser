// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pašek
package output

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

const (
	defaultTilemakerBin  = "tilemaker"
	generatedConfigName  = "tilemaker.generated.config.json"
	generatedProcessName = "tilemaker.generated.process.lua"
	defaultConfigsDir    = "configs"
	defaultProcessName   = "tilemaker.process.lua"
	mapLayersMaxZoom     = 10
	mapLayersBaseZoom    = 10
	mapLayersMinZoom     = 5
)

// TilemakerRunner executes tilemaker to build PMTiles.
type TilemakerRunner interface {
	Run(ctx context.Context, req domain.MapExportRequest, artifacts domain.MapGeoJSONArtifacts) error
}

// ExecTilemakerRunner runs tilemaker via os/exec.
type ExecTilemakerRunner struct{}

// Run validates runtime requirements, generates config/process if needed, and executes tilemaker.
func (r ExecTilemakerRunner) Run(ctx context.Context, req domain.MapExportRequest, artifacts domain.MapGeoJSONArtifacts) error {
	bin := strings.TrimSpace(req.TilemakerBin)
	if bin == "" {
		bin = defaultTilemakerBin
	}

	if _, err := exec.LookPath(bin); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("tilemaker binary %q not found (strict-fail map mode)", bin), err)
	}

	configPath := strings.TrimSpace(req.TilemakerConfig)
	processPath := strings.TrimSpace(req.TilemakerProcess)
	if configPath == "" || processPath == "" {
		workDir := req.TempDir
		if strings.TrimSpace(workDir) == "" {
			workDir = os.TempDir()
		}

		generatedConfigPath, generatedProcessPath, err := generateTilemakerRuntimeFiles(workDir, artifacts)
		if err != nil {
			return err
		}
		if configPath == "" {
			configPath = generatedConfigPath
		}
		if processPath == "" {
			processPath = generatedProcessPath
		}
	}

	args := []string{
		"--input", req.PBFInputPath,
		"--output", req.PMTilesOutputPath,
		"--config", configPath,
		"--process", processPath,
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("tilemaker failed: %v: %s", err, strings.TrimSpace(string(output))), err)
	}

	if _, err := os.Stat(req.PMTilesOutputPath); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("tilemaker did not produce PMTiles output %q", req.PMTilesOutputPath), err)
	}

	return nil
}

func generateTilemakerRuntimeFiles(dir string, artifacts domain.MapGeoJSONArtifacts) (string, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create tilemaker runtime dir %q", dir), err)
	}

	configPath := filepath.Join(dir, generatedConfigName)
	processPath := filepath.Join(dir, generatedProcessName)

	configContent, err := buildTilemakerConfigJSON(artifacts)
	if err != nil {
		return "", "", err
	}

	if err := os.WriteFile(configPath, append(configContent, '\n'), 0o644); err != nil {
		return "", "", domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to write tilemaker config %q", configPath), err)
	}

	processContent, err := loadDefaultTilemakerProcessLua()
	if err != nil {
		return "", "", err
	}

	if err := os.WriteFile(processPath, processContent, 0o644); err != nil {
		return "", "", domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to write tilemaker process file %q", processPath), err)
	}

	return configPath, processPath, nil
}

func loadDefaultTilemakerProcessLua() ([]byte, error) {
	processTemplatePath, err := resolveConfigPath(defaultProcessName)
	if err != nil {
		return nil, domain.NewError(domain.ErrOutput, "failed to resolve default tilemaker process.lua template", err)
	}

	processContent, err := os.ReadFile(processTemplatePath)
	if err != nil {
		return nil, domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to read default tilemaker process template %q", processTemplatePath), err)
	}

	return processContent, nil
}

func resolveConfigPath(name string) (string, error) {
	relativePath := filepath.Join(defaultConfigsDir, name)
	if _, err := os.Stat(relativePath); err == nil {
		return relativePath, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, relativePath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			break
		}
	}

	return "", fmt.Errorf("unable to locate %q from %q", relativePath, wd)
}

func buildTilemakerConfigJSON(artifacts domain.MapGeoJSONArtifacts) ([]byte, error) {
	type layerConfig struct {
		MinZoom       int     `json:"minzoom"`
		MaxZoom       int     `json:"maxzoom"`
		Source        string  `json:"source,omitempty"`
		SourceColumns bool    `json:"source_columns,omitempty"`
		SimplifyBelow int     `json:"simplify_below,omitempty"`
		SimplifyLevel float64 `json:"simplify_level,omitempty"`
		FilterBelow   int     `json:"filter_below,omitempty"`
		FilterArea    float64 `json:"filter_area,omitempty"`
	}

	config := map[string]any{
		"layers": map[string]layerConfig{
			// Consolidated OpenMapTiles-compatible OSM layers with class attributes.
			// Polygon layers: simplify geometry + drop tiny features at low zoom.
			"landuse": {
				MinZoom: 6, MaxZoom: mapLayersMaxZoom,
				SimplifyBelow: 10, SimplifyLevel: 0.0003,
				FilterBelow: 10, FilterArea: 0.00005,
			},
			"landcover": {
				MinZoom: 6, MaxZoom: mapLayersMaxZoom,
				SimplifyBelow: 10, SimplifyLevel: 0.0003,
				FilterBelow: 10, FilterArea: 0.00005,
			},
			"water": {
				MinZoom: 8, MaxZoom: mapLayersMaxZoom,
				SimplifyBelow: 10, SimplifyLevel: 0.0001,
				FilterBelow: 10, FilterArea: 0.00001,
			},
			// Line layers: simplify geometry at low zoom.
			"waterway": {
				MinZoom: 6, MaxZoom: mapLayersMaxZoom,
				SimplifyBelow: 10, SimplifyLevel: 0.0001,
			},
			"transportation": {
				MinZoom: 5, MaxZoom: mapLayersMaxZoom,
				SimplifyBelow: 10, SimplifyLevel: 0.0001,
			},
			// Point layer: no simplification (MinZoom set per-feature in Lua).
			"place": {MinZoom: 5, MaxZoom: mapLayersMaxZoom},
			// Aviation overlay layers from GeoJSON sources.
			"aviation_airports":         {MinZoom: 6, MaxZoom: mapLayersMaxZoom, Source: artifacts.AirportsPath, SourceColumns: true},
			"aviation_zones":            {MinZoom: 6, MaxZoom: mapLayersMaxZoom, Source: artifacts.ZonesPath, SourceColumns: true},
			"aviation_poi":              {MinZoom: 8, MaxZoom: mapLayersMaxZoom, Source: artifacts.PointsOfInterestPath, SourceColumns: true},
			"aviation_airspace_borders": {MinZoom: 6, MaxZoom: mapLayersMaxZoom, Source: artifacts.AirspaceBordersPath, SourceColumns: true},
			"countries_boundary": {
				MinZoom: 5, MaxZoom: mapLayersMaxZoom, Source: artifacts.CountriesBoundaryPath, SourceColumns: true,
				SimplifyBelow: 10, SimplifyLevel: 0.0001,
			},
		},
		"settings": map[string]any{
			"minzoom":     mapLayersMinZoom,
			"maxzoom":     mapLayersMaxZoom,
			"basezoom":    mapLayersBaseZoom,
			"include_ids": false,
			"compress":    "gzip",
			"name":        "OFMX aviation map",
			"version":     "1.0",
			"description": "PMTiles with selected OpenMapTiles layers and OFMX aviation overlays",
		},
	}

	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, domain.NewError(domain.ErrOutput, "failed to marshal generated tilemaker config", err)
	}

	return b, nil
}
