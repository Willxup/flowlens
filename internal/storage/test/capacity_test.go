package storage_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Willxup/flowlens/internal/storage"
)

func TestCapacityProtectionPreservesExistingDimensionsAndRedirectsNewOnes(t *testing.T) {
	store, databasePath, softLimit := capacityTestStore(t)
	first := firstBatch()
	commitBatch(t, store, first)
	status, err := store.CapacityStatus(context.Background())
	if err != nil {
		t.Fatalf("initial CapacityStatus() error = %v", err)
	}
	if status.Protecting {
		t.Fatal("CapacityStatus().Protecting = true before fixture growth")
	}

	database := openRawDatabase(t, databasePath)
	growCapacityFixture(t, database, store, softLimit)
	second := nextBatch()
	existing := second.Flows[0]
	existing.UploadBytes = 30
	existing.DownloadBytes = 120
	unseen := existing
	unseen.Dimension.Host = "new-capacity.example.test"
	other := second.Flows[1]
	unattributed := second.Flows[2]
	second.Flows = []storage.FlowRollup{existing, unseen, other, unattributed}
	commitBatch(t, store, second)

	status, err = store.CapacityStatus(context.Background())
	if err != nil || !status.Protecting {
		t.Fatalf("protected CapacityStatus() = %#v, %v", status, err)
	}
	flows, err := store.FlowRollups(context.Background(), 10, second.Global.BucketStart)
	if err != nil {
		t.Fatalf("protected FlowRollups() error = %v", err)
	}
	assertFlowClasses(t, flows, map[int64]storage.ByteTotals{
		1: {Upload: 30, Download: 120},
		2: {Upload: 40, Download: 160},
		3: {Upload: 30, Download: 120},
	})
	var unseenCount int64
	if err := database.QueryRow(`SELECT COUNT(*) FROM flow_dimension WHERE host = ?`, unseen.Dimension.Host).Scan(&unseenCount); err != nil {
		t.Fatalf("count unseen dimension: %v", err)
	}
	if unseenCount != 0 {
		t.Fatalf("unseen dimension count = %d", unseenCount)
	}

	shrinkCapacityFixture(t, database, store)
	status, err = store.CapacityStatus(context.Background())
	if err != nil {
		t.Fatalf("shrunk CapacityStatus() error = %v", err)
	}
	if total := status.DatabaseBytes + status.WALBytes; total >= softLimit-softLimit/5 {
		t.Fatalf("shrunk database bytes = %d, recovery threshold = %d", total, softLimit-softLimit/5)
	}
	third := batchAfter(second)
	third.Flows[0].Dimension.Host = "after-recovery.example.test"
	commitBatch(t, store, third)
	status, err = store.CapacityStatus(context.Background())
	if err != nil || status.Protecting {
		t.Fatalf("recovered CapacityStatus() = %#v, %v", status, err)
	}
	flows, err = store.FlowRollups(context.Background(), 10, third.Global.BucketStart)
	if err != nil || len(flows) != 3 {
		t.Fatalf("recovered FlowRollups() = %#v, %v", flows, err)
	}
	var recoveredDimensionCount int64
	if err := database.QueryRow(`SELECT COUNT(*) FROM flow_dimension WHERE host = ?`, third.Flows[0].Dimension.Host).Scan(&recoveredDimensionCount); err != nil {
		t.Fatalf("count recovered dimension: %v", err)
	}
	if recoveredDimensionCount != 1 {
		t.Fatalf("recovered dimension count = %d", recoveredDimensionCount)
	}
	if count, err := store.QualityEventCount(context.Background(), third.BatchID); err != nil || count != 1 {
		t.Fatalf("recovery QualityEventCount() = %d, %v", count, err)
	}
}

func TestCapacityOptionsAndStatusValidation(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	if _, err := storage.Open(context.Background(), storage.Options{
		DatabasePath:   databasePath,
		SoftLimitBytes: -1,
	}); err == nil {
		t.Fatal("Open() accepted negative soft limit")
	}
}

func capacityTestStore(t *testing.T) (*storage.Store, string, int64) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	initial, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		t.Fatalf("initial Open() error = %v", err)
	}
	if _, err := initial.Migrate(context.Background()); err != nil {
		t.Fatalf("initial Migrate() error = %v", err)
	}
	if err := initial.Close(); err != nil {
		t.Fatalf("initial Close() error = %v", err)
	}
	info, err := os.Stat(databasePath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	softLimit := info.Size() * 8
	store, err := storage.Open(context.Background(), storage.Options{
		DatabasePath:   databasePath,
		SoftLimitBytes: softLimit,
	})
	if err != nil {
		t.Fatalf("capacity Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("capacity Close() error = %v", err)
		}
	})
	return store, databasePath, softLimit
}

func growCapacityFixture(t *testing.T, database *sql.DB, store *storage.Store, softLimit int64) {
	t.Helper()
	detail := strings.Repeat("x", 4096)
	for iteration := 0; iteration < 64; iteration++ {
		status, err := store.CapacityStatus(context.Background())
		if err != nil {
			t.Fatalf("growth CapacityStatus() error = %v", err)
		}
		if status.DatabaseBytes+status.WALBytes >= softLimit {
			return
		}
		transaction, err := database.Begin()
		if err != nil {
			t.Fatalf("growth Begin() error = %v", err)
		}
		for index := 0; index < 128; index++ {
			if _, err := transaction.Exec(`
				INSERT INTO maintenance_run(
					operation, started_at, ended_at, deleted_rows,
					database_bytes, wal_bytes, error
				) VALUES ('capacity_fixture', 1, NULL, 0, 0, 0, ?)
			`, detail); err != nil {
				_ = transaction.Rollback()
				t.Fatalf("growth insert error = %v", err)
			}
		}
		if err := transaction.Commit(); err != nil {
			t.Fatalf("growth Commit() error = %v", err)
		}
	}
	t.Fatal("capacity fixture did not reach soft limit")
}

func shrinkCapacityFixture(t *testing.T, database *sql.DB, store *storage.Store) {
	t.Helper()
	if _, err := database.Exec(`DELETE FROM maintenance_run WHERE operation = 'capacity_fixture'`); err != nil {
		t.Fatalf("delete capacity fixture: %v", err)
	}
	if err := store.Checkpoint(context.Background(), true); err != nil {
		t.Fatalf("fixture Checkpoint() error = %v", err)
	}
	if _, err := database.Exec(`VACUUM`); err != nil {
		t.Fatalf("fixture VACUUM error = %v", err)
	}
	if err := store.Checkpoint(context.Background(), true); err != nil {
		t.Fatalf("post-vacuum Checkpoint() error = %v", err)
	}
}

func batchAfter(previous storage.Batch) storage.Batch {
	batch := firstBatch()
	batch.BatchID = "00000000-0000-4000-8000-000000000103"
	expected := previous.NewState.LastTotals
	batch.ExpectedOldTotals = &expected
	batch.NewState.LastTotals = storage.ByteTotals{
		Upload:   expected.Upload + batch.Global.UploadBytes,
		Download: expected.Download + batch.Global.DownloadBytes,
	}
	batch.NewState.LastSampleAt = previous.NewState.LastSampleAt + 10
	batch.Global.BucketStart = previous.Global.BucketStart + 10
	batch.Global.BucketEnd = previous.Global.BucketEnd + 10
	batch.NewRuntimeSession = nil
	batch.EndRuntimeSession = nil
	batch.QualityEvents = nil
	return batch
}
