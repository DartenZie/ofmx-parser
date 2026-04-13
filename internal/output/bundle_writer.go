// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pašek
package output

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// bundleFixedTime is a deterministic timestamp used for all ZIP entries to
// guarantee byte-reproducible archives given identical inputs.
var bundleFixedTime = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

const bundleSchemaVersion = "1.0.0"

// BundleWriter packages produced artifacts into a single .ofpkg archive.
type BundleWriter interface {
	Write(ctx context.Context, req domain.BundleRequest) error
}

// ZIPBundleWriter implements BundleWriter using a standard ZIP64 archive.
type ZIPBundleWriter struct{}

func (w ZIPBundleWriter) Write(ctx context.Context, req domain.BundleRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(req.Entries) == 0 {
		return domain.NewError(domain.ErrOutput, "bundle has no artifacts to package", nil)
	}

	// Compute SHA-256 and file sizes for all entries.
	metas := make([]bundleEntryMeta, len(req.Entries))
	for i, e := range req.Entries {
		checksum, size, err := sha256FileWithSize(e.SourcePath)
		if err != nil {
			return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to hash bundle entry %q", e.SourcePath), err)
		}
		metas[i] = bundleEntryMeta{sha256: checksum, sizeBytes: size}
	}

	// Build manifest.
	artifacts := make([]domain.BundleArtifact, len(req.Entries))
	for i, e := range req.Entries {
		compression := "deflate"
		if e.Store {
			compression = "store"
		}
		artifacts[i] = domain.BundleArtifact{
			Role:        e.Role,
			Path:        e.ArchivePath,
			MediaType:   e.MediaType,
			Compression: compression,
			SizeBytes:   metas[i].sizeBytes,
			SHA256:      metas[i].sha256,
		}
	}

	manifest := domain.BundleManifest{
		SchemaVersion: bundleSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Source:        "ofmx-parser",
		Artifacts:     artifacts,
		Metadata:      req.Metadata,
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return domain.NewError(domain.ErrOutput, "failed to marshal bundle manifest", err)
	}
	manifestBytes = append(manifestBytes, '\n')

	// Build checksums.sha256 content.
	checksumLines := buildChecksumLines(req.Entries, metas)

	// Write the archive to a temp file, then rename atomically.
	dir := filepath.Dir(req.OutputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to create bundle output dir %q", dir), err)
	}

	tmp, err := os.CreateTemp(dir, ".ofmx-parser-bundle-*")
	if err != nil {
		return domain.NewError(domain.ErrOutput, "failed to create temp file for bundle", err)
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	zw := zip.NewWriter(tmp)

	// 1. manifest.json (DEFLATE, first entry).
	if err := writeZIPEntry(zw, "manifest.json", manifestBytes, zip.Deflate); err != nil {
		return domain.NewError(domain.ErrOutput, "failed to write manifest.json to bundle", err)
	}

	// 2. Payload and report entries in declaration order.
	for i, e := range req.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		method := zip.Deflate
		if e.Store {
			method = zip.Store
		}
		if err := writeZIPEntryFromFile(zw, e.ArchivePath, e.SourcePath, method); err != nil {
			return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to write %q to bundle", e.ArchivePath), err)
		}
		_ = i // consumed in metas above
	}

	// 3. checksums.sha256 (DEFLATE, last entry).
	if err := writeZIPEntry(zw, "checksums.sha256", checksumLines, zip.Deflate); err != nil {
		return domain.NewError(domain.ErrOutput, "failed to write checksums.sha256 to bundle", err)
	}

	if err := zw.Close(); err != nil {
		return domain.NewError(domain.ErrOutput, "failed to finalize bundle archive", err)
	}
	if err := tmp.Sync(); err != nil {
		return domain.NewError(domain.ErrOutput, "failed to sync bundle archive", err)
	}
	if err := tmp.Close(); err != nil {
		return domain.NewError(domain.ErrOutput, "failed to close bundle archive", err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, req.OutputPath); err != nil {
		return domain.NewError(domain.ErrOutput, fmt.Sprintf("failed to rename bundle to %q", req.OutputPath), err)
	}
	success = true
	return nil
}

// writeZIPEntry writes an in-memory blob as a ZIP entry with a fixed timestamp.
func writeZIPEntry(zw *zip.Writer, name string, data []byte, method uint16) error {
	header := &zip.FileHeader{
		Name:     name,
		Method:   method,
		Modified: bundleFixedTime,
	}
	header.SetMode(0o644)

	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// writeZIPEntryFromFile streams a filesystem file into a ZIP entry.
func writeZIPEntryFromFile(zw *zip.Writer, name, srcPath string, method uint16) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	header := &zip.FileHeader{
		Name:     name,
		Method:   method,
		Modified: bundleFixedTime,
	}
	header.SetMode(0o644)

	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

// sha256FileWithSize computes the SHA-256 checksum and byte size for a file.
func sha256FileWithSize(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// bundleEntryMeta holds precomputed hash and size for a bundle entry.
type bundleEntryMeta struct {
	sha256    string
	sizeBytes int64
}

// buildChecksumLines produces the checksums.sha256 content sorted by archive path.
func buildChecksumLines(entries []domain.BundleEntry, metas []bundleEntryMeta) []byte {
	type line struct {
		hash string
		path string
	}
	lines := make([]line, len(entries))
	for i, e := range entries {
		lines[i] = line{hash: metas[i].sha256, path: e.ArchivePath}
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].path < lines[j].path })

	var sb strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&sb, "%s  %s\n", l.hash, l.path)
	}
	return []byte(sb.String())
}
