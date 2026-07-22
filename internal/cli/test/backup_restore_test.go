package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/backup"
	"github.com/Willxup/flowlens/internal/cli"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/migrations"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestBackupCreatesValidatedArtifact(t *testing.T) {
	databasePath := migratedDatabase(t)
	backupDirectory := t.TempDir()
	cfg := backupCommandConfig(databasePath, backupDirectory)
	createdAt := time.Date(2026, time.July, 22, 4, 0, 0, 0, time.UTC)
	dependencies := cli.Dependencies{
		LoadConfig: func() (config.Config, error) { return cfg, nil },
		Now:        func() time.Time { return createdAt },
	}

	code, stdout, stderr := runWithDependencies([]string{"backup"}, dependencies)
	if code != 0 || stdout != "backup: ok\n" || stderr != "" {
		t.Fatalf("backup = (%d, %q, %q)", code, stdout, stderr)
	}
	manifests, err := filepath.Glob(filepath.Join(backupDirectory, "*.manifest.json"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("backup manifests = %v", manifests)
	}
	latest, err := migrations.LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if _, err := backup.Validate(context.Background(), manifests[0], backup.ValidationPolicy{
		ExpectedBucketTimezone: cfg.Time.Timezone,
		MaximumSchemaVersion:   latest,
	}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestBackupRefusesRunningInstanceWithoutLeakingPaths(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	store, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("store.Migrate() error = %v", err)
	}
	backupDirectory := t.TempDir()
	cfg := backupCommandConfig(databasePath, backupDirectory)

	code, stdout, stderr := runWithDependencies([]string{"backup"}, cli.Dependencies{
		LoadConfig: func() (config.Config, error) { return cfg, nil },
		Now:        func() time.Time { return time.Date(2026, time.July, 22, 4, 0, 0, 0, time.UTC) },
	})
	if code != 1 || stdout != "" || stderr != "FlowLens backup failed\n" {
		t.Fatalf("locked backup = (%d, %q, %q)", code, stdout, stderr)
	}
	if containsAny(stderr, databasePath, backupDirectory) {
		t.Fatalf("backup error leaked a path: %q", stderr)
	}
}

func TestRestoreCheckAndOutputUseConfiguredCompatibilityPolicy(t *testing.T) {
	databasePath := migratedDatabase(t)
	backupDirectory := t.TempDir()
	cfg := backupCommandConfig(databasePath, backupDirectory)
	manifestPath := createCommandBackup(t, cfg)
	dependencies := cli.Dependencies{
		LoadConfig: func() (config.Config, error) { return cfg, nil },
	}

	code, stdout, stderr := runWithDependencies([]string{"restore", "--check", manifestPath}, dependencies)
	if code != 0 || stdout != "restore check: ok\n" || stderr != "" {
		t.Fatalf("restore --check = (%d, %q, %q)", code, stdout, stderr)
	}

	outputPath := filepath.Join(t.TempDir(), "restored.db")
	code, stdout, stderr = runWithDependencies(
		[]string{"restore", "--output", outputPath, manifestPath}, dependencies,
	)
	if code != 0 || stdout != "restore: ok\n" || stderr != "" {
		t.Fatalf("restore --output = (%d, %q, %q)", code, stdout, stderr)
	}
	if info, err := os.Stat(outputPath); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("restored output = (%v, %v)", info, err)
	}
}

func TestRestoreNeverTargetsConfiguredLiveDatabase(t *testing.T) {
	backupDatabase := migratedDatabase(t)
	backupDirectory := t.TempDir()
	cfg := backupCommandConfig(backupDatabase, backupDirectory)
	manifestPath := createCommandBackup(t, cfg)
	liveDatabase := filepath.Join(t.TempDir(), "flowlens.db")
	cfg.Storage.DatabasePath = liveDatabase

	code, stdout, stderr := runWithDependencies(
		[]string{"restore", "--output", liveDatabase, manifestPath},
		cli.Dependencies{LoadConfig: func() (config.Config, error) { return cfg, nil }},
	)
	if code != 1 || stdout != "" || stderr != "FlowLens restore failed\n" {
		t.Fatalf("restore to live database = (%d, %q, %q)", code, stdout, stderr)
	}
	if _, err := os.Stat(liveDatabase); !os.IsNotExist(err) {
		t.Fatalf("live database was created by restore: %v", err)
	}
}

func TestBackupAndRestoreRejectInvalidArguments(t *testing.T) {
	tests := [][]string{
		{},
		{"backup", "extra"},
		{"restore"},
		{"restore", "manifest"},
		{"restore", "--check"},
		{"restore", "--check", "manifest", "extra"},
		{"restore", "manifest", "--check"},
		{"restore", "--output", "manifest"},
		{"restore", "--output", "manifest", "output", "extra"},
		{"restore", "manifest", "--output", "output"},
	}
	const want = "usage: flowlens <serve|version|healthcheck|doctor|backup|restore>\n"
	for _, args := range tests {
		code, stdout, stderr := run(args)
		if code != 2 || stdout != "" || stderr != want {
			t.Errorf("Run(%q) = (%d, %q, %q)", args, code, stdout, stderr)
		}
	}
}

func backupCommandConfig(databasePath, backupDirectory string) config.Config {
	return config.Config{
		Storage: config.Storage{DatabasePath: databasePath, SoftLimit: config.ByteSize(1 << 28)},
		Time:    config.Time{Timezone: "UTC"},
		Backup: config.Backup{
			Directory: backupDirectory, DailyKeep: 3, MonthlyKeep: 3,
		},
	}
}

func createCommandBackup(t *testing.T, cfg config.Config) string {
	t.Helper()
	store, err := storage.Open(context.Background(), storage.Options{
		DatabasePath: cfg.Storage.DatabasePath, SoftLimitBytes: int64(cfg.Storage.SoftLimit),
	})
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	artifact, createErr := backup.Create(context.Background(), backup.Options{
		Store: store, Directory: cfg.Backup.Directory,
		DailyKeep: cfg.Backup.DailyKeep, MonthlyKeep: cfg.Backup.MonthlyKeep,
		BucketTimezone: cfg.Time.Timezone, ApplicationVersion: "0.1.0-test",
	}, time.Date(2026, time.July, 22, 4, 0, 0, 0, time.UTC))
	closeErr := store.Close()
	if createErr != nil {
		t.Fatalf("backup.Create() error = %v", createErr)
	}
	if closeErr != nil {
		t.Fatalf("store.Close() error = %v", closeErr)
	}
	return artifact.ManifestPath
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
