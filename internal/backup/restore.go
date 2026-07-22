package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Restore validates and materializes one committed backup at a new path.
func Restore(
	ctx context.Context,
	manifestPath string,
	outputPath string,
	policy ValidationPolicy,
) (Validation, error) {
	if err := ctx.Err(); err != nil {
		return Validation{}, err
	}
	if outputPath == "" || !filepath.IsAbs(outputPath) || filepath.Clean(outputPath) != outputPath {
		return Validation{}, errors.New("invalid FlowLens restore output path")
	}
	if policy.ExpectedBucketTimezone == "" || policy.MaximumSchemaVersion <= 0 {
		return Validation{}, errors.New("invalid FlowLens restore validation policy")
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return Validation{}, err
	}
	if manifest.BucketTimezone != policy.ExpectedBucketTimezone || manifest.SchemaVersion > policy.MaximumSchemaVersion {
		return Validation{}, errors.New("FlowLens backup is incompatible")
	}

	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Validation{}, errors.New("cannot create FlowLens restore output")
	}
	complete := false
	defer func() {
		if complete {
			return
		}
		_ = output.Close()
		_ = os.Remove(outputPath)
		_ = os.Remove(outputPath + "-wal")
		_ = os.Remove(outputPath + "-shm")
	}()
	if err := output.Chmod(0o600); err != nil {
		return Validation{}, errors.New("cannot secure FlowLens restore output")
	}
	dataPath := strings.TrimSuffix(manifestPath, ".manifest.json") + ".db.zst"
	written, digest, err := decompressAndHash(ctx, dataPath, output, manifest.OriginalSize)
	if err != nil || written != manifest.OriginalSize || digest != manifest.DatabaseSHA256 {
		return Validation{}, errors.New("FlowLens backup data validation failed")
	}
	if err := output.Sync(); err != nil {
		return Validation{}, errors.New("cannot sync FlowLens restore output")
	}
	if err := output.Close(); err != nil {
		return Validation{}, errors.New("cannot close FlowLens restore output")
	}
	schemaVersion, err := inspectDatabase(outputPath)
	if err != nil || schemaVersion != manifest.SchemaVersion || schemaVersion > policy.MaximumSchemaVersion {
		return Validation{}, errors.New("FlowLens backup database validation failed")
	}
	complete = true
	return Validation{Manifest: manifest}, nil
}
