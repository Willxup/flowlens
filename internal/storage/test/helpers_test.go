package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Willxup/flowlens/internal/storage"
)

func openTestStore(t *testing.T) (*storage.Store, string) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	store, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store, databasePath
}

func openRawDatabase(t *testing.T, databasePath string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("raw Close() error = %v", err)
		}
	})
	return database
}

func applicationTableNames(t *testing.T, database *sql.DB) []string {
	t.Helper()
	rows, err := database.Query(`
		SELECT name
		FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
	`)
	if err != nil {
		t.Fatalf("query application tables: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan application table: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate application tables: %v", err)
	}
	sort.Strings(names)
	return names
}
