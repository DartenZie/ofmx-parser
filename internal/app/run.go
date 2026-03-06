// Package app wires CLI input to the parser pipeline.
//
// Author: Miroslav Pašek
package app

import (
	"context"

	"github.com/DartenZie/ofmx-parser/internal/config"
	"github.com/DartenZie/ofmx-parser/internal/domain"
	"github.com/DartenZie/ofmx-parser/internal/ingest"
	"github.com/DartenZie/ofmx-parser/internal/output"
	"github.com/DartenZie/ofmx-parser/internal/pipeline"
	"github.com/DartenZie/ofmx-parser/internal/transform"
)

// Run executes one CLI invocation of the parser application.
func Run(ctx context.Context, args []string) error {
	cfg, err := config.ParseArgs(args)
	if err != nil {
		return err
	}

	fileCfg := config.FileConfig{}

	if cfg.ConfigPath != "" {
		loadedCfg, err := config.LoadFile(cfg.ConfigPath)
		if err != nil {
			return err
		}
		fileCfg = loadedCfg
	}

	reader := ingest.FileReader{ArcMaxChordLengthMeters: cfg.ArcMaxChordM}
	xmlMapper := transform.DefaultMapper{
		AllowedAirspaceTypes: fileCfg.Transform.Airspace.AllowedTypes,
		MaxAirspaceLowerFL:   fileCfg.EffectiveAirspaceMaxAltitudeFL(),
	}
	mapMapper := transform.DefaultMapMapper{
		AllowedAirspaceTypes: fileCfg.Transform.Airspace.AllowedTypes,
		MaxAirspaceLowerFL:   fileCfg.EffectiveAirspaceMaxAltitudeFL(),
	}

	if cfg.OutputPath != "" {
		runner := pipeline.New(
			reader,
			xmlMapper,
			output.XMLFileWriter{},
		)

		result, err := runner.Execute(ctx, cfg.InputPath, cfg.OutputPath)
		if err != nil {
			return err
		}

		if cfg.ReportPath != "" {
			if err := (output.JSONReportWriter{}).Write(ctx, result.Report, cfg.ReportPath); err != nil {
				return err
			}
		}
	}

	if cfg.PMTilesOutputPath != "" {
		doc, err := reader.Read(ctx, cfg.InputPath)
		if err != nil {
			return err
		}

		mapReq := domain.MapExportRequest{
			PBFInputPath:      cfg.PBFInputPath,
			PMTilesOutputPath: cfg.PMTilesOutputPath,
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
	}

	return nil
}
