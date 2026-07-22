package backup_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/backup"
)

func TestRestoreExtractsValidatedDatabase(t *testing.T) {
	options := backupOptions(t)
	artifact, err := backup.Create(
		context.Background(),
		options,
		time.Date(2026, time.July, 22, 4, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	outputPath := filepath.Join(t.TempDir(), "restored.db")

	validation, err := backup.Restore(context.Background(), artifact.ManifestPath, outputPath, backup.ValidationPolicy{
		ExpectedBucketTimezone: options.BucketTimezone,
		MaximumSchemaVersion:   latestSchemaVersion(t),
	})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(output) error = %v", err)
	}
	digest := sha256.Sum256(contents)
	if got := hex.EncodeToString(digest[:]); got != validation.Manifest.DatabaseSHA256 {
		t.Fatalf("restored digest = %q, want %q", got, validation.Manifest.DatabaseSHA256)
	}
	if int64(len(contents)) != validation.Manifest.OriginalSize {
		t.Fatalf("restored size = %d, want %d", len(contents), validation.Manifest.OriginalSize)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Stat(output) error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("restored mode = %o, want 600", info.Mode().Perm())
	}
	if _, err := backup.Validate(context.Background(), artifact.ManifestPath, backup.ValidationPolicy{
		ExpectedBucketTimezone: options.BucketTimezone,
		MaximumSchemaVersion:   latestSchemaVersion(t),
	}); err != nil {
		t.Fatalf("Validate() after Restore() error = %v", err)
	}
}

func TestRestoreRejectsExistingOutputWithoutChangingIt(t *testing.T) {
	options := backupOptions(t)
	artifact, err := backup.Create(
		context.Background(),
		options,
		time.Date(2026, time.July, 22, 4, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	outputPath := filepath.Join(t.TempDir(), "restored.db")
	const original = "keep me"
	if err := os.WriteFile(outputPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile(output) error = %v", err)
	}

	if _, err := backup.Restore(context.Background(), artifact.ManifestPath, outputPath, backup.ValidationPolicy{
		ExpectedBucketTimezone: options.BucketTimezone,
		MaximumSchemaVersion:   latestSchemaVersion(t),
	}); err == nil {
		t.Fatal("Restore() error = nil with existing output")
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(output) error = %v", err)
	}
	if string(contents) != original {
		t.Fatalf("existing output = %q, want %q", contents, original)
	}
}

func TestRestoreRejectsIncompatibleBackupBeforeCreatingOutput(t *testing.T) {
	tests := []struct {
		name   string
		policy backup.ValidationPolicy
	}{
		{
			name: "timezone mismatch",
			policy: backup.ValidationPolicy{
				ExpectedBucketTimezone: "UTC",
				MaximumSchemaVersion:   latestSchemaVersion(t),
			},
		},
		{
			name: "schema too new",
			policy: backup.ValidationPolicy{
				ExpectedBucketTimezone: "Asia/Shanghai",
				MaximumSchemaVersion:   1,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := backupOptions(t)
			artifact, err := backup.Create(
				context.Background(),
				options,
				time.Date(2026, time.July, 22, 4, 0, 0, 0, time.UTC),
			)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			outputPath := filepath.Join(t.TempDir(), "restored.db")
			if _, err := backup.Restore(context.Background(), artifact.ManifestPath, outputPath, test.policy); err == nil {
				t.Fatal("Restore() error = nil")
			}
			if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
				t.Fatalf("output exists after rejected restore: %v", err)
			}
		})
	}
}

func TestRestoreRejectsCorruptionAndRemovesPartialOutput(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, artifact backup.Artifact)
	}{
		{
			name: "compressed data",
			mutate: func(t *testing.T, artifact backup.Artifact) {
				t.Helper()
				if err := os.WriteFile(artifact.DataPath, []byte("not zstd"), 0o600); err != nil {
					t.Fatalf("corrupt data: %v", err)
				}
			},
		},
		{
			name: "manifest",
			mutate: func(t *testing.T, artifact backup.Artifact) {
				t.Helper()
				contents, err := os.ReadFile(artifact.ManifestPath)
				if err != nil {
					t.Fatalf("read manifest: %v", err)
				}
				var fields map[string]any
				if err := json.Unmarshal(contents, &fields); err != nil {
					t.Fatalf("decode manifest: %v", err)
				}
				fields["database_sha256"] = "invalid"
				contents, err = json.Marshal(fields)
				if err != nil {
					t.Fatalf("encode manifest: %v", err)
				}
				if err := os.WriteFile(artifact.ManifestPath, contents, 0o600); err != nil {
					t.Fatalf("corrupt manifest: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := backupOptions(t)
			artifact, err := backup.Create(
				context.Background(),
				options,
				time.Date(2026, time.July, 22, 4, 0, 0, 0, time.UTC),
			)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			test.mutate(t, artifact)
			outputPath := filepath.Join(t.TempDir(), "restored.db")
			if _, err := backup.Restore(context.Background(), artifact.ManifestPath, outputPath, backup.ValidationPolicy{
				ExpectedBucketTimezone: options.BucketTimezone,
				MaximumSchemaVersion:   latestSchemaVersion(t),
			}); err == nil {
				t.Fatal("Restore() error = nil")
			}
			for _, path := range []string{outputPath, outputPath + "-wal", outputPath + "-shm"} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("partial output remains at %q: %v", filepath.Base(path), err)
				}
			}
		})
	}
}
