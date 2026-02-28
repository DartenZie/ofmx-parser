// Package app wires CLI input to the parser pipeline.
//
// Author: Miroslav Pašek
package app

import (
	"context"

	"github.com/DartenZie/ofmx-parser/internal/config"
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

	if cfg.ConfigPath != "" {
		if _, err := config.LoadFile(cfg.ConfigPath); err != nil {
			return err
		}
	}

	runner := pipeline.New(
		ingest.FileReader{},
		transform.DefaultMapper{},
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

	return nil
}
