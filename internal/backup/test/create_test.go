package backup_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/backup"
)

func TestCreateCommitsCompressedSnapshotThenManifest(t *testing.T) {
	options := backupOptions(t)
	createdAt := time.Date(2026, time.July, 18, 4, 0, 0, 0, time.UTC)

	artifact, err := backup.Create(context.Background(), options, createdAt)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	for _, path := range []string{artifact.DataPath, artifact.ManifestPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", filepath.Base(path), err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o", filepath.Base(path), info.Mode().Perm())
		}
	}
	validation, err := backup.Validate(context.Background(), artifact.ManifestPath, backup.ValidationPolicy{
		ExpectedBucketTimezone: options.BucketTimezone,
		MaximumSchemaVersion:   1,
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if validation.Manifest.FormatVersion != 1 || validation.Manifest.ApplicationVersion != options.ApplicationVersion ||
		validation.Manifest.SchemaVersion != 1 || validation.Manifest.CreatedAt != createdAt.Unix() ||
		validation.Manifest.OriginalSize <= 0 || len(validation.Manifest.DatabaseSHA256) != 64 ||
		validation.Manifest.BucketTimezone != options.BucketTimezone {
		t.Fatalf("validation manifest = %#v", validation.Manifest)
	}
	contents, err := os.ReadFile(artifact.ManifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	for _, forbidden := range []string{options.Directory, "flowlens.db", "fixture-clash-secret"} {
		if strings.Contains(string(contents), forbidden) {
			t.Errorf("manifest contains %q: %s", forbidden, contents)
		}
	}
	entries, err := os.ReadDir(options.Directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Errorf("temporary backup file remains: %s", entry.Name())
		}
	}
	for _, value := range []any{artifact, validation} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(format, value)
			if strings.Contains(formatted, options.Directory) {
				t.Errorf("fmt.Sprintf(%q) leaked path: %s", format, formatted)
			}
		}
	}
}

func TestHasCommittedForDayRecognizesOnlyValidatedArtifact(t *testing.T) {
	options := backupOptions(t)
	createdAt := time.Date(2026, time.July, 18, 20, 30, 0, 0, time.UTC)
	artifact, err := backup.Create(context.Background(), options, createdAt)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	committed, err := backup.HasCommittedForDay(
		context.Background(), options.Directory, createdAt.Add(30*time.Minute), options.BucketTimezone,
	)
	if err != nil {
		t.Fatalf("HasCommittedForDay() error = %v", err)
	}
	if !committed {
		t.Fatal("HasCommittedForDay() = false on the same configured natural day")
	}

	committed, err = backup.HasCommittedForDay(
		context.Background(), options.Directory, createdAt.Add(24*time.Hour), options.BucketTimezone,
	)
	if err != nil {
		t.Fatalf("next-day HasCommittedForDay() error = %v", err)
	}
	if committed {
		t.Fatal("next-day HasCommittedForDay() = true")
	}

	missingData := artifact.DataPath + ".missing"
	if err := os.Rename(artifact.DataPath, missingData); err != nil {
		t.Fatalf("hide backup data: %v", err)
	}
	committed, err = backup.HasCommittedForDay(
		context.Background(), options.Directory, createdAt, options.BucketTimezone,
	)
	if err != nil {
		t.Fatalf("missing-data HasCommittedForDay() error = %v", err)
	}
	if committed {
		t.Fatal("missing-data HasCommittedForDay() = true")
	}
}

func TestCreateDoesNotCommitManifestWhenDataDestinationExists(t *testing.T) {
	options := backupOptions(t)
	createdAt := time.Date(2026, time.July, 18, 4, 0, 0, 0, time.UTC)
	base := "flowlens-20260718T040000Z"
	dataPath := filepath.Join(options.Directory, base+".db.zst")
	manifestPath := filepath.Join(options.Directory, base+".manifest.json")
	if err := os.WriteFile(dataPath, []byte("fixture collision"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := backup.Create(context.Background(), options, createdAt); err == nil {
		t.Fatal("Create() error = nil with existing data destination")
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("manifest exists after failed Create(): %v", err)
	}
}

func TestManifestUsesOnlyDocumentedFields(t *testing.T) {
	options := backupOptions(t)
	artifact, err := backup.Create(context.Background(), options, time.Date(2026, time.July, 18, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	contents, err := os.ReadFile(artifact.ManifestPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(contents, &fields); err != nil {
		t.Fatalf("manifest JSON error = %v", err)
	}
	want := []string{
		"format_version", "application_version", "schema_version", "created_at",
		"original_size", "database_sha256", "bucket_timezone",
	}
	if len(fields) != len(want) {
		t.Fatalf("manifest fields = %#v", fields)
	}
	for _, field := range want {
		if _, exists := fields[field]; !exists {
			t.Errorf("manifest missing %q", field)
		}
	}
}
