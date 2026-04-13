package output

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

func TestZIPBundleWriterCreatesValidArchive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create sample artifact files.
	xmlContent := []byte(`<?xml version="1.0" encoding="UTF-8"?><NavSnapshot/>`)
	xmlPath := filepath.Join(dir, "nav.xml")
	if err := os.WriteFile(xmlPath, xmlContent, 0o644); err != nil {
		t.Fatalf("write xml fixture: %v", err)
	}

	reportContent := []byte(`{"total_features":1}`)
	reportPath := filepath.Join(dir, "report.json")
	if err := os.WriteFile(reportPath, reportContent, 0o644); err != nil {
		t.Fatalf("write report fixture: %v", err)
	}

	bundlePath := filepath.Join(dir, "out.ofpkg")
	req := domain.BundleRequest{
		OutputPath: bundlePath,
		Entries: []domain.BundleEntry{
			{
				SourcePath:  xmlPath,
				ArchivePath: "payload/navsnapshot.xml",
				Role:        "navsnapshot",
				MediaType:   "application/xml",
				Store:       false,
			},
			{
				SourcePath:  reportPath,
				ArchivePath: "reports/report.json",
				Role:        "parse-report",
				MediaType:   "application/json",
				Store:       false,
			},
		},
		Metadata: domain.BundleMetadata{Cycle: "20260115", Region: "CZ"},
	}

	if err := (ZIPBundleWriter{}).Write(context.Background(), req); err != nil {
		t.Fatalf("bundle write failed: %v", err)
	}

	// Open and inspect the archive.
	r, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("open bundle archive: %v", err)
	}
	defer r.Close()

	// Expect: manifest.json, payload/navsnapshot.xml, reports/report.json, checksums.sha256
	expectedNames := []string{
		"manifest.json",
		"payload/navsnapshot.xml",
		"reports/report.json",
		"checksums.sha256",
	}
	if len(r.File) != len(expectedNames) {
		t.Fatalf("expected %d entries, got %d", len(expectedNames), len(r.File))
	}
	for i, f := range r.File {
		if f.Name != expectedNames[i] {
			t.Errorf("entry %d: expected %q, got %q", i, expectedNames[i], f.Name)
		}
	}
}

func TestZIPBundleWriterManifestContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	xmlContent := []byte("<NavSnapshot/>")
	xmlPath := filepath.Join(dir, "nav.xml")
	if err := os.WriteFile(xmlPath, xmlContent, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	bundlePath := filepath.Join(dir, "out.ofpkg")
	req := domain.BundleRequest{
		OutputPath: bundlePath,
		Entries: []domain.BundleEntry{{
			SourcePath:  xmlPath,
			ArchivePath: "payload/navsnapshot.xml",
			Role:        "navsnapshot",
			MediaType:   "application/xml",
			Store:       false,
		}},
		Metadata: domain.BundleMetadata{Cycle: "20260115", Region: "CZ"},
	}

	if err := (ZIPBundleWriter{}).Write(context.Background(), req); err != nil {
		t.Fatalf("bundle write failed: %v", err)
	}

	r, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer r.Close()

	// Read manifest.json.
	manifestFile := r.File[0]
	if manifestFile.Name != "manifest.json" {
		t.Fatalf("first entry should be manifest.json, got %q", manifestFile.Name)
	}

	rc, err := manifestFile.Open()
	if err != nil {
		t.Fatalf("open manifest entry: %v", err)
	}
	defer rc.Close()

	var manifest domain.BundleManifest
	if err := json.NewDecoder(rc).Decode(&manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	if manifest.SchemaVersion != "1.0.0" {
		t.Errorf("expected schemaVersion 1.0.0, got %q", manifest.SchemaVersion)
	}
	if manifest.Source != "ofmx-parser" {
		t.Errorf("expected source ofmx-parser, got %q", manifest.Source)
	}
	if manifest.Metadata.Cycle != "20260115" {
		t.Errorf("expected cycle 20260115, got %q", manifest.Metadata.Cycle)
	}
	if manifest.Metadata.Region != "CZ" {
		t.Errorf("expected region CZ, got %q", manifest.Metadata.Region)
	}
	if len(manifest.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(manifest.Artifacts))
	}

	art := manifest.Artifacts[0]
	if art.Role != "navsnapshot" {
		t.Errorf("expected role navsnapshot, got %q", art.Role)
	}
	if art.Path != "payload/navsnapshot.xml" {
		t.Errorf("expected path payload/navsnapshot.xml, got %q", art.Path)
	}
	if art.MediaType != "application/xml" {
		t.Errorf("expected mediaType application/xml, got %q", art.MediaType)
	}
	if art.Compression != "deflate" {
		t.Errorf("expected compression deflate, got %q", art.Compression)
	}
	if art.SizeBytes != int64(len(xmlContent)) {
		t.Errorf("expected sizeBytes %d, got %d", len(xmlContent), art.SizeBytes)
	}

	h := sha256.Sum256(xmlContent)
	expectedHash := hex.EncodeToString(h[:])
	if art.SHA256 != expectedHash {
		t.Errorf("expected sha256 %q, got %q", expectedHash, art.SHA256)
	}
}

func TestZIPBundleWriterStoreCompression(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blobContent := []byte("PMTILES binary blob content")
	blobPath := filepath.Join(dir, "map.pmtiles")
	if err := os.WriteFile(blobPath, blobContent, 0o644); err != nil {
		t.Fatalf("write blob fixture: %v", err)
	}

	bundlePath := filepath.Join(dir, "out.ofpkg")
	req := domain.BundleRequest{
		OutputPath: bundlePath,
		Entries: []domain.BundleEntry{{
			SourcePath:  blobPath,
			ArchivePath: "payload/map.pmtiles",
			Role:        "map-pmtiles",
			MediaType:   "application/octet-stream",
			Store:       true,
		}},
		Metadata: domain.BundleMetadata{},
	}

	if err := (ZIPBundleWriter{}).Write(context.Background(), req); err != nil {
		t.Fatalf("bundle write failed: %v", err)
	}

	r, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer r.Close()

	// payload/map.pmtiles should be the second entry (after manifest.json).
	pmtilesEntry := r.File[1]
	if pmtilesEntry.Name != "payload/map.pmtiles" {
		t.Fatalf("expected payload/map.pmtiles, got %q", pmtilesEntry.Name)
	}
	if pmtilesEntry.Method != zip.Store {
		t.Errorf("expected STORE method for pmtiles entry, got %d", pmtilesEntry.Method)
	}
}

func TestZIPBundleWriterFixedTimestamps(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "nav.xml")
	if err := os.WriteFile(xmlPath, []byte("<x/>"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	bundlePath := filepath.Join(dir, "out.ofpkg")
	req := domain.BundleRequest{
		OutputPath: bundlePath,
		Entries: []domain.BundleEntry{{
			SourcePath:  xmlPath,
			ArchivePath: "payload/navsnapshot.xml",
			Role:        "navsnapshot",
			MediaType:   "application/xml",
			Store:       false,
		}},
		Metadata: domain.BundleMetadata{},
	}

	if err := (ZIPBundleWriter{}).Write(context.Background(), req); err != nil {
		t.Fatalf("bundle write failed: %v", err)
	}

	r, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if !f.Modified.Equal(bundleFixedTime) {
			t.Errorf("entry %q has non-fixed timestamp: %v", f.Name, f.Modified)
		}
	}
}

func TestZIPBundleWriterChecksumsSHA256Content(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content1 := []byte("alpha")
	path1 := filepath.Join(dir, "a.xml")
	if err := os.WriteFile(path1, content1, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	content2 := []byte("beta")
	path2 := filepath.Join(dir, "b.json")
	if err := os.WriteFile(path2, content2, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	bundlePath := filepath.Join(dir, "out.ofpkg")
	req := domain.BundleRequest{
		OutputPath: bundlePath,
		Entries: []domain.BundleEntry{
			{SourcePath: path1, ArchivePath: "payload/navsnapshot.xml", Role: "navsnapshot", MediaType: "application/xml"},
			{SourcePath: path2, ArchivePath: "reports/report.json", Role: "parse-report", MediaType: "application/json"},
		},
		Metadata: domain.BundleMetadata{},
	}

	if err := (ZIPBundleWriter{}).Write(context.Background(), req); err != nil {
		t.Fatalf("bundle write failed: %v", err)
	}

	r, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer r.Close()

	// checksums.sha256 is the last entry.
	csFile := r.File[len(r.File)-1]
	if csFile.Name != "checksums.sha256" {
		t.Fatalf("expected last entry checksums.sha256, got %q", csFile.Name)
	}

	rc, err := csFile.Open()
	if err != nil {
		t.Fatalf("open checksums entry: %v", err)
	}
	defer rc.Close()

	buf := make([]byte, 4096)
	n, _ := rc.Read(buf)
	csContent := string(buf[:n])

	// Lines should be sorted by path.
	lines := strings.Split(strings.TrimSpace(csContent), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 checksum lines, got %d: %q", len(lines), csContent)
	}

	h1 := sha256.Sum256(content1)
	expectedLine1 := hex.EncodeToString(h1[:]) + "  payload/navsnapshot.xml"
	h2 := sha256.Sum256(content2)
	expectedLine2 := hex.EncodeToString(h2[:]) + "  reports/report.json"

	if lines[0] != expectedLine1 {
		t.Errorf("checksum line 0: expected %q, got %q", expectedLine1, lines[0])
	}
	if lines[1] != expectedLine2 {
		t.Errorf("checksum line 1: expected %q, got %q", expectedLine2, lines[1])
	}
}

func TestZIPBundleWriterRejectsEmptyEntries(t *testing.T) {
	t.Parallel()

	bundlePath := filepath.Join(t.TempDir(), "out.ofpkg")
	req := domain.BundleRequest{
		OutputPath: bundlePath,
		Entries:    nil,
		Metadata:   domain.BundleMetadata{},
	}

	err := (ZIPBundleWriter{}).Write(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty entries")
	}
}

func TestZIPBundleWriterRejectsMissingSourceFile(t *testing.T) {
	t.Parallel()

	bundlePath := filepath.Join(t.TempDir(), "out.ofpkg")
	req := domain.BundleRequest{
		OutputPath: bundlePath,
		Entries: []domain.BundleEntry{{
			SourcePath:  filepath.Join(t.TempDir(), "nonexistent.xml"),
			ArchivePath: "payload/navsnapshot.xml",
			Role:        "navsnapshot",
			MediaType:   "application/xml",
		}},
		Metadata: domain.BundleMetadata{},
	}

	err := (ZIPBundleWriter{}).Write(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestZIPBundleWriterRespectsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "nav.xml")
	if err := os.WriteFile(xmlPath, []byte("<x/>"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	bundlePath := filepath.Join(dir, "out.ofpkg")
	req := domain.BundleRequest{
		OutputPath: bundlePath,
		Entries: []domain.BundleEntry{{
			SourcePath:  xmlPath,
			ArchivePath: "payload/navsnapshot.xml",
			Role:        "navsnapshot",
			MediaType:   "application/xml",
		}},
		Metadata: domain.BundleMetadata{},
	}

	err := (ZIPBundleWriter{}).Write(ctx, req)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestZIPBundleWriterManifestArtifactStoreCompression(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blobPath := filepath.Join(dir, "map.pmtiles")
	if err := os.WriteFile(blobPath, []byte("blob"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	bundlePath := filepath.Join(dir, "out.ofpkg")
	req := domain.BundleRequest{
		OutputPath: bundlePath,
		Entries: []domain.BundleEntry{{
			SourcePath:  blobPath,
			ArchivePath: "payload/map.pmtiles",
			Role:        "map-pmtiles",
			MediaType:   "application/octet-stream",
			Store:       true,
		}},
		Metadata: domain.BundleMetadata{},
	}

	if err := (ZIPBundleWriter{}).Write(context.Background(), req); err != nil {
		t.Fatalf("bundle write failed: %v", err)
	}

	r, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer r.Close()

	// Read manifest and verify compression field is "store".
	rc, err := r.File[0].Open()
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer rc.Close()

	var manifest domain.BundleManifest
	if err := json.NewDecoder(rc).Decode(&manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	if len(manifest.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(manifest.Artifacts))
	}
	if manifest.Artifacts[0].Compression != "store" {
		t.Errorf("expected compression 'store', got %q", manifest.Artifacts[0].Compression)
	}
}
