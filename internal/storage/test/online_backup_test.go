package storage_test

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOnlineBackupCreatesIndependentConsistentSnapshot(t *testing.T) {
	store, _ := migratedTestStore(t)
	commitBatch(t, store, firstBatch())
	destination := filepath.Join(t.TempDir(), "snapshot.db")

	if err := store.OnlineBackup(context.Background(), destination); err != nil {
		t.Fatalf("OnlineBackup() error = %v", err)
	}
	snapshot := openRawDatabase(t, destination)
	var quickCheck string
	if err := snapshot.QueryRow(`PRAGMA quick_check`).Scan(&quickCheck); err != nil || quickCheck != "ok" {
		t.Fatalf("snapshot quick_check = %q, %v", quickCheck, err)
	}
	var trafficRows, stateRows int64
	if err := snapshot.QueryRow(`SELECT COUNT(*) FROM traffic_rollup`).Scan(&trafficRows); err != nil {
		t.Fatalf("count snapshot traffic: %v", err)
	}
	if err := snapshot.QueryRow(`SELECT COUNT(*) FROM collector_state`).Scan(&stateRows); err != nil {
		t.Fatalf("count snapshot state: %v", err)
	}
	if trafficRows != 1 || stateRows != 1 {
		t.Fatalf("snapshot rows = traffic:%d state:%d", trafficRows, stateRows)
	}
	commitBatch(t, store, nextBatch())
	if err := snapshot.QueryRow(`SELECT COUNT(*) FROM traffic_rollup`).Scan(&trafficRows); err != nil {
		t.Fatalf("recount snapshot traffic: %v", err)
	}
	if trafficRows != 1 {
		t.Fatalf("snapshot changed after source commit: %d", trafficRows)
	}
}

func TestOnlineBackupRejectsInvalidOrExistingDestination(t *testing.T) {
	store, _ := migratedTestStore(t)
	if err := store.OnlineBackup(context.Background(), "relative.db"); err == nil {
		t.Fatal("OnlineBackup() accepted relative destination")
	}
	destination := filepath.Join(t.TempDir(), "snapshot.db")
	if err := store.OnlineBackup(context.Background(), destination); err != nil {
		t.Fatalf("first OnlineBackup() error = %v", err)
	}
	if err := store.OnlineBackup(context.Background(), destination); err == nil {
		t.Fatal("OnlineBackup() overwrote existing destination")
	}
}

func TestOnlineBackupHonorsCanceledContext(t *testing.T) {
	store, _ := migratedTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.OnlineBackup(ctx, filepath.Join(t.TempDir(), "snapshot.db")); err == nil {
		t.Fatal("OnlineBackup() error = nil with canceled context")
	}
}
