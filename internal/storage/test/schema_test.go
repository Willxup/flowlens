package storage_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Willxup/flowlens/internal/migrations"
	"github.com/Willxup/flowlens/internal/storage"
)

var expectedApplicationTables = []string{
	"collector_state",
	"flow_dimension",
	"flow_rollup",
	"maintenance_run",
	"quality_event",
	"runtime_session",
	"schema_migration",
	"service_label",
	"traffic_rollup",
}

func TestInspectSchemaAndMigrateEmptyDatabase(t *testing.T) {
	store, databasePath := openTestStore(t)
	raw := openRawDatabase(t, databasePath)

	status, err := store.InspectSchema(context.Background())
	if err != nil {
		t.Fatalf("InspectSchema() error = %v", err)
	}
	wantBefore := storage.SchemaStatus{CurrentVersion: 0, LatestVersion: 1, NeedsMigration: true}
	if status != wantBefore {
		t.Errorf("InspectSchema() = %#v, want %#v", status, wantBefore)
	}
	if tables := applicationTableNames(t, raw); len(tables) != 0 {
		t.Fatalf("Open/InspectSchema created tables: %#v", tables)
	}

	status, err = store.Migrate(context.Background())
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	wantAfter := storage.SchemaStatus{CurrentVersion: 1, LatestVersion: 1, NeedsMigration: false}
	if status != wantAfter {
		t.Errorf("Migrate() = %#v, want %#v", status, wantAfter)
	}
	if tables := applicationTableNames(t, raw); !reflect.DeepEqual(tables, expectedApplicationTables) {
		t.Errorf("application tables = %#v, want %#v", tables, expectedApplicationTables)
	}
	if err := store.QuickCheck(context.Background()); err != nil {
		t.Errorf("QuickCheck() error = %v", err)
	}

	second, err := store.Migrate(context.Background())
	if err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	if second != wantAfter {
		t.Errorf("second Migrate() = %#v", second)
	}

	migrationList, err := migrations.List()
	if err != nil {
		t.Fatalf("migrations.List() error = %v", err)
	}
	var appliedAt int64
	var checksum string
	if err := raw.QueryRow(`SELECT applied_at, checksum FROM schema_migration WHERE version = 1`).Scan(&appliedAt, &checksum); err != nil {
		t.Fatalf("read migration record: %v", err)
	}
	if appliedAt <= 0 || checksum != migrationList[0].Checksum {
		t.Errorf("migration record = applied_at:%d checksum:%q", appliedAt, checksum)
	}
}

func TestInspectSchemaRejectsUnversionedNonemptyDatabase(t *testing.T) {
	store, databasePath := openTestStore(t)
	raw := openRawDatabase(t, databasePath)
	if _, err := raw.Exec(`CREATE TABLE fixture_unversioned (id INTEGER PRIMARY KEY) STRICT`); err != nil {
		t.Fatalf("create fixture table: %v", err)
	}

	_, err := store.InspectSchema(context.Background())
	if !errors.Is(err, storage.ErrUnversionedDatabase) {
		t.Errorf("InspectSchema() error = %v, want ErrUnversionedDatabase", err)
	}
}

func TestInspectSchemaRejectsTamperedChecksum(t *testing.T) {
	store, databasePath := openTestStore(t)
	if _, err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	raw := openRawDatabase(t, databasePath)
	if _, err := raw.Exec(`UPDATE schema_migration SET checksum = ? WHERE version = 1`, strings.Repeat("0", 64)); err != nil {
		t.Fatalf("tamper checksum: %v", err)
	}

	_, err := store.InspectSchema(context.Background())
	if !errors.Is(err, storage.ErrSchemaChecksum) {
		t.Errorf("InspectSchema() error = %v, want ErrSchemaChecksum", err)
	}
}

func TestInspectSchemaRejectsMissingApplicationTable(t *testing.T) {
	store, databasePath := openTestStore(t)
	if _, err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	raw := openRawDatabase(t, databasePath)
	if _, err := raw.Exec(`DROP TABLE traffic_rollup`); err != nil {
		t.Fatalf("drop application table: %v", err)
	}

	if _, err := store.InspectSchema(context.Background()); err == nil {
		t.Fatal("InspectSchema() error = nil")
	}
}

func TestInspectSchemaRejectsNonpositiveRecordedVersion(t *testing.T) {
	store, databasePath := openTestStore(t)
	raw := openRawDatabase(t, databasePath)
	if _, err := raw.Exec(`
		CREATE TABLE schema_migration (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL,
			checksum TEXT NOT NULL
		) STRICT;
		INSERT INTO schema_migration(version, applied_at, checksum) VALUES (0, 1, ?)
	`, strings.Repeat("0", 64)); err != nil {
		t.Fatalf("create invalid migration history: %v", err)
	}

	if _, err := store.InspectSchema(context.Background()); err == nil {
		t.Fatal("InspectSchema() error = nil")
	}
}

func TestInspectSchemaRejectsNewerDatabaseVersion(t *testing.T) {
	store, databasePath := openTestStore(t)
	if _, err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	raw := openRawDatabase(t, databasePath)
	if _, err := raw.Exec(`INSERT INTO schema_migration(version, applied_at, checksum) VALUES (2, 1, ?)`, strings.Repeat("0", 64)); err != nil {
		t.Fatalf("insert newer migration: %v", err)
	}

	_, err := store.InspectSchema(context.Background())
	if !errors.Is(err, storage.ErrSchemaTooNew) {
		t.Errorf("InspectSchema() error = %v, want ErrSchemaTooNew", err)
	}
}
