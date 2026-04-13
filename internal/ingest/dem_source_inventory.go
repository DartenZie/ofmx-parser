// Package ingest reads and parses input sources.
//
// Author: Miroslav Pasek
package ingest

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

// DEMSourceIngestor loads and validates source DEM files.
type DEMSourceIngestor interface {
	Ingest(ctx context.Context, sourceDir, checksumsPath string) (domain.DEMSourceInventory, error)
}

// FileDEMSourcesIngestor reads DEM files from the filesystem.
type FileDEMSourcesIngestor struct{}

// Ingest scans DEM files, validates integrity, and returns deterministic inventory.
func (i FileDEMSourcesIngestor) Ingest(_ context.Context, sourceDir, checksumsPath string) (domain.DEMSourceInventory, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to read DEM source dir %q", sourceDir), err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		nameLower := strings.ToLower(entry.Name())
		if strings.HasSuffix(nameLower, ".tif") || strings.HasSuffix(nameLower, ".tiff") {
			files = append(files, filepath.Join(sourceDir, entry.Name()))
		}
	}

	sort.Strings(files)
	if len(files) == 0 {
		return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, "no Copernicus DEM source files (*.tif/*.tiff) found", nil)
	}

	expectedChecksums, err := parseChecksumFile(checksumsPath)
	if err != nil {
		return domain.DEMSourceInventory{}, err
	}

	inventory := domain.DEMSourceInventory{Files: make([]domain.DEMSourceFile, 0, len(files))}
	for _, absPath := range files {
		stat, err := os.Stat(absPath)
		if err != nil {
			return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to stat DEM source file %q", absPath), err)
		}

		rel := filepath.Base(absPath)
		checksum, err := sha256File(absPath)
		if err != nil {
			return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to checksum DEM source file %q", absPath), err)
		}

		if len(expectedChecksums) > 0 {
			expected, ok := expectedChecksums[rel]
			if !ok {
				return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("checksum entry missing for source file %q", rel), nil)
			}
			if !strings.EqualFold(expected, checksum) {
				return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("checksum mismatch for source file %q", rel), nil)
			}
		}

		inventory.Files = append(inventory.Files, domain.DEMSourceFile{
			Path:           absPath,
			RelativePath:   rel,
			SizeBytes:      stat.Size(),
			SHA256Checksum: checksum,
		})
	}

	if len(expectedChecksums) > len(inventory.Files) {
		for filename := range expectedChecksums {
			if !hasSourceFile(inventory.Files, filename) {
				return domain.DEMSourceInventory{}, domain.NewError(domain.ErrIngest, fmt.Sprintf("checksum references missing source file %q", filename), nil)
			}
		}
	}

	return inventory, nil
}

func hasSourceFile(files []domain.DEMSourceFile, filename string) bool {
	for _, file := range files {
		if file.RelativePath == filename {
			return true
		}
	}
	return false
}

func parseChecksumFile(path string) (map[string]string, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]string{}, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed to open checksum file %q", path), err)
	}
	defer f.Close()

	checksums := map[string]string{}
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		parts := strings.Fields(raw)
		if len(parts) < 2 {
			return nil, domain.NewError(domain.ErrIngest, fmt.Sprintf("invalid checksum line %d in %q", line, path), nil)
		}
		checksum := strings.ToLower(strings.TrimSpace(parts[0]))
		filename := strings.TrimSpace(parts[len(parts)-1])
		if checksum == "" || filename == "" {
			return nil, domain.NewError(domain.ErrIngest, fmt.Sprintf("invalid checksum line %d in %q", line, path), nil)
		}
		checksums[filename] = checksum
	}
	if err := scanner.Err(); err != nil {
		return nil, domain.NewError(domain.ErrIngest, fmt.Sprintf("failed reading checksum file %q", path), err)
	}

	return checksums, nil
}

func sha256File(path string) (string, error) {
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
