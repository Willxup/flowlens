package storage_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/storage"
)

func TestOpenRejectsInvalidDatabasePathsWithoutCreatingParents(t *testing.T) {
	missingParent := filepath.Join(t.TempDir(), "missing")
	tests := map[string]string{
		"empty":          "",
		"relative":       "relative.db",
		"missing parent": filepath.Join(missingParent, "flowlens.db"),
	}
	for name, databasePath := range tests {
		t.Run(name, func(t *testing.T) {
			store, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
			if err == nil {
				t.Fatal("Open() error = nil")
			}
			if store != nil {
				t.Errorf("Open() store = %#v", store)
			}
			if strings.Contains(err.Error(), databasePath) && databasePath != "" {
				t.Errorf("Open() error contains database path: %v", err)
			}
		})
	}
	if _, err := os.Stat(missingParent); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing parent was created or has unexpected error: %v", err)
	}
}

func TestOptionsAndStoreFormattingRedactDatabasePath(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "fixture-private-database.db")
	options := storage.Options{DatabasePath: databasePath}
	store, err := storage.Open(context.Background(), options)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	for name, value := range map[string]any{"options": options, "store": store} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(format, value)
			for _, forbidden := range []string{directory, "fixture-private-database.db"} {
				if strings.Contains(formatted, forbidden) {
					t.Errorf("%s fmt.Sprintf(%q) contains %q: %s", name, format, forbidden, formatted)
				}
			}
		}
	}
}

func TestOpenEnforcesExclusiveLockAndReleasesItOnClose(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	first, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}

	started := time.Now()
	second, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if !errors.Is(err, storage.ErrLocked) {
		t.Fatalf("second Open() error = %v, want ErrLocked", err)
	}
	if second != nil {
		t.Errorf("second Open() store = %#v", second)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Errorf("second Open() elapsed = %v", elapsed)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	reopened, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("reopened Close() error = %v", err)
	}
}

func TestOpenAppliesRequiredPragmasAndKeepsFilesInDataDirectory(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "flowlens.db")
	store, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	diagnostics, err := store.Diagnostics(context.Background())
	if err != nil {
		t.Fatalf("Diagnostics() error = %v", err)
	}
	if diagnostics.JournalMode != "wal" ||
		diagnostics.Synchronous != 1 ||
		diagnostics.ForeignKeys != 1 ||
		diagnostics.BusyTimeoutMS != 5000 ||
		diagnostics.AutoVacuum != 2 ||
		diagnostics.TempStore != 2 {
		t.Errorf("Diagnostics() = %#v", diagnostics)
	}
	if err := store.QuickCheck(context.Background()); err != nil {
		t.Fatalf("QuickCheck() error = %v", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	allowed := map[string]bool{
		".flowlens.lock":      true,
		"flowlens.db":         true,
		"flowlens.db-journal": true,
		"flowlens.db-shm":     true,
		"flowlens.db-wal":     true,
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			t.Errorf("unexpected data-directory entry %q", entry.Name())
		}
	}
}
