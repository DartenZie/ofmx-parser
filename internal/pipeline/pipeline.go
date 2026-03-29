// Package pipeline orchestrates ingest, transform, validation, and output.
//
// Author: Miroslav Pašek
package pipeline

import (
	"context"
	"log"
	"time"

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

	return s.ExecuteDocument(ctx, in, outputPath)
}

// ExecuteDocument runs map-validate-write for an already ingested OFMX document.
func (s Service) ExecuteDocument(ctx context.Context, in domain.OFMXDocument, outputPath string) (result Result, runErr error) {
	startedAt := time.Now()
	defer func() {
		duration := time.Since(startedAt).Round(time.Millisecond)
		if runErr != nil {
			log.Printf("XML pipeline failed after %s", duration)
			return
		}
		log.Printf("XML pipeline finished in %s", duration)
	}()

	mapStartedAt := time.Now()
	log.Printf("Mapping OFMX data to XML output model")

	out, err := s.mapper.Map(ctx, in)
	if err != nil {
		runErr = domain.NewError(domain.ErrTransform, "failed to map OFMX to output model", err)
		return Result{}, runErr
	}
	log.Printf("Mapped OFMX data to XML output model in %s", time.Since(mapStartedAt).Round(time.Millisecond))

	validateStartedAt := time.Now()
	log.Printf("Validating XML output model")

	if err := s.validator.Validate(ctx, out); err != nil {
		runErr = domain.NewError(domain.ErrValidate, "output XML model validation failed", err)
		return Result{}, runErr
	}
	log.Printf("Validated XML output model in %s", time.Since(validateStartedAt).Round(time.Millisecond))

	writeStartedAt := time.Now()
	log.Printf("Serializing XML output")

	if err := s.writer.Write(ctx, out, outputPath); err != nil {
		runErr = err
		return Result{}, runErr
	}
	log.Printf("Serialized XML output in %s", time.Since(writeStartedAt).Round(time.Millisecond))

	result = Result{Report: domain.ParseReport{
		SnapshotMeta:  in.SnapshotMeta,
		TotalFeatures: sumFeatures(in.FeatureCounts),
		FeatureCounts: in.FeatureCounts,
	}}

	return result, nil
}

func sumFeatures(counts map[string]int) int {
	total := 0
	for _, v := range counts {
		total += v
	}
	return total
}
