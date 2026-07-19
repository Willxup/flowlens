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
	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/collector"
	flowstatus "github.com/Willxup/flowlens/internal/status"
	"github.com/Willxup/flowlens/internal/storage"
	_ "modernc.org/sqlite"
)

const runtimeBucketStart = int64(1767225600)

func TestRuntimeEnablesAttributionFromLaterCapabilities(t *testing.T) {
	snapshots := []clashapi.ConnectionsSnapshot{
		{UploadTotal: 1000, DownloadTotal: 4000, Connections: []clashapi.Connection{{
			Upload: 100, Download: 200,
			Metadata: clashapi.Metadata{DestinationIP: "198.51.100.1", DestinationPort: "443", Network: "tcp"},
		}}},
		{UploadTotal: 1005, DownloadTotal: 4005, Connections: []clashapi.Connection{{
			ID: "fixture-runtime-id", Upload: 105, Download: 205,
			Metadata: clashapi.Metadata{SourceIP: "192.0.2.10", DestinationIP: "198.51.100.1", DestinationPort: "443", Network: "tcp", Host: "api.example.test"},
		}}},
		{UploadTotal: 1015, DownloadTotal: 4025, Connections: []clashapi.Connection{{
			ID: "fixture-runtime-id", Upload: 115, Download: 225,
			Metadata: clashapi.Metadata{SourceIP: "192.0.2.10", DestinationIP: "198.51.100.1", DestinationPort: "443", Network: "tcp", Host: "api.example.test"},
		}}},
	}
	client, closeServer := runtimeClient(t, snapshots)
	defer closeServer()
	store, _ := runtimeStore(t)
	ring, _ := collector.NewRing(collector.DefaultRingCapacity)
	flowStatus := flowstatus.NewTracker()
	attributionTracker, err := attribution.NewTracker(attribution.Options{
		TopK: 20, SourceMode: attribution.SourcePrefix, IPv4Prefix: 24, IPv6Prefix: 64,
	})
	if err != nil {
		t.Fatalf("attribution.NewTracker() error = %v", err)
	}
	runtime, err := app.NewRuntime(context.Background(), app.RuntimeOptions{
		Client: client, Store: store, Ring: ring, Status: flowStatus,
		BucketTimezone: "Asia/Shanghai", ConnectionsInterval: time.Second,
		Attribution: attributionTracker, TopK: 20,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	for _, second := range []int64{1, 11, 21} {
		observeAndSeal(t, runtime, runtimeBucketStart+second)
	}
	flows, err := store.FlowRollups(context.Background(), 10, runtimeBucketStart+20)
	if err != nil {
		t.Fatalf("FlowRollups() error = %v", err)
	}
	if len(flows) != 3 || flows[0].Dimension.ClassificationCode != 1 ||
		flows[0].UploadBytes != 10 || flows[0].DownloadBytes != 20 {
		t.Fatalf("attributed flows = %#v", flows)
	}
	capabilities := attributionTracker.Capabilities()
	if !capabilities.ConnectionID || !capabilities.SourceIP || !capabilities.DestinationIP ||
		!capabilities.DestinationPort || !capabilities.Network || !capabilities.Host {
		t.Errorf("Capabilities() = %#v", capabilities)
	}
	live := attributionTracker.Snapshot()
	if len(live.Targets) != 1 || live.Targets[0].RawEndpoint != "198.51.100.1:443" {
		t.Errorf("Snapshot() = %#v", live)
	}
}

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

func TestRuntimePersistsResetBeforeInitialBucketSealWithoutLosingBytes(t *testing.T) {
	client, closeServer := runtimeClient(t, []clashapi.ConnectionsSnapshot{
		{UploadTotal: 1000, DownloadTotal: 4000},
		{UploadTotal: 40, DownloadTotal: 100},
		{UploadTotal: 50, DownloadTotal: 130},
	})
	defer closeServer()
	store, _ := runtimeStore(t)
	ring, _ := collector.NewRing(collector.DefaultRingCapacity)
	runtime := newRuntime(t, client, store, ring, flowstatus.NewTracker())

	for _, second := range []int64{1, 2, 3} {
		if err := runtime.ObserveConnections(
			context.Background(), time.Unix(runtimeBucketStart+second, 0), false,
		); err != nil {
			t.Fatalf("ObserveConnections(second %d) error = %v", second, err)
		}
	}
	if err := runtime.Seal(context.Background(), time.Unix(runtimeBucketStart+10, 0)); err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	rollup := mustRollup(t, store, runtimeBucketStart)
	if rollup.UploadBytes != 50 || rollup.DownloadBytes != 130 || rollup.ResetCount != 1 {
		t.Errorf("reset rollup = %#v", rollup)
	}
	state := mustState(t, store)
	if state.LastTotals != (storage.ByteTotals{Upload: 50, Download: 130}) {
		t.Errorf("reset state = %#v", state)
	}
	session, found, err := store.RuntimeSession(context.Background(), state.RuntimeSessionID)
	if err != nil || !found || session.StartReason != "counter_reset" {
		t.Errorf("RuntimeSession() = %#v, %t, %v", session, found, err)
	}
}

func TestRuntimeDoesNotAdvanceCounterWhenWallClockMovesBeforeCurrentBucket(t *testing.T) {
	client, closeServer := runtimeClient(t, []clashapi.ConnectionsSnapshot{
		{UploadTotal: 1000, DownloadTotal: 4000},
		{UploadTotal: 1100, DownloadTotal: 4300},
		{UploadTotal: 1200, DownloadTotal: 4500},
	})
	defer closeServer()
	store, _ := runtimeStore(t)
	ring, _ := collector.NewRing(collector.DefaultRingCapacity)
	runtime := newRuntime(t, client, store, ring, flowstatus.NewTracker())

	if err := runtime.ObserveConnections(
		context.Background(), time.Unix(runtimeBucketStart+11, 0), false,
	); err != nil {
		t.Fatalf("baseline ObserveConnections() error = %v", err)
	}
	if err := runtime.ObserveConnections(
		context.Background(), time.Unix(runtimeBucketStart+1, 0), false,
	); err == nil {
		t.Fatal("backward-clock ObserveConnections() error = nil")
	}
	if err := runtime.ObserveConnections(
		context.Background(), time.Unix(runtimeBucketStart+12, 0), true,
	); err != nil {
		t.Fatalf("recovered ObserveConnections() error = %v", err)
	}
	if err := runtime.Seal(context.Background(), time.Unix(runtimeBucketStart+20, 0)); err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	rollup := mustRollup(t, store, runtimeBucketStart+10)
	if rollup.UploadBytes != 200 || rollup.DownloadBytes != 500 ||
		rollup.RecoveredUploadBytes != 200 || rollup.RecoveredDownloadBytes != 500 {
		t.Fatalf("recovered rollup = %#v", rollup)
	}
	state := mustState(t, store)
	if state.LastTotals != (storage.ByteTotals{Upload: 1200, Download: 4500}) {
		t.Fatalf("collector state = %#v", state)
	}
}

func TestRuntimeRetainsTrafficSampleUntilMatchingCounterBucketExists(t *testing.T) {
	client, closeServer := runtimeClient(t, []clashapi.ConnectionsSnapshot{
		{UploadTotal: 1000, DownloadTotal: 4000},
		{UploadTotal: 1100, DownloadTotal: 4300},
	})
	defer closeServer()
	store, _ := runtimeStore(t)
	ring, _ := collector.NewRing(collector.DefaultRingCapacity)
	runtime := newRuntime(t, client, store, ring, flowstatus.NewTracker())

	if err := runtime.ObserveConnections(
		context.Background(), time.Unix(runtimeBucketStart+1, 0), false,
	); err != nil {
		t.Fatalf("baseline ObserveConnections() error = %v", err)
	}
	if err := runtime.ObserveTraffic(time.Unix(runtimeBucketStart+11, 0), clashapi.TrafficSample{
		Up: 30, Down: 40,
	}); err != nil {
		t.Fatalf("boundary ObserveTraffic() error = %v", err)
	}
	if err := runtime.ObserveConnections(
		context.Background(), time.Unix(runtimeBucketStart+11, 0), false,
	); err != nil {
		t.Fatalf("next-bucket ObserveConnections() error = %v", err)
	}
	if err := runtime.Seal(context.Background(), time.Unix(runtimeBucketStart+20, 0)); err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	rollup := mustRollup(t, store, runtimeBucketStart+10)
	if rollup.SpeedSampleCount != 1 || rollup.SpeedUploadSampleSum != 30 ||
		rollup.SpeedDownloadSampleSum != 40 || rollup.PeakUploadBytesPerSecond != 30 ||
		rollup.PeakDownloadBytesPerSecond != 40 {
		t.Fatalf("boundary speed rollup = %#v", rollup)
	}
}

func TestRuntimeRunCommitsAlreadyCompleteBucketOnCancellation(t *testing.T) {
	client, closeServer := runtimeClient(t, []clashapi.ConnectionsSnapshot{
		{UploadTotal: 1000, DownloadTotal: 4000},
	})
	defer closeServer()
	store, _ := runtimeStore(t)
	ring, _ := collector.NewRing(collector.DefaultRingCapacity)
	runtime := newRuntime(t, client, store, ring, flowstatus.NewTracker())

	if err := runtime.ObserveConnections(
		context.Background(), time.Unix(runtimeBucketStart+1, 0), false,
	); err != nil {
		t.Fatalf("ObserveConnections() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runtime.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rollup := mustRollup(t, store, runtimeBucketStart)
	if rollup.UploadBytes != 0 || rollup.DownloadBytes != 0 {
		t.Fatalf("shutdown rollup = %#v", rollup)
	}
}

func TestRuntimePersistsOnlyExceptionalQualityEvents(t *testing.T) {
	client, closeServer := runtimeClient(t, []clashapi.ConnectionsSnapshot{
		{UploadTotal: 1000, DownloadTotal: 4000},
		{UploadTotal: 1100, DownloadTotal: 4300},
		{UploadTotal: 1200, DownloadTotal: 4600},
	})
	defer closeServer()
	store, _ := runtimeStore(t)
	ring, _ := collector.NewRing(collector.DefaultRingCapacity)
	runtime := newRuntime(t, client, store, ring, flowstatus.NewTracker())

	observeAndSeal(t, runtime, runtimeBucketStart+1)
	observeAndSeal(t, runtime, runtimeBucketStart+11)
	ordinary, err := store.QualityEvents(
		context.Background(), runtimeBucketStart+10, runtimeBucketStart+20,
	)
	if err != nil {
		t.Fatalf("ordinary QualityEvents() error = %v", err)
	}
	if len(ordinary) != 0 {
		t.Fatalf("ordinary QualityEvents() = %#v", ordinary)
	}

	if err := runtime.ObserveConnections(
		context.Background(), time.Unix(runtimeBucketStart+21, 0), true,
	); err != nil {
		t.Fatalf("gap ObserveConnections() error = %v", err)
	}
	if err := runtime.Seal(context.Background(), time.Unix(runtimeBucketStart+30, 0)); err != nil {
		t.Fatalf("gap Seal() error = %v", err)
	}
	exceptional, err := store.QualityEvents(
		context.Background(), runtimeBucketStart+20, runtimeBucketStart+30,
	)
	if err != nil {
		t.Fatalf("exceptional QualityEvents() error = %v", err)
	}
	if len(exceptional) != 1 || exceptional[0].Flags&collector.QualityFlagGap == 0 {
		t.Fatalf("exceptional QualityEvents() = %#v", exceptional)
	}
}

func TestRuntimeRunMarksFirstPersistedObservationAsGap(t *testing.T) {
	seedClient, closeSeedServer := runtimeClient(t, []clashapi.ConnectionsSnapshot{
		{UploadTotal: 1000, DownloadTotal: 4000},
	})
	defer closeSeedServer()
	store, _ := runtimeStore(t)
	ring, _ := collector.NewRing(collector.DefaultRingCapacity)
	seedRuntime := newRuntime(t, seedClient, store, ring, flowstatus.NewTracker())
	observeAndSeal(t, seedRuntime, runtimeBucketStart+1)

	observedBucket := make(chan int64, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer fixture-clash-secret" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/version":
			_ = json.NewEncoder(writer).Encode(map[string]any{"version": "sing-box 1.12.0-fixture"})
		case "/connections":
			_ = json.NewEncoder(writer).Encode(clashapi.ConnectionsSnapshot{
				UploadTotal: 1100, DownloadTotal: 4300,
			})
			select {
			case observedBucket <- time.Now().UTC().Unix() / 10 * 10:
			default:
			}
		case "/traffic":
			flusher, _ := writer.(http.Flusher)
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				_, _ = writer.Write([]byte("{\"up\":1,\"down\":2}\n"))
				if flusher != nil {
					flusher.Flush()
				}
				select {
				case <-request.Context().Done():
					return
				case <-ticker.C:
				}
			}
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := clashapi.New(clashapi.Options{
		BaseURL: server.URL, Secret: "fixture-clash-secret", RequestTimeout: time.Second, MaxResponseSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("clashapi.New() error = %v", err)
	}
	tracker := flowstatus.NewTracker()
	restarted := newRuntimeWithInterval(t, client, store, ring, tracker, 25*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	go func() { runResult <- restarted.Run(ctx) }()

	var bucketStart int64
	select {
	case bucketStart = <-observedBucket:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("runtime did not poll persisted counter")
	}
	waitStatusLevel(t, tracker, flowstatus.LevelOK)
	cancel()
	if err := <-runResult; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := restarted.Seal(context.Background(), time.Unix(bucketStart+20, 0)); err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	rollup := mustRollupNear(t, store, bucketStart)
	if rollup.UploadBytes != 100 || rollup.DownloadBytes != 300 ||
		rollup.RecoveredUploadBytes != 100 || rollup.RecoveredDownloadBytes != 300 ||
		rollup.QualityFlags&collector.QualityFlagGap == 0 {
		t.Errorf("restart rollup = %#v", rollup)
	}
}

func TestRuntimeRunDoesNotTreatTrafficStreamErrorAsCounterGap(t *testing.T) {
	connectionBuckets := make(chan int64, 4)
	releaseTrafficError := make(chan struct{})
	var mutex sync.Mutex
	connectionsCall := 0
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
			connectionsCall++
			call := connectionsCall
			mutex.Unlock()
			snapshot := clashapi.ConnectionsSnapshot{UploadTotal: 1000, DownloadTotal: 4000}
			if call > 1 {
				snapshot = clashapi.ConnectionsSnapshot{UploadTotal: 1100, DownloadTotal: 4300}
			}
			_ = json.NewEncoder(writer).Encode(snapshot)
			connectionBuckets <- time.Now().UTC().Unix() / 10 * 10
		case "/traffic":
			select {
			case <-releaseTrafficError:
			case <-request.Context().Done():
			}
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := clashapi.New(clashapi.Options{
		BaseURL: server.URL, Secret: "fixture-clash-secret", RequestTimeout: time.Second, MaxResponseSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("clashapi.New() error = %v", err)
	}
	store, _ := runtimeStore(t)
	ring, _ := collector.NewRing(collector.DefaultRingCapacity)
	tracker := flowstatus.NewTracker()
	runtime := newRuntimeWithInterval(t, client, store, ring, tracker, 200*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	go func() { runResult <- runtime.Run(ctx) }()

	select {
	case <-connectionBuckets:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("runtime did not establish counter baseline")
	}
	waitStatusLevel(t, tracker, flowstatus.LevelOK)
	close(releaseTrafficError)
	waitStatusLevel(t, tracker, flowstatus.LevelDegraded)
	var secondBucketStart int64
	select {
	case secondBucketStart = <-connectionBuckets:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("runtime did not poll counter after traffic error")
	}
	waitStatusLevel(t, tracker, flowstatus.LevelOK)
	cancel()
	if err := <-runResult; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := runtime.Seal(context.Background(), time.Unix(secondBucketStart+20, 0)); err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	rollup := mustRollup(t, store, secondBucketStart)
	if rollup.UploadBytes != 100 || rollup.DownloadBytes != 300 ||
		rollup.RecoveredUploadBytes != 0 || rollup.RecoveredDownloadBytes != 0 ||
		rollup.QualityFlags&collector.QualityFlagGap != 0 {
		t.Errorf("traffic recovery rollup = %#v", rollup)
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
	return newRuntimeWithInterval(t, client, store, ring, tracker, time.Second)
}

func newRuntimeWithInterval(
	t *testing.T,
	client *clashapi.Client,
	store *storage.Store,
	ring *collector.Ring,
	tracker *flowstatus.Tracker,
	interval time.Duration,
) *app.Runtime {
	t.Helper()
	attributionTracker, err := attribution.NewTracker(attribution.Options{
		TopK: 20, SourceMode: attribution.SourcePrefix, IPv4Prefix: 24, IPv6Prefix: 64,
	})
	if err != nil {
		t.Fatalf("attribution.NewTracker() error = %v", err)
	}
	runtime, err := app.NewRuntime(context.Background(), app.RuntimeOptions{
		Client: client, Store: store, Ring: ring, Status: tracker,
		BucketTimezone: "Asia/Shanghai", ConnectionsInterval: interval,
		Attribution: attributionTracker, TopK: 20,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	return runtime
}

func waitStatusLevel(t *testing.T, tracker *flowstatus.Tracker, level flowstatus.Level) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if tracker.Snapshot().Level == level {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("status level = %q, want %q", tracker.Snapshot().Level, level)
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

func mustRollupNear(t *testing.T, store *storage.Store, start int64) storage.TrafficRollup {
	t.Helper()
	for _, candidate := range []int64{start - 10, start, start + 10} {
		rollup, found, err := store.TrafficRollup(context.Background(), 10, candidate)
		if err != nil {
			t.Fatalf("TrafficRollup(%d) error = %v", candidate, err)
		}
		if found {
			return rollup
		}
	}
	t.Fatalf("TrafficRollup near %d was not found", start)
	return storage.TrafficRollup{}
}
