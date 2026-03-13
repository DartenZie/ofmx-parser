// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pašek
package output

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

type ReportWriter interface {
	Write(ctx context.Context, report domain.ParseReport, path string) error
}

// JSONReportWriter writes parse reports as JSON.
type JSONReportWriter struct{}

// Write serializes the parse report and writes it to disk.
func (w JSONReportWriter) Write(ctx context.Context, report domain.ParseReport, path string) error {
	if err := ctx.Err(); err != nil {
		return domain.NewError(domain.ErrOutput, "parse report write cancelled", err)
	}

	report.FeatureCounts = cloneSortedMap(report.FeatureCounts)

	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return domain.NewError(domain.ErrOutput, "failed to encode parse report", err)
	}

	if err := writeFileAtomic(ctx, path, append(b, '\n'), 0o644); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to write parse report %q", path), err)
	}

	return nil
}

func cloneSortedMap(m map[string]int) map[string]int {
	if len(m) == 0 {
		return map[string]int{}
	}

	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make(map[string]int, len(m))
	for _, key := range keys {
		out[key] = m[key]
	}

	return out
}
