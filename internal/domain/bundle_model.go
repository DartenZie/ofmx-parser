// Package domain defines shared domain models and typed errors.
//
// Author: Miroslav Pašek
package domain

// BundleManifest describes the contents of an .ofpkg bundle archive.
type BundleManifest struct {
	SchemaVersion string           `json:"schemaVersion"`
	GeneratedAt   string           `json:"generatedAt"`
	Source        string           `json:"source"`
	Artifacts     []BundleArtifact `json:"artifacts"`
	Metadata      BundleMetadata   `json:"metadata"`
}

// BundleArtifact describes a single file entry inside the bundle.
type BundleArtifact struct {
	Role        string `json:"role"`
	Path        string `json:"path"`
	MediaType   string `json:"mediaType"`
	Compression string `json:"compression"`
	SizeBytes   int64  `json:"sizeBytes"`
	SHA256      string `json:"sha256"`
}

// BundleMetadata carries high-level snapshot metadata into the bundle manifest.
type BundleMetadata struct {
	Cycle  string `json:"cycle,omitempty"`
	Region string `json:"region,omitempty"`
}

// BundleEntry represents a single artifact that should be placed into the bundle.
type BundleEntry struct {
	// SourcePath is the filesystem path to the artifact file.
	SourcePath string
	// ArchivePath is the entry path inside the ZIP archive.
	ArchivePath string
	// Role is the manifest role identifier for this artifact.
	Role string
	// MediaType is the MIME type for the manifest entry.
	MediaType string
	// Store disables DEFLATE compression (used for pre-compressed binaries like PMTiles).
	Store bool
}

// BundleRequest contains everything needed to produce an .ofpkg bundle.
type BundleRequest struct {
	// OutputPath is the filesystem destination for the .ofpkg archive.
	OutputPath string
	// Entries lists the artifact files to include.
	Entries []BundleEntry
	// Metadata carries snapshot-level metadata for the manifest.
	Metadata BundleMetadata
}
