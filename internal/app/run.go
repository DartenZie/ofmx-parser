// Package app wires CLI input to the parser pipeline.
//
// Author: Miroslav Pašek
package app

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/DartenZie/ofmx-parser/internal/config"
	"github.com/DartenZie/ofmx-parser/internal/domain"
	"github.com/DartenZie/ofmx-parser/internal/ingest"
	"github.com/DartenZie/ofmx-parser/internal/output"
	"github.com/DartenZie/ofmx-parser/internal/pipeline"
	"github.com/DartenZie/ofmx-parser/internal/transform"
)

// Run executes one CLI invocation of the parser application.
func Run(ctx context.Context, args []string) (runErr error) {
	startedAt := time.Now()
	defer func() {
		duration := time.Since(startedAt).Round(time.Millisecond)
		if runErr != nil {
			log.Printf("Process failed after %s", duration)
			return
		}
		log.Printf("Completed all requested work in %s", duration)
	}()

	cfg, err := config.ParseArgs(args)
	if err != nil {
		return err
	}

	fileCfg := config.FileConfig{}
	configPath := resolveConfigPath(cfg.ConfigPath, configPathExists)
	if configPath != "" {
		log.Printf("Loading config from %q", configPath)
		loadedCfg, err := config.LoadFile(configPath)
		if err != nil {
			return err
		}
		fileCfg = loadedCfg
	}

	runErr = runWithReader(ctx, cfg, fileCfg, ingest.FileReader{ArcMaxChordLengthMeters: cfg.ArcMaxChordM})
	return runErr
}

func resolveConfigPath(explicit string, exists func(string) bool) string {
	if explicit != "" {
		return explicit
	}

	for _, candidate := range []string{
		filepath.Join("configs", "parser.yaml"),
		filepath.Join("configs", "parser.yml"),
		filepath.Join("configs", "parser.example.yaml"),
	} {
		if exists(candidate) {
			return candidate
		}
	}

	return ""
}

func configPathExists(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !st.IsDir()
}

func runWithReader(ctx context.Context, cfg config.CLIConfig, fileCfg config.FileConfig, reader ingest.OFMXReader) error {
	xmlMapper := transform.DefaultMapper{
		AllowedAirspaceTypes: fileCfg.Transform.Airspace.AllowedTypes,
		MaxAirspaceLowerFL:   fileCfg.EffectiveAirspaceMaxAltitudeFL(),
	}
	mapMapper := transform.DefaultMapMapper{
		AllowedAirspaceTypes: fileCfg.Transform.Airspace.AllowedTypes,
		MaxAirspaceLowerFL:   fileCfg.EffectiveAirspaceMaxAltitudeFL(),
	}

	xmlRequested := cfg.OutputPath != ""
	mapRequested := cfg.PMTilesOutputPath != ""
	terrainRequested := cfg.TerrainPMTilesOutputPath != ""

	var (
		doc domain.OFMXDocument
		err error
	)
	if xmlRequested || mapRequested {
		parseStartedAt := time.Now()
		log.Printf("Parsing OFMX data from %q", cfg.InputPath)

		doc, err = reader.Read(ctx, cfg.InputPath)
		if err != nil {
			return err
		}
		log.Printf("Parsing OFMX data finished in %s", time.Since(parseStartedAt).Round(time.Millisecond))
	}

	if xmlRequested {
		xmlStartedAt := time.Now()
		log.Printf("Writing XML output to %q", cfg.OutputPath)

		runner := pipeline.New(
			reader,
			xmlMapper,
			output.XMLFileWriter{},
		)

		result, err := runner.ExecuteDocument(ctx, doc, cfg.OutputPath)
		if err != nil {
			return err
		}
		log.Printf("Writing XML output finished in %s", time.Since(xmlStartedAt).Round(time.Millisecond))

		if cfg.ReportPath != "" {
			reportStartedAt := time.Now()
			log.Printf("Writing parse report to %q", cfg.ReportPath)

			if err := (output.JSONReportWriter{}).Write(ctx, result.Report, cfg.ReportPath); err != nil {
				return err
			}
			log.Printf("Writing parse report finished in %s", time.Since(reportStartedAt).Round(time.Millisecond))
		}
	}

	if mapRequested {
		pmtilesStartedAt := time.Now()
		log.Printf("Writing PMTiles output to %q", cfg.PMTilesOutputPath)

		mapReq := domain.MapExportRequest{
			PBFInputPath:      cfg.PBFInputPath,
			PMTilesOutputPath: cfg.PMTilesOutputPath,
			GeoJSONOutputDir:  cfg.GeoJSONOutputDir,
			TilemakerBin:      cfg.TilemakerBin,
			TilemakerConfig:   cfg.TilemakerConfig,
			TilemakerProcess:  cfg.TilemakerProcess,
			TempDir:           cfg.MapTempDir,
		}

		mapSvc := pipeline.NewMapService(
			mapMapper,
			output.GeoJSONFileWriter{},
			output.ExecTilemakerRunner{},
		)
		if _, err := mapSvc.Execute(ctx, doc, mapReq); err != nil {
			return err
		}
		log.Printf("Writing PMTiles output finished in %s", time.Since(pmtilesStartedAt).Round(time.Millisecond))
	}

	if terrainRequested {
		terrainStartedAt := time.Now()
		log.Printf("Writing terrain PMTiles output to %q", cfg.TerrainPMTilesOutputPath)

		bbox, err := config.ParseBoundingBox(cfg.TerrainAOIBBox)
		if err != nil {
			return domain.NewError(domain.ErrConfig, "invalid terrain AOI bbox", err)
		}

		terrainReq := domain.TerrainExportRequest{
			AOIBounds:               bbox,
			Version:                 cfg.TerrainVersion,
			SourceDir:               cfg.TerrainSourceDir,
			SourceChecksumsPath:     cfg.TerrainSourceChecksumsPath,
			PMTilesOutputPath:       cfg.TerrainPMTilesOutputPath,
			ManifestOutputPath:      cfg.TerrainManifestOutputPath,
			BuildReportOutputPath:   cfg.TerrainBuildReportOutputPath,
			BuildDir:                cfg.TerrainBuildDir,
			Encoding:                cfg.TerrainEncoding,
			TileSize:                cfg.TerrainTileSize,
			MinZoom:                 cfg.TerrainMinZoom,
			MaxZoom:                 cfg.TerrainMaxZoom,
			VerticalDatum:           cfg.TerrainVerticalDatum,
			SchemaVersion:           cfg.TerrainSchemaVersion,
			NodataFillMaxDistance:   cfg.TerrainNodataFillMaxDistance,
			NodataFillSmoothingIter: cfg.TerrainNodataFillSmoothingIter,
			SeamPixelThreshold:      uint8(cfg.TerrainSeamPixelThreshold),
			RMSEThresholdM:          cfg.TerrainRMSEThresholdM,
			ControlPointsPath:       cfg.TerrainControlPointsPath,
			BuildTimestamp:          cfg.TerrainBuildTimestamp,
			Toolchain: domain.TerrainToolchain{
				GDALBuildVRTBin:     cfg.TerrainGDALBuildVRTBin,
				GDALFillNodataBin:   cfg.TerrainGDALFillNodataBin,
				GDALWarpBin:         cfg.TerrainGDALWarpBin,
				GDALTranslateBin:    cfg.TerrainGDALTranslateBin,
				GDALAddoBin:         cfg.TerrainGDALAddoBin,
				GDALCalcBin:         cfg.TerrainGDALCalcBin,
				GDALMergeBin:        cfg.TerrainGDALMergeBin,
				GDAL2TilesBin:       cfg.TerrainGDAL2TilesBin,
				GDALDEMBin:          cfg.TerrainGDALDEMBin,
				GDALInfoBin:         cfg.TerrainGDALInfoBin,
				GDALLocationInfoBin: cfg.TerrainGDALLocationInfoBin,
				PMTilesBin:          cfg.TerrainPMTilesBin,
			},
		}

		terrainSvc := pipeline.NewTerrainService(
			ingest.FileDEMSourcesIngestor{},
			transform.DefaultTerrainPlanner{},
			output.ExecTerrainRunner{},
			output.JSONTerrainMetadataWriter{},
			output.DefaultTerrainValidator{},
		)
		if _, err := terrainSvc.Execute(ctx, terrainReq); err != nil {
			return err
		}

		log.Printf("Writing terrain PMTiles output finished in %s", time.Since(terrainStartedAt).Round(time.Millisecond))
	}

	return nil
}
