package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/app"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/collector"
	flowstatus "github.com/Willxup/flowlens/internal/status"
	"github.com/Willxup/flowlens/internal/storage"
	_ "modernc.org/sqlite"
)

const runtimeBucketStart = int64(1767225600)

func TestRuntimePersistsBaselineNormalGapAndResetTransitions(t *testing.T) {
	snapshots := []clashapi.ConnectionsSnapshot{
		{UploadTotal: 1000, DownloadTotal: 4000, Connections: make([]clashapi.Connection, 2)},
		{UploadTotal: 1250, DownloadTotal: 4750, Connections: make([]clashapi.Connection, 3)},
		{UploadTotal: 1300, DownloadTotal: 4800, Connections: make([]clashapi.Connection, 1)},
		{UploadTotal: 40, DownloadTotal: 100, Connections: make([]clashapi.Connection, 1)},
	}
	client, closeServer := runtimeClient(t, snapshots)
	defer closeServer()
	store, _ := runtimeStore(t)
	tracker := flowstatus.NewTracker()
	ring, _ := collector.NewRing(collector.DefaultRingCapacity)

	runtime := newRuntime(t, client, store, ring, tracker)
	observeAndSeal(t, runtime, runtimeBucketStart+1)
	baseline := mustRollup(t, store, runtimeBucketStart)
	if baseline.UploadBytes != 0 || baseline.DownloadBytes != 0 {
		t.Errorf("baseline rollup = %#v", baseline)
	}
	firstState := mustState(t, store)
	if firstState.LastTotals != (storage.ByteTotals{Upload: 1000, Download: 4000}) {
		t.Errorf("baseline state = %#v", firstState)
	}

	observeAndSeal(t, runtime, runtimeBucketStart+11)
	normal := mustRollup(t, store, runtimeBucketStart+10)
	if normal.UploadBytes != 250 || normal.DownloadBytes != 750 ||
		normal.UnattributedUploadBytes != 250 || normal.UnattributedDownloadBytes != 750 {
		t.Errorf("normal rollup = %#v", normal)
	}

	restarted := newRuntime(t, client, store, ring, tracker)
	if err := restarted.ObserveConnections(context.Background(), time.Unix(runtimeBucketStart+21, 0), true); err != nil {
		t.Fatalf("gap ObserveConnections() error = %v", err)
	}
	if err := restarted.Seal(context.Background(), time.Unix(runtimeBucketStart+30, 0)); err != nil {
		t.Fatalf("gap Seal() error = %v", err)
	}
	recovered := mustRollup(t, store, runtimeBucketStart+20)
	if recovered.UploadBytes != 50 || recovered.DownloadBytes != 50 ||
		recovered.RecoveredUploadBytes != 50 || recovered.RecoveredDownloadBytes != 50 ||
		recovered.QualityFlags&collector.QualityFlagGap == 0 {
		t.Errorf("recovered rollup = %#v", recovered)
	}

	previousSession := mustState(t, store).RuntimeSessionID
	if err := restarted.ObserveConnections(context.Background(), time.Unix(runtimeBucketStart+31, 0), false); err != nil {
		t.Fatalf("reset ObserveConnections() error = %v", err)
	}
	if err := restarted.Seal(context.Background(), time.Unix(runtimeBucketStart+40, 0)); err != nil {
		t.Fatalf("reset Seal() error = %v", err)
	}
	reset := mustRollup(t, store, runtimeBucketStart+30)
	if reset.UploadBytes != 40 || reset.DownloadBytes != 100 || reset.ResetCount != 1 {
		t.Errorf("reset rollup = %#v", reset)
	}
	state := mustState(t, store)
	if state.RuntimeSessionID == previousSession {
		t.Error("counter reset did not change runtime session")
	}
	oldSession, found, err := store.RuntimeSession(context.Background(), previousSession)
	if err != nil || !found || oldSession.EndedAt == nil {
		t.Errorf("old RuntimeSession() = %#v, %t, %v", oldSession, found, err)
	}
}

func TestRuntimeRetriesIdenticalPendingBatchWithoutAdvancingCursor(t *testing.T) {
	client, closeServer := runtimeClient(t, []clashapi.ConnectionsSnapshot{
		{UploadTotal: 1000, DownloadTotal: 4000},
		{UploadTotal: 1100, DownloadTotal: 4400},
	})
	defer closeServer()
	store, databasePath := runtimeStore(t)
	ring, _ := collector.NewRing(collector.DefaultRingCapacity)
	runtime := newRuntime(t, client, store, ring, flowstatus.NewTracker())
	observeAndSeal(t, runtime, runtimeBucketStart+1)
	before := mustState(t, store)
	if err := runtime.ObserveConnections(context.Background(), time.Unix(runtimeBucketStart+11, 0), false); err != nil {
		t.Fatalf("ObserveConnections() error = %v", err)
	}

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(`
		CREATE TRIGGER fixture_runtime_abort BEFORE UPDATE ON collector_state
		BEGIN SELECT RAISE(ABORT, 'fixture-only abort'); END
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if err := runtime.Seal(context.Background(), time.Unix(runtimeBucketStart+20, 0)); err == nil {
		t.Fatal("Seal() error = nil with abort trigger")
	}
	if after := mustState(t, store); after != before {
		t.Errorf("state advanced after failure: %#v, want %#v", after, before)
	}
	if _, found, err := store.TrafficRollup(context.Background(), 10, runtimeBucketStart+10); err != nil || found {
		t.Errorf("failed rollup = found:%t err:%v", found, err)
	}
	if _, err := database.Exec(`DROP TRIGGER fixture_runtime_abort`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	if err := runtime.Seal(context.Background(), time.Unix(runtimeBucketStart+20, 0)); err != nil {
		t.Fatalf("retry Seal() error = %v", err)
	}
	committed := mustState(t, store)
	if committed.LastTotals != (storage.ByteTotals{Upload: 1100, Download: 4400}) || committed.LastBatchID == before.LastBatchID {
		t.Errorf("retry state = %#v", committed)
	}
	batchID := committed.LastBatchID
	if err := runtime.Seal(context.Background(), time.Unix(runtimeBucketStart+20, 0)); err != nil {
		t.Fatalf("idempotent Seal() error = %v", err)
	}
	if again := mustState(t, store); again.LastBatchID != batchID {
		t.Errorf("idempotent state = %#v", again)
	}
}

func runtimeClient(t *testing.T, snapshots []clashapi.ConnectionsSnapshot) (*clashapi.Client, func()) {
	t.Helper()
	var mutex sync.Mutex
	index := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer fixture-clash-secret" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/version":
			_ = json.NewEncoder(writer).Encode(map[string]any{"version": "sing-box 1.12.0-fixture"})
		case "/connections":
			mutex.Lock()
			if index >= len(snapshots) {
				index = len(snapshots) - 1
			}
			snapshot := snapshots[index]
			index++
			mutex.Unlock()
			_ = json.NewEncoder(writer).Encode(snapshot)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	client, err := clashapi.New(clashapi.Options{
		BaseURL: server.URL, Secret: "fixture-clash-secret", RequestTimeout: time.Second, MaxResponseSize: 1 << 20,
	})
	if err != nil {
		server.Close()
		t.Fatalf("clashapi.New() error = %v", err)
	}
	return client, server.Close
}

func runtimeStore(t *testing.T) (*storage.Store, string) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	store, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return store, databasePath
}

func newRuntime(t *testing.T, client *clashapi.Client, store *storage.Store, ring *collector.Ring, tracker *flowstatus.Tracker) *app.Runtime {
	t.Helper()
	runtime, err := app.NewRuntime(context.Background(), app.RuntimeOptions{
		Client: client, Store: store, Ring: ring, Status: tracker,
		BucketTimezone: "Asia/Shanghai", ConnectionsInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	return runtime
}

func observeAndSeal(t *testing.T, runtime *app.Runtime, at int64) {
	t.Helper()
	if err := runtime.ObserveConnections(context.Background(), time.Unix(at, 0), false); err != nil {
		t.Fatalf("ObserveConnections() error = %v", err)
	}
	end := at/10*10 + 10
	if err := runtime.Seal(context.Background(), time.Unix(end, 0)); err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
}

func mustState(t *testing.T, store *storage.Store) storage.CollectorState {
	t.Helper()
	state, found, err := store.LoadCollectorState(context.Background())
	if err != nil || !found {
		t.Fatalf("LoadCollectorState() = %#v, %t, %v", state, found, err)
	}
	return state
}

func mustRollup(t *testing.T, store *storage.Store, start int64) storage.TrafficRollup {
	t.Helper()
	rollup, found, err := store.TrafficRollup(context.Background(), 10, start)
	if err != nil || !found {
		t.Fatalf("TrafficRollup(%d) = %#v, %t, %v", start, rollup, found, err)
	}
	return rollup
}
