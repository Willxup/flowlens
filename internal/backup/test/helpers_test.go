package backup_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Willxup/flowlens/internal/backup"
	"github.com/Willxup/flowlens/internal/migrations"
	"github.com/Willxup/flowlens/internal/storage"
)

func latestSchemaVersion(t *testing.T) int {
	t.Helper()
	version, err := migrations.LatestVersion()
	if err != nil {
		t.Fatalf("migrations.LatestVersion() error = %v", err)
	}
	return version
}

func backupOptions(t *testing.T) backup.Options {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	store, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	if _, err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("storage.Migrate() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("storage.Close() error = %v", err)
		}
	})
	return backup.Options{
		Store: store, Directory: t.TempDir(), DailyKeep: 10, MonthlyKeep: 10,
		BucketTimezone: "Asia/Shanghai", ApplicationVersion: "0.1.0-test",
	}
}
