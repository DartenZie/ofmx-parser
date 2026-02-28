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
	mapLayersMaxZoom     = 14
	mapLayersBaseZoom    = 14
	mapLayersMinZoom     = 0
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

	if err := os.WriteFile(processPath, []byte(tilemakerProcessLua), 0o644); err != nil {
		return "", "", domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to write tilemaker process file %q", processPath), err)
	}

	return configPath, processPath, nil
}

func buildTilemakerConfigJSON(artifacts domain.MapGeoJSONArtifacts) ([]byte, error) {
	type layerConfig struct {
		MinZoom int    `json:"minzoom"`
		MaxZoom int    `json:"maxzoom"`
		Source  string `json:"source,omitempty"`
	}

	config := map[string]any{
		"layers": map[string]layerConfig{
			"landuse-residential":       {MinZoom: 6, MaxZoom: mapLayersMaxZoom},
			"landcover_grass":           {MinZoom: 6, MaxZoom: mapLayersMaxZoom},
			"landcover_wood":            {MinZoom: 6, MaxZoom: mapLayersMaxZoom},
			"water":                     {MinZoom: 6, MaxZoom: mapLayersMaxZoom},
			"water_intermittent":        {MinZoom: 8, MaxZoom: mapLayersMaxZoom},
			"waterway":                  {MinZoom: 8, MaxZoom: mapLayersMaxZoom},
			"waterway-tunnel":           {MinZoom: 10, MaxZoom: mapLayersMaxZoom},
			"waterway_intermittent":     {MinZoom: 10, MaxZoom: mapLayersMaxZoom},
			"road_major_motorway":       {MinZoom: 5, MaxZoom: mapLayersMaxZoom},
			"road_trunk_primary":        {MinZoom: 7, MaxZoom: mapLayersMaxZoom},
			"road_secondary_tertiary":   {MinZoom: 9, MaxZoom: mapLayersMaxZoom},
			"place_label_city":          {MinZoom: 5, MaxZoom: mapLayersMaxZoom},
			"place_label_other":         {MinZoom: 9, MaxZoom: mapLayersMaxZoom},
			"aviation_airports":         {MinZoom: 6, MaxZoom: mapLayersMaxZoom, Source: artifacts.AirportsPath},
			"aviation_zones":            {MinZoom: 6, MaxZoom: mapLayersMaxZoom, Source: artifacts.ZonesPath},
			"aviation_poi":              {MinZoom: 8, MaxZoom: mapLayersMaxZoom, Source: artifacts.PointsOfInterestPath},
			"aviation_airspace_borders": {MinZoom: 6, MaxZoom: mapLayersMaxZoom, Source: artifacts.AirspaceBordersPath},
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

const tilemakerProcessLua = `
node_keys = { "place" }
way_keys  = { "landuse", "natural", "waterway", "intermittent", "tunnel", "highway" }

local place_other = {
  village = true,
  hamlet = true,
  suburb = true,
  quarter = true,
  neighbourhood = true,
  locality = true
}

function node_function()
  local place = Find("place")
  if place == "city" or place == "town" then
    Layer("place_label_city", false)
    Attribute("class", place)
    Attribute("name", Find("name"))
    return
  end

  if place_other[place] then
    Layer("place_label_other", false)
    Attribute("class", place)
    Attribute("name", Find("name"))
    return
  end
end

function way_function()
  local landuse = Find("landuse")
  local natural = Find("natural")
  local waterway = Find("waterway")
  local highway = Find("highway")
  local tunnel = Find("tunnel")
  local intermittent = Find("intermittent")
  local is_closed = IsClosed()

  if landuse == "residential" and is_closed then
    Layer("landuse-residential", true)
  end

  if is_closed and (natural == "grassland" or natural == "grass" or landuse == "meadow") then
    Layer("landcover_grass", true)
  end

  if is_closed and (natural == "wood" or landuse == "forest") then
    Layer("landcover_wood", true)
  end

  if is_closed and (natural == "water" or waterway == "riverbank") then
    Layer("water", true)
    if intermittent == "yes" then
      Layer("water_intermittent", true)
    end
  end

  if waterway ~= "" and not is_closed then
    Layer("waterway", false)
    if tunnel == "yes" then
      Layer("waterway-tunnel", false)
    end
    if intermittent == "yes" then
      Layer("waterway_intermittent", false)
    end
  end

  if highway == "motorway" then
    Layer("road_major_motorway", false)
  elseif highway == "trunk" or highway == "primary" then
    Layer("road_trunk_primary", false)
  elseif highway == "secondary" or highway == "tertiary" then
    Layer("road_secondary_tertiary", false)
  end
end
`
