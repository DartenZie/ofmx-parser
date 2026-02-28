// Package pipeline orchestrates ingest, transform, validation, and output.
//
// Author: Miroslav Pašek
package pipeline

import (
	"context"

	"github.com/DartenZie/ofmx-parser/internal/domain"
	"github.com/DartenZie/ofmx-parser/internal/ingest"
	"github.com/DartenZie/ofmx-parser/internal/output"
	"github.com/DartenZie/ofmx-parser/internal/transform"
)

type Service struct {
	reader    ingest.OFMXReader
	mapper    transform.Mapper
	validator output.SchemaValidator
	writer    output.XMLWriter
}

// Result contains pipeline execution artifacts.
type Result struct {
	Report domain.ParseReport
}

// New constructs a pipeline service with default semantic validation.
func New(reader ingest.OFMXReader, mapper transform.Mapper, writer output.XMLWriter) Service {
	return Service{
		reader:    reader,
		mapper:    mapper,
		validator: output.SemanticSchemaValidator{},
		writer:    writer,
	}
}

// Execute runs the full parse-map-validate-write workflow.
func (s Service) Execute(ctx context.Context, inputPath, outputPath string) (Result, error) {
	in, err := s.reader.Read(ctx, inputPath)
	if err != nil {
		return Result{}, err
	}

	out, err := s.mapper.Map(ctx, in)
	if err != nil {
		return Result{}, domain.NewError(domain.ErrTransform, "failed to map OFMX to output model", err)
	}

	if err := s.validator.Validate(ctx, out); err != nil {
		return Result{}, domain.NewError(domain.ErrValidate, "output XML model validation failed", err)
	}

	if err := s.writer.Write(ctx, out, outputPath); err != nil {
		return Result{}, err
	}

	return Result{Report: domain.ParseReport{
		SnapshotMeta:  in.SnapshotMeta,
		TotalFeatures: sumFeatures(in.FeatureCounts),
		FeatureCounts: in.FeatureCounts,
	}}, nil
}

func sumFeatures(counts map[string]int) int {
	total := 0
	for _, v := range counts {
		total += v
	}
	return total
}
