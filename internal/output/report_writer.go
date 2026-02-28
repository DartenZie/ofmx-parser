// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pašek
package output

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

type ReportWriter interface {
	Write(ctx context.Context, report domain.ParseReport, path string) error
}

// JSONReportWriter writes parse reports as JSON.
type JSONReportWriter struct{}

// Write serializes the parse report and writes it to disk.
func (w JSONReportWriter) Write(_ context.Context, report domain.ParseReport, path string) error {
	report.FeatureCounts = cloneSortedMap(report.FeatureCounts)

	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return domain.NewError(domain.ErrOutput, "failed to encode parse report", err)
	}

	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
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
