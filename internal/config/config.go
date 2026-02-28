// Package config parses and validates CLI and file-based configuration.
//
// Author: Miroslav Pašek
package config

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

type CLIConfig struct {
	InputPath  string
	OutputPath string
	ConfigPath string
	ReportPath string
}

// ParseArgs parses CLI flags into CLIConfig and validates required arguments.
func ParseArgs(args []string) (CLIConfig, error) {
	fs := flag.NewFlagSet("ofmx-parser", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	input := fs.String("input", "", "Path to OFMX input file")
	output := fs.String("output", "", "Path to output XML file")
	configPath := fs.String("config", "", "Path to optional config file")
	reportPath := fs.String("report", "", "Path to optional parse report JSON output")

	if err := fs.Parse(args); err != nil {
		return CLIConfig{}, domain.NewError(domain.ErrConfig, "invalid CLI arguments", err)
	}

	cfg := CLIConfig{
		InputPath:  *input,
		OutputPath: *output,
		ConfigPath: *configPath,
		ReportPath: *reportPath,
	}

	if err := cfg.Validate(); err != nil {
		return CLIConfig{}, err
	}

	return cfg, nil
}

// Validate validates required CLI configuration fields.
func (c CLIConfig) Validate() error {
	if c.InputPath == "" {
		return domain.NewError(domain.ErrConfig, "--input is required", nil)
	}

	if c.OutputPath == "" {
		return domain.NewError(domain.ErrConfig, "--output is required", nil)
	}

	return nil
}

// FileConfig stores raw config file content for future extension.
type FileConfig struct {
	Raw []byte
}

// LoadFile loads a config file from disk.
func LoadFile(path string) (FileConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, domain.NewError(domain.ErrConfig, fmt.Sprintf("failed to read config file %q", path), err)
	}

	return FileConfig{Raw: b}, nil
}
