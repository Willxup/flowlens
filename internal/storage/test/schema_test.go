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
	wantBefore := storage.SchemaStatus{CurrentVersion: 0, LatestVersion: 2, NeedsMigration: true}
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
	wantAfter := storage.SchemaStatus{CurrentVersion: 2, LatestVersion: 2, NeedsMigration: false}
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

func TestStage3BackfillMigratesStage2History(t *testing.T) {
	store, databasePath := openTestStore(t)
	raw := openRawDatabase(t, databasePath)
	migrationList, err := migrations.List()
	if err != nil {
		t.Fatalf("migrations.List() error = %v", err)
	}
	if len(migrationList) < 1 {
		t.Fatal("migration 001 missing")
	}
	if _, err := raw.Exec(migrationList[0].SQL); err != nil {
		t.Fatalf("apply migration 001: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO schema_migration(version, applied_at, checksum) VALUES (1, 1, ?)`,
		migrationList[0].Checksum,
	); err != nil {
		t.Fatalf("record migration 001: %v", err)
	}
	insertTraffic := `INSERT INTO traffic_rollup (
		resolution_sec, bucket_start, bucket_end, upload_bytes, download_bytes,
		recovered_upload_bytes, recovered_download_bytes,
		speed_upload_sample_sum, speed_download_sample_sum, speed_sample_count,
		peak_upload_bytes_per_second, peak_download_bytes_per_second,
		peak_upload_at, peak_download_at, counter_observed_seconds,
		attribution_observed_seconds, active_connections_sum,
		active_connections_samples, active_connections_max,
		memory_bytes_sum, memory_samples, memory_bytes_max,
		unattributed_upload_bytes, unattributed_download_bytes,
		reset_count, quality_flags
	) VALUES (?, ?, ?, ?, ?, 0, 0, 0, 0, 0, 0, 0, NULL, NULL, ?, 0, 0, 0, 0, 0, 0, 0, ?, ?, 0, 8)`
	for _, row := range []struct {
		resolution, start, end, upload, download, observed int64
	}{
		{60, 120, 180, 30, 70, 6},
		{10, 200, 210, 5, 9, 1},
		{3600, 3600, 7200, 0, 0, 0},
	} {
		if _, err := raw.Exec(insertTraffic, row.resolution, row.start, row.end, row.upload, row.download,
			row.observed, row.upload, row.download); err != nil {
			t.Fatalf("insert traffic fixture: %v", err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO flow_dimension (
			source_family, source_network, source_prefix_len,
			destination_family, destination_ip, destination_port,
			host, network_code, classification_code
		) VALUES (0, X'', 0, 0, X'', -1, '', 0, 3);
		INSERT INTO flow_rollup (
			resolution_sec, bucket_start, dimension_id,
			upload_bytes, download_bytes, flow_observation_count
		) SELECT 10, 200, id, 5, 9, 1 FROM flow_dimension WHERE classification_code = 3;
	`); err != nil {
		t.Fatalf("insert existing flow fixture: %v", err)
	}

	status, err := store.Migrate(context.Background())
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if status.CurrentVersion != 2 || status.NeedsMigration {
		t.Fatalf("Migrate() status = %#v", status)
	}
	rows, err := raw.Query(`
		SELECT f.resolution_sec, f.bucket_start, f.upload_bytes, f.download_bytes, f.flow_observation_count
		FROM flow_rollup AS f
		JOIN flow_dimension AS d ON d.id = f.dimension_id
		WHERE d.classification_code = 3
		ORDER BY f.resolution_sec, f.bucket_start
	`)
	if err != nil {
		t.Fatalf("query backfill: %v", err)
	}
	defer rows.Close()
	want := [][5]int64{{10, 200, 5, 9, 1}, {60, 120, 30, 70, 6}}
	var got [][5]int64
	for rows.Next() {
		var row [5]int64
		if err := rows.Scan(&row[0], &row[1], &row[2], &row[3], &row[4]); err != nil {
			t.Fatalf("scan backfill: %v", err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate backfill: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("backfilled rows = %#v, want %#v", got, want)
	}
	var versions int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM schema_migration WHERE version IN (1, 2)`).Scan(&versions); err != nil {
		t.Fatalf("count migration records: %v", err)
	}
	if versions != 2 {
		t.Errorf("migration record count = %d", versions)
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
	latest, err := migrations.LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO schema_migration(version, applied_at, checksum) VALUES (?, 1, ?)`, latest+1, strings.Repeat("0", 64)); err != nil {
		t.Fatalf("insert newer migration: %v", err)
	}

	_, err = store.InspectSchema(context.Background())
	if !errors.Is(err, storage.ErrSchemaTooNew) {
		t.Errorf("InspectSchema() error = %v, want ErrSchemaTooNew", err)
	}
}
