// Package pipeline orchestrates ingest, transform, validation, and output.
//
// Author: Miroslav Pašek
package pipeline

import (
	"context"
	"log"
	"time"

	"github.com/DartenZie/ofmx-parser/internal/domain"
	"github.com/DartenZie/ofmx-parser/internal/output"
)

// BundleService packages produced artifacts into a single .ofpkg archive.
type BundleService struct {
	writer output.BundleWriter
}

// NewBundleService constructs a bundle export service.
func NewBundleService(writer output.BundleWriter) BundleService {
	return BundleService{writer: writer}
}

// Execute writes the bundle archive from the collected artifact entries.
func (s BundleService) Execute(ctx context.Context, req domain.BundleRequest) (runErr error) {
	startedAt := time.Now()
	defer func() {
		duration := time.Since(startedAt).Round(time.Millisecond)
		if runErr != nil {
			log.Printf("Bundle packaging failed after %s", duration)
			return
		}
		log.Printf("Bundle packaging finished in %s", duration)
	}()

	log.Printf("Packaging %d artifact(s) into %q", len(req.Entries), req.OutputPath)

	if err := s.writer.Write(ctx, req); err != nil {
		runErr = err
		return runErr
	}

	return nil
}
