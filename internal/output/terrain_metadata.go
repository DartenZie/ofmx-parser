// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pasek
package output

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// TerrainMetadataWriter writes terrain manifest and build reports.
type TerrainMetadataWriter interface {
	WriteManifest(ctx context.Context, path string, manifest domain.TerrainManifest) error
	WriteBuildReport(ctx context.Context, path string, report domain.TerrainBuildReport) error
}

// JSONTerrainMetadataWriter writes manifest/build report as canonical JSON.
type JSONTerrainMetadataWriter struct{}

func (w JSONTerrainMetadataWriter) WriteManifest(ctx context.Context, path string, manifest domain.TerrainManifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create manifest dir for %q", path), err)
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return domain.NewError(domain.ErrOutput, "failed to marshal terrain manifest", err)
	}
	b = append(b, '\n')
	if err := writeFileAtomic(ctx, path, b, 0o644); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to write terrain manifest %q", path), err)
	}
	return nil
}

func (w JSONTerrainMetadataWriter) WriteBuildReport(ctx context.Context, path string, report domain.TerrainBuildReport) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create build report dir for %q", path), err)
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return domain.NewError(domain.ErrOutput, "failed to marshal terrain build report", err)
	}
	b = append(b, '\n')
	if err := writeFileAtomic(ctx, path, b, 0o644); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to write terrain build report %q", path), err)
	}
	return nil
}

// BuildTerrainManifest constructs manifest metadata from artifacts and inventory.
func BuildTerrainManifest(req domain.TerrainExportRequest, inventory domain.DEMSourceInventory, pmtilesChecksum string) domain.TerrainManifest {
	checksums := make([]string, 0, len(inventory.Files))
	for _, file := range inventory.Files {
		checksums = append(checksums, fmt.Sprintf("%s:%s", file.RelativePath, file.SHA256Checksum))
	}
	sort.Strings(checksums)

	buildTime := req.BuildTimestamp.UTC().Format(time.RFC3339)
	if req.BuildTimestamp.IsZero() {
		buildTime = time.Now().UTC().Format(time.RFC3339)
	}

	return domain.TerrainManifest{
		SchemaVersion:   req.SchemaVersion,
		Version:         req.Version,
		BuildTimestamp:  buildTime,
		Bounds:          [4]float64{req.AOIBounds.MinLon, req.AOIBounds.MinLat, req.AOIBounds.MaxLon, req.AOIBounds.MaxLat},
		MinZoom:         req.MinZoom,
		MaxZoom:         req.MaxZoom,
		Encoding:        req.Encoding,
		TileSize:        req.TileSize,
		VerticalDatum:   req.VerticalDatum,
		QuantizationM:   req.ElevationQuantizationM,
		PMTilesChecksum: pmtilesChecksum,
		SourceFileCount: len(inventory.Files),
		SourceChecksums: checksums,
	}
}

// SHA256File computes SHA256 checksum for a file.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
