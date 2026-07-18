package backup

import "github.com/Willxup/flowlens/internal/storage"

// Options configures one local backup run.
type Options struct {
	Store              *storage.Store
	Directory          string
	DailyKeep          int
	MonthlyKeep        int
	BucketTimezone     string
	ApplicationVersion string
}

func (Options) String() string           { return "BackupOptions{redacted}" }
func (options Options) GoString() string { return options.String() }

// Manifest is the complete committed backup metadata contract.
type Manifest struct {
	FormatVersion      int    `json:"format_version"`
	ApplicationVersion string `json:"application_version"`
	SchemaVersion      int    `json:"schema_version"`
	CreatedAt          int64  `json:"created_at"`
	OriginalSize       int64  `json:"original_size"`
	DatabaseSHA256     string `json:"database_sha256"`
	BucketTimezone     string `json:"bucket_timezone"`
}

// Artifact identifies one committed local pair. Formatting never reveals paths.
type Artifact struct {
	DataPath     string
	ManifestPath string
}

func (Artifact) String() string            { return "BackupArtifact{redacted}" }
func (artifact Artifact) GoString() string { return artifact.String() }

// ValidationPolicy constrains restore candidates to this application state.
type ValidationPolicy struct {
	ExpectedBucketTimezone string
	MaximumSchemaVersion   int
}

// Validation is one successful restore-validation result.
type Validation struct {
	Manifest Manifest
}

func (Validation) String() string              { return "BackupValidation{redacted}" }
func (validation Validation) GoString() string { return validation.String() }
