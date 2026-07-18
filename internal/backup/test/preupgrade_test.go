package backup_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/backup"
)

func TestCreateAndRemovePreUpgradeSnapshot(t *testing.T) {
	options := backupOptions(t)
	path, err := backup.CreatePreUpgrade(
		context.Background(), options.Store, options.Directory,
		time.Date(2026, time.July, 18, 3, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("CreatePreUpgrade() error = %v", err)
	}
	if !strings.Contains(path, "pre-upgrade") {
		t.Fatalf("pre-upgrade path = %q", path)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("pre-upgrade Stat() = %#v, %v", info, err)
	}
	if err := backup.RemovePreUpgrade(path); err != nil {
		t.Fatalf("RemovePreUpgrade() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("pre-upgrade snapshot still exists: %v", err)
	}
}

func TestCreatePreUpgradeRejectsExistingDestination(t *testing.T) {
	options := backupOptions(t)
	at := time.Date(2026, time.July, 18, 3, 0, 0, 0, time.UTC)
	if _, err := backup.CreatePreUpgrade(context.Background(), options.Store, options.Directory, at); err != nil {
		t.Fatalf("first CreatePreUpgrade() error = %v", err)
	}
	if _, err := backup.CreatePreUpgrade(context.Background(), options.Store, options.Directory, at); err == nil {
		t.Fatal("second CreatePreUpgrade() error = nil")
	}
}
