package storage_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Willxup/flowlens/internal/storage"
)

func TestSQLiteMaintenancePrimitivesAndRunRoundTrip(t *testing.T) {
	store, _ := migratedTestStore(t)
	seedTenSecondRollups(t, store)
	if err := store.Checkpoint(context.Background(), false); err != nil {
		t.Fatalf("passive Checkpoint() error = %v", err)
	}
	if err := store.Checkpoint(context.Background(), true); err != nil {
		t.Fatalf("truncate Checkpoint() error = %v", err)
	}
	if err := store.Optimize(context.Background()); err != nil {
		t.Fatalf("Optimize() error = %v", err)
	}
	if err := store.IncrementalVacuum(context.Background(), 1); err != nil {
		t.Fatalf("IncrementalVacuum() error = %v", err)
	}

	endedAt := firstBucketAt + 10
	run := storage.MaintenanceRun{
		Operation:     "rollup_cleanup",
		StartedAt:     firstBucketAt,
		EndedAt:       &endedAt,
		DeletedRows:   6,
		DatabaseBytes: 4096,
		WALBytes:      1024,
	}
	if err := store.RecordMaintenance(context.Background(), run); err != nil {
		t.Fatalf("RecordMaintenance() error = %v", err)
	}
	got, found, err := store.LatestMaintenance(context.Background(), run.Operation)
	if err != nil || !found || !reflect.DeepEqual(got, run) {
		t.Fatalf("LatestMaintenance() = %#v, %t, %v; want %#v", got, found, err, run)
	}
	if missing, found, err := store.LatestMaintenance(context.Background(), "backup"); err != nil || found {
		t.Fatalf("missing LatestMaintenance() = %#v, %t, %v", missing, found, err)
	}

	detail := "fixture maintenance failure"
	failed := run
	failed.StartedAt += 20
	failed.EndedAt = nil
	failed.Error = &detail
	if err := store.RecordMaintenance(context.Background(), failed); err != nil {
		t.Fatalf("failed RecordMaintenance() error = %v", err)
	}
	got, found, err = store.LatestMaintenance(context.Background(), run.Operation)
	if err != nil || !found || !reflect.DeepEqual(got, failed) {
		t.Fatalf("failed LatestMaintenance() = %#v, %t, %v; want %#v", got, found, err, failed)
	}
}

func TestSQLiteMaintenanceRejectsInvalidInputs(t *testing.T) {
	store, _ := migratedTestStore(t)
	for _, pages := range []int64{0, -1, 1025} {
		if err := store.IncrementalVacuum(context.Background(), pages); !errors.Is(err, storage.ErrInvalidMaintenance) {
			t.Errorf("IncrementalVacuum(%d) error = %v", pages, err)
		}
	}
	tests := []storage.MaintenanceRun{
		{},
		{Operation: strings.Repeat("x", 65), StartedAt: 1},
		{Operation: "fixture", StartedAt: 0},
		{Operation: "fixture", StartedAt: 2, EndedAt: int64Pointer(1)},
		{Operation: "fixture", StartedAt: 1, DeletedRows: -1},
		{Operation: "fixture", StartedAt: 1, DatabaseBytes: -1},
		{Operation: "fixture", StartedAt: 1, WALBytes: -1},
		{Operation: "fixture", StartedAt: 1, Error: stringPointer(strings.Repeat("x", 4097))},
	}
	for index, run := range tests {
		if err := store.RecordMaintenance(context.Background(), run); !errors.Is(err, storage.ErrInvalidMaintenance) {
			t.Errorf("RecordMaintenance(case %d) error = %v", index, err)
		}
	}
	if _, _, err := store.LatestMaintenance(context.Background(), ""); !errors.Is(err, storage.ErrInvalidMaintenance) {
		t.Fatalf("LatestMaintenance(empty) error = %v", err)
	}
}

func TestCleanupMaintenanceRemovesOnlyRowsOlderThanCutoff(t *testing.T) {
	store, databasePath := migratedTestStore(t)
	for _, startedAt := range []int64{100, 200, 300} {
		if err := store.RecordMaintenance(context.Background(), storage.MaintenanceRun{
			Operation: "fixture", StartedAt: startedAt,
		}); err != nil {
			t.Fatalf("RecordMaintenance(%d) error = %v", startedAt, err)
		}
	}
	cleaner, ok := any(store).(interface {
		CleanupMaintenance(context.Context, int64) (int64, error)
	})
	if !ok {
		t.Fatal("Store does not support maintenance retention")
	}
	deleted, err := cleaner.CleanupMaintenance(context.Background(), 200)
	if err != nil || deleted != 1 {
		t.Fatalf("CleanupMaintenance() = %d, %v", deleted, err)
	}
	database := openRawDatabase(t, databasePath)
	var remaining int64
	if err := database.QueryRow(`SELECT COUNT(*) FROM maintenance_run`).Scan(&remaining); err != nil {
		t.Fatalf("count maintenance rows: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("remaining maintenance rows = %d, want 2", remaining)
	}
}

func TestMaintenanceRunFormattingRedactsStoredDetail(t *testing.T) {
	detail := "fixture maintenance failure"
	run := storage.MaintenanceRun{Operation: "fixture", StartedAt: 1, Error: &detail}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		formatted := fmt.Sprintf(format, run)
		if strings.Contains(formatted, detail) || strings.Contains(formatted, run.Operation) {
			t.Errorf("fmt.Sprintf(%q) leaked maintenance content: %s", format, formatted)
		}
	}
}

func int64Pointer(value int64) *int64    { return &value }
func stringPointer(value string) *string { return &value }
