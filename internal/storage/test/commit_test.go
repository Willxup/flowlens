package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Willxup/flowlens/internal/storage"
)

const (
	firstBatchID    = "00000000-0000-4000-8000-000000000101"
	firstSessionID  = "00000000-0000-4000-8000-000000000201"
	firstBucketAt   = int64(1767225600)
	secondBatchID   = "00000000-0000-4000-8000-000000000102"
	secondSessionID = "00000000-0000-4000-8000-000000000202"
)

func TestCommitBatchFirstBatchPersistsAtomicState(t *testing.T) {
	store, _ := migratedTestStore(t)
	batch := firstBatch()

	result, err := store.CommitBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	if result.AlreadyCommitted {
		t.Error("CommitBatch().AlreadyCommitted = true")
	}

	state, found, err := store.LoadCollectorState(context.Background())
	if err != nil {
		t.Fatalf("LoadCollectorState() error = %v", err)
	}
	wantState := storage.CollectorState{
		CollectorCursor: batch.NewState,
		LastBatchID:     batch.BatchID,
	}
	if !found || state != wantState {
		t.Errorf("LoadCollectorState() = %#v, %t; want %#v, true", state, found, wantState)
	}

	global, found, err := store.TrafficRollup(context.Background(), 10, firstBucketAt)
	if err != nil {
		t.Fatalf("TrafficRollup() error = %v", err)
	}
	if !found || !reflect.DeepEqual(global, batch.Global) {
		t.Errorf("TrafficRollup() = %#v, %t; want %#v, true", global, found, batch.Global)
	}

	flows, err := store.FlowRollups(context.Background(), 10, firstBucketAt)
	if err != nil {
		t.Fatalf("FlowRollups() error = %v", err)
	}
	assertFlowClasses(t, flows, map[int64]storage.ByteTotals{
		1: {Upload: 60, Download: 240},
		2: {Upload: 10, Download: 40},
		3: {Upload: 30, Download: 120},
	})
	flows[0].Dimension.DestinationIP[0] = 0
	flows[0].UploadBytes = 0
	freshFlows, err := store.FlowRollups(context.Background(), 10, firstBucketAt)
	if err != nil {
		t.Fatalf("second FlowRollups() error = %v", err)
	}
	if freshFlows[0].Dimension.DestinationIP[0] == 0 || freshFlows[0].UploadBytes == 0 {
		t.Error("FlowRollups() exposed mutable slice or BLOB storage")
	}

	session, found, err := store.RuntimeSession(context.Background(), firstSessionID)
	if err != nil {
		t.Fatalf("RuntimeSession() error = %v", err)
	}
	if !found || session.ID != firstSessionID || session.StartedAt != firstBucketAt ||
		session.EndedAt != nil || session.EndReason != nil ||
		session.LastTotals != batch.NewState.LastTotals ||
		session.LastSeenAt != batch.NewState.LastSampleAt ||
		session.SingBoxVersion != "sing-box 1.12.0-fixture" {
		t.Errorf("RuntimeSession() = %#v, %t", session, found)
	}

	eventCount, err := store.QualityEventCount(context.Background(), firstBatchID)
	if err != nil {
		t.Fatalf("QualityEventCount() error = %v", err)
	}
	if eventCount != 1 {
		t.Errorf("QualityEventCount() = %d", eventCount)
	}
}

func TestCommitBatchLargeIntegerRoundTrip(t *testing.T) {
	store, _ := migratedTestStore(t)
	largeUpload := int64(1<<54 + 12345)
	largeDownload := int64(1<<55 + 67890)
	batch := firstBatch()
	batch.BatchID = "00000000-0000-4000-8000-000000000102"
	batch.NewRuntimeSession.ID = "00000000-0000-4000-8000-000000000202"
	batch.NewState.RuntimeSessionID = batch.NewRuntimeSession.ID
	batch.NewState.LastTotals = storage.ByteTotals{Upload: largeUpload + 1000, Download: largeDownload + 4000}
	batch.Global.UploadBytes = largeUpload
	batch.Global.DownloadBytes = largeDownload
	batch.Global.UnattributedUploadBytes = 0
	batch.Global.UnattributedDownloadBytes = 0
	batch.Flows = []storage.FlowRollup{{
		Dimension:            attributedDimension(),
		UploadBytes:          largeUpload,
		DownloadBytes:        largeDownload,
		FlowObservationCount: 1,
	}}
	batch.QualityEvents = nil

	if _, err := store.CommitBatch(context.Background(), batch); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	state, found, err := store.LoadCollectorState(context.Background())
	if err != nil || !found {
		t.Fatalf("LoadCollectorState() = %#v, %t, %v", state, found, err)
	}
	if state.LastTotals != batch.NewState.LastTotals {
		t.Errorf("state totals = %#v, want %#v", state.LastTotals, batch.NewState.LastTotals)
	}
	global, found, err := store.TrafficRollup(context.Background(), 10, firstBucketAt)
	if err != nil || !found {
		t.Fatalf("TrafficRollup() = %#v, %t, %v", global, found, err)
	}
	if global.UploadBytes != largeUpload || global.DownloadBytes != largeDownload {
		t.Errorf("global bytes = upload:%d download:%d", global.UploadBytes, global.DownloadBytes)
	}
}

func TestBatchModelFormattingRedactsRowContent(t *testing.T) {
	batch := firstBatch()
	values := map[string]any{
		"totals":          batch.NewState.LastTotals,
		"cursor":          batch.NewState,
		"state":           storage.CollectorState{CollectorCursor: batch.NewState, LastBatchID: batch.BatchID},
		"global":          batch.Global,
		"dimension":       batch.Flows[0].Dimension,
		"flow":            batch.Flows[0],
		"session start":   *batch.NewRuntimeSession,
		"session end":     storage.RuntimeSessionEnd{ID: firstSessionID, EndedAt: firstBucketAt + 10, EndReason: "fixture_end"},
		"runtime session": storage.RuntimeSession{ID: firstSessionID, StartReason: "startup", SingBoxVersion: "sing-box 1.12.0-fixture"},
		"quality event":   batch.QualityEvents[0],
		"batch":           batch,
	}
	for name, value := range values {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(format, value)
			for _, forbidden := range []string{firstSessionID, "api.example.test", "fixture-only detail", "Asia/Shanghai", "1100"} {
				if strings.Contains(formatted, forbidden) {
					t.Errorf("%s fmt.Sprintf(%q) contains %q: %s", name, format, forbidden, formatted)
				}
			}
		}
	}
}

func TestCommitBatchRejectsInvalidBatchBeforeSQL(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*storage.Batch)
	}{
		{name: "empty batch id", mutate: func(batch *storage.Batch) { batch.BatchID = "" }},
		{name: "long batch id", mutate: func(batch *storage.Batch) { batch.BatchID = strings.Repeat("b", 129) }},
		{name: "empty session id", mutate: func(batch *storage.Batch) { batch.NewState.RuntimeSessionID = "" }},
		{name: "long session id", mutate: func(batch *storage.Batch) { batch.NewState.RuntimeSessionID = strings.Repeat("s", 129) }},
		{name: "empty timezone", mutate: func(batch *storage.Batch) { batch.NewState.BucketTimezone = "" }},
		{name: "long timezone", mutate: func(batch *storage.Batch) { batch.NewState.BucketTimezone = strings.Repeat("t", 129) }},
		{name: "nonpositive sample time", mutate: func(batch *storage.Batch) { batch.NewState.LastSampleAt = 0 }},
		{name: "negative new upload total", mutate: func(batch *storage.Batch) { batch.NewState.LastTotals.Upload = -1 }},
		{name: "negative expected total", mutate: func(batch *storage.Batch) {
			batch.ExpectedOldTotals = &storage.ByteTotals{Upload: -1}
		}},
		{name: "non ten second resolution", mutate: func(batch *storage.Batch) { batch.Global.ResolutionSec = 60 }},
		{name: "nonpositive bucket start", mutate: func(batch *storage.Batch) { batch.Global.BucketStart = 0 }},
		{name: "wrong bucket end", mutate: func(batch *storage.Batch) { batch.Global.BucketEnd++ }},
		{name: "sample before bucket", mutate: func(batch *storage.Batch) { batch.NewState.LastSampleAt = batch.Global.BucketStart - 1 }},
		{name: "sample at bucket end", mutate: func(batch *storage.Batch) { batch.NewState.LastSampleAt = batch.Global.BucketEnd }},
		{name: "recovered upload exceeds global", mutate: func(batch *storage.Batch) { batch.Global.RecoveredUploadBytes = batch.Global.UploadBytes + 1 }},
		{name: "unattributed download exceeds global", mutate: func(batch *storage.Batch) { batch.Global.UnattributedDownloadBytes = batch.Global.DownloadBytes + 1 }},
		{name: "peak before bucket", mutate: func(batch *storage.Batch) {
			value := batch.Global.BucketStart - 1
			batch.Global.PeakUploadAt = &value
		}},
		{name: "too many flows", mutate: func(batch *storage.Batch) { batch.Flows = make([]storage.FlowRollup, 103) }},
		{name: "duplicate dimension", mutate: func(batch *storage.Batch) { batch.Flows = append(batch.Flows, batch.Flows[0]) }},
		{name: "invalid source family", mutate: func(batch *storage.Batch) { batch.Flows[0].Dimension.SourceFamily = 5 }},
		{name: "invalid source address length", mutate: func(batch *storage.Batch) { batch.Flows[0].Dimension.SourceNetwork = []byte{1} }},
		{name: "invalid source prefix", mutate: func(batch *storage.Batch) { batch.Flows[0].Dimension.SourcePrefixLen = 33 }},
		{name: "invalid destination family", mutate: func(batch *storage.Batch) { batch.Flows[0].Dimension.DestinationFamily = 5 }},
		{name: "invalid destination address length", mutate: func(batch *storage.Batch) { batch.Flows[0].Dimension.DestinationIP = []byte{1} }},
		{name: "invalid destination port", mutate: func(batch *storage.Batch) { batch.Flows[0].Dimension.DestinationPort = 65536 }},
		{name: "long host", mutate: func(batch *storage.Batch) { batch.Flows[0].Dimension.Host = strings.Repeat("h", 254) }},
		{name: "invalid network code", mutate: func(batch *storage.Batch) { batch.Flows[0].Dimension.NetworkCode = 3 }},
		{name: "invalid classification code", mutate: func(batch *storage.Batch) { batch.Flows[0].Dimension.ClassificationCode = 4 }},
		{name: "structured special dimension", mutate: func(batch *storage.Batch) { batch.Flows[1].Dimension.Host = "invalid.example.test" }},
		{name: "negative flow upload", mutate: func(batch *storage.Batch) { batch.Flows[0].UploadBytes = -1 }},
		{name: "negative flow download", mutate: func(batch *storage.Batch) { batch.Flows[0].DownloadBytes = -1 }},
		{name: "negative flow observations", mutate: func(batch *storage.Batch) { batch.Flows[0].FlowObservationCount = -1 }},
		{name: "upload conservation mismatch", mutate: func(batch *storage.Batch) { batch.Flows[0].UploadBytes-- }},
		{name: "download conservation mismatch", mutate: func(batch *storage.Batch) { batch.Flows[0].DownloadBytes-- }},
		{name: "unattributed conservation mismatch", mutate: func(batch *storage.Batch) {
			batch.Flows[0].UploadBytes++
			batch.Flows[2].UploadBytes--
		}},
		{name: "flow sum overflow", mutate: func(batch *storage.Batch) {
			batch.Global.UploadBytes = math.MaxInt64
			batch.Global.DownloadBytes = 0
			batch.Global.UnattributedUploadBytes = 0
			batch.Global.UnattributedDownloadBytes = 0
			first := storage.FlowRollup{Dimension: attributedDimension(), UploadBytes: math.MaxInt64}
			secondDimension := attributedDimension()
			secondDimension.Host = "overflow.example.test"
			batch.Flows = []storage.FlowRollup{first, {Dimension: secondDimension, UploadBytes: 1}}
		}},
		{name: "session start id mismatch", mutate: func(batch *storage.Batch) { batch.NewRuntimeSession.ID = "different-session" }},
		{name: "nonpositive session start", mutate: func(batch *storage.Batch) { batch.NewRuntimeSession.StartedAt = 0 }},
		{name: "session start after sample", mutate: func(batch *storage.Batch) { batch.NewRuntimeSession.StartedAt = batch.NewState.LastSampleAt + 1 }},
		{name: "empty session start reason", mutate: func(batch *storage.Batch) { batch.NewRuntimeSession.StartReason = "" }},
		{name: "long session start reason", mutate: func(batch *storage.Batch) { batch.NewRuntimeSession.StartReason = strings.Repeat("r", 65) }},
		{name: "empty sing box version", mutate: func(batch *storage.Batch) { batch.NewRuntimeSession.SingBoxVersion = "" }},
		{name: "long sing box version", mutate: func(batch *storage.Batch) { batch.NewRuntimeSession.SingBoxVersion = strings.Repeat("v", 257) }},
		{name: "empty boot id", mutate: func(batch *storage.Batch) {
			value := ""
			batch.NewRuntimeSession.HostBootID = &value
		}},
		{name: "long boot id", mutate: func(batch *storage.Batch) {
			value := strings.Repeat("i", 129)
			batch.NewRuntimeSession.HostBootID = &value
		}},
		{name: "negative gap", mutate: func(batch *storage.Batch) { batch.NewRuntimeSession.DataGapBeforeSeconds = -1 }},
		{name: "same session start and end", mutate: func(batch *storage.Batch) {
			batch.EndRuntimeSession = &storage.RuntimeSessionEnd{ID: batch.NewRuntimeSession.ID, EndedAt: batch.Global.BucketStart, EndReason: "reset"}
		}},
		{name: "nonpositive session end", mutate: func(batch *storage.Batch) {
			batch.EndRuntimeSession = &storage.RuntimeSessionEnd{ID: "old-session", EndedAt: 0, EndReason: "reset"}
		}},
		{name: "long session end id", mutate: func(batch *storage.Batch) {
			batch.EndRuntimeSession = &storage.RuntimeSessionEnd{ID: strings.Repeat("e", 129), EndedAt: batch.Global.BucketStart, EndReason: "reset"}
		}},
		{name: "empty session end reason", mutate: func(batch *storage.Batch) {
			batch.EndRuntimeSession = &storage.RuntimeSessionEnd{ID: "old-session", EndedAt: batch.Global.BucketStart}
		}},
		{name: "long session end reason", mutate: func(batch *storage.Batch) {
			batch.EndRuntimeSession = &storage.RuntimeSessionEnd{ID: "old-session", EndedAt: batch.Global.BucketStart, EndReason: strings.Repeat("e", 65)}
		}},
		{name: "session end after new start", mutate: func(batch *storage.Batch) {
			batch.EndRuntimeSession = &storage.RuntimeSessionEnd{ID: "old-session", EndedAt: batch.NewRuntimeSession.StartedAt + 1, EndReason: "reset"}
		}},
		{name: "empty quality code", mutate: func(batch *storage.Batch) { batch.QualityEvents[0].Code = "" }},
		{name: "long quality code", mutate: func(batch *storage.Batch) { batch.QualityEvents[0].Code = strings.Repeat("q", 65) }},
		{name: "nonpositive quality start", mutate: func(batch *storage.Batch) { batch.QualityEvents[0].StartedAt = 0 }},
		{name: "negative quality flags", mutate: func(batch *storage.Batch) { batch.QualityEvents[0].Flags = -1 }},
		{name: "long quality detail", mutate: func(batch *storage.Batch) { batch.QualityEvents[0].Detail = strings.Repeat("d", 4097) }},
		{name: "quality end before start", mutate: func(batch *storage.Batch) {
			value := batch.QualityEvents[0].StartedAt - 1
			batch.QualityEvents[0].EndedAt = &value
		}},
		{name: "too many quality events", mutate: func(batch *storage.Batch) { batch.QualityEvents = make([]storage.QualityEvent, 129) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, _ := migratedTestStore(t)
			batch := firstBatch()
			test.mutate(&batch)

			if _, err := store.CommitBatch(context.Background(), batch); !errors.Is(err, storage.ErrInvalidBatch) {
				t.Fatalf("CommitBatch() error = %v, want ErrInvalidBatch", err)
			}
			assertStoreHasNoCommittedBatch(t, store)
		})
	}
}

func TestCommitBatchRejectsEveryNegativeRollupValue(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*storage.TrafficRollup)
	}{
		{name: "upload", mutate: func(v *storage.TrafficRollup) { v.UploadBytes = -1 }},
		{name: "download", mutate: func(v *storage.TrafficRollup) { v.DownloadBytes = -1 }},
		{name: "recovered upload", mutate: func(v *storage.TrafficRollup) { v.RecoveredUploadBytes = -1 }},
		{name: "recovered download", mutate: func(v *storage.TrafficRollup) { v.RecoveredDownloadBytes = -1 }},
		{name: "speed upload sum", mutate: func(v *storage.TrafficRollup) { v.SpeedUploadSampleSum = -1 }},
		{name: "speed download sum", mutate: func(v *storage.TrafficRollup) { v.SpeedDownloadSampleSum = -1 }},
		{name: "speed samples", mutate: func(v *storage.TrafficRollup) { v.SpeedSampleCount = -1 }},
		{name: "peak upload", mutate: func(v *storage.TrafficRollup) { v.PeakUploadBytesPerSecond = -1 }},
		{name: "peak download", mutate: func(v *storage.TrafficRollup) { v.PeakDownloadBytesPerSecond = -1 }},
		{name: "counter seconds", mutate: func(v *storage.TrafficRollup) { v.CounterObservedSeconds = -1 }},
		{name: "attribution seconds", mutate: func(v *storage.TrafficRollup) { v.AttributionObservedSeconds = -1 }},
		{name: "connection sum", mutate: func(v *storage.TrafficRollup) { v.ActiveConnectionsSum = -1 }},
		{name: "connection samples", mutate: func(v *storage.TrafficRollup) { v.ActiveConnectionsSamples = -1 }},
		{name: "connection max", mutate: func(v *storage.TrafficRollup) { v.ActiveConnectionsMax = -1 }},
		{name: "memory sum", mutate: func(v *storage.TrafficRollup) { v.MemoryBytesSum = -1 }},
		{name: "memory samples", mutate: func(v *storage.TrafficRollup) { v.MemorySamples = -1 }},
		{name: "memory max", mutate: func(v *storage.TrafficRollup) { v.MemoryBytesMax = -1 }},
		{name: "unattributed upload", mutate: func(v *storage.TrafficRollup) { v.UnattributedUploadBytes = -1 }},
		{name: "unattributed download", mutate: func(v *storage.TrafficRollup) { v.UnattributedDownloadBytes = -1 }},
		{name: "reset count", mutate: func(v *storage.TrafficRollup) { v.ResetCount = -1 }},
		{name: "quality flags", mutate: func(v *storage.TrafficRollup) { v.QualityFlags = -1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, _ := migratedTestStore(t)
			batch := firstBatch()
			test.mutate(&batch.Global)
			if _, err := store.CommitBatch(context.Background(), batch); !errors.Is(err, storage.ErrInvalidBatch) {
				t.Fatalf("CommitBatch() error = %v, want ErrInvalidBatch", err)
			}
			assertStoreHasNoCommittedBatch(t, store)
		})
	}
}

func TestCommitBatchAcceptsZeroGlobalBatchWithoutFlows(t *testing.T) {
	store, _ := migratedTestStore(t)
	batch := firstBatch()
	batch.Global.UploadBytes = 0
	batch.Global.DownloadBytes = 0
	batch.Global.SpeedUploadSampleSum = 0
	batch.Global.SpeedDownloadSampleSum = 0
	batch.Global.PeakUploadBytesPerSecond = 0
	batch.Global.PeakDownloadBytesPerSecond = 0
	batch.Global.UnattributedUploadBytes = 0
	batch.Global.UnattributedDownloadBytes = 0
	batch.Flows = nil

	if _, err := store.CommitBatch(context.Background(), batch); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	flows, err := store.FlowRollups(context.Background(), 10, firstBucketAt)
	if err != nil {
		t.Fatalf("FlowRollups() error = %v", err)
	}
	if len(flows) != 0 {
		t.Errorf("len(FlowRollups()) = %d, want 0", len(flows))
	}
}

func TestCommitBatchAcceptsExactly102DistinctFlowRows(t *testing.T) {
	store, _ := migratedTestStore(t)
	batch := firstBatch()
	batch.Global.UnattributedUploadBytes = 0
	batch.Global.UnattributedDownloadBytes = 0
	batch.Flows = make([]storage.FlowRollup, 102)
	for index := range batch.Flows {
		dimension := attributedDimension()
		dimension.Host = fmt.Sprintf("%03d.example.test", index)
		batch.Flows[index].Dimension = dimension
	}
	batch.Flows[0].UploadBytes = batch.Global.UploadBytes
	batch.Flows[0].DownloadBytes = batch.Global.DownloadBytes
	batch.Flows[0].FlowObservationCount = 1

	commitBatch(t, store, batch)
	flows, err := store.FlowRollups(context.Background(), 10, firstBucketAt)
	if err != nil {
		t.Fatalf("FlowRollups() error = %v", err)
	}
	if len(flows) != 102 {
		t.Errorf("len(FlowRollups()) = %d, want 102", len(flows))
	}
}

func TestStorageReadAPIsDistinguishMissingRows(t *testing.T) {
	store, _ := migratedTestStore(t)
	if state, found, err := store.LoadCollectorState(context.Background()); err != nil || found {
		t.Errorf("LoadCollectorState() = %#v, %t, %v", state, found, err)
	}
	if rollup, found, err := store.TrafficRollup(context.Background(), 10, firstBucketAt); err != nil || found {
		t.Errorf("TrafficRollup() = %#v, %t, %v", rollup, found, err)
	}
	if flows, err := store.FlowRollups(context.Background(), 10, firstBucketAt); err != nil || len(flows) != 0 {
		t.Errorf("FlowRollups() = %#v, %v", flows, err)
	}
	if session, found, err := store.RuntimeSession(context.Background(), firstSessionID); err != nil || found {
		t.Errorf("RuntimeSession() = %#v, %t, %v", session, found, err)
	}
	if count, err := store.QualityEventCount(context.Background(), firstBatchID); err != nil || count != 0 {
		t.Errorf("QualityEventCount() = %d, %v", count, err)
	}
}

func TestCommitBatchCommitsNormalNextBatch(t *testing.T) {
	store, _ := migratedTestStore(t)
	commitBatch(t, store, firstBatch())
	batch := nextBatch()

	result, err := store.CommitBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	if result.AlreadyCommitted {
		t.Error("CommitBatch().AlreadyCommitted = true")
	}
	assertCommittedBatchState(t, store, batch)
}

func TestCommitBatchCommitsZeroTrafficBatch(t *testing.T) {
	store, _ := migratedTestStore(t)
	first := firstBatch()
	commitBatch(t, store, first)
	batch := nextBatch()
	batch.NewState.LastTotals = first.NewState.LastTotals
	batch.Global.UploadBytes = 0
	batch.Global.DownloadBytes = 0
	batch.Global.RecoveredUploadBytes = 0
	batch.Global.RecoveredDownloadBytes = 0
	batch.Global.UnattributedUploadBytes = 0
	batch.Global.UnattributedDownloadBytes = 0
	batch.Flows = nil

	commitBatch(t, store, batch)
	assertCommittedBatchState(t, store, batch)
	flows, err := store.FlowRollups(context.Background(), 10, batch.Global.BucketStart)
	if err != nil {
		t.Fatalf("FlowRollups() error = %v", err)
	}
	if len(flows) != 0 {
		t.Errorf("len(FlowRollups()) = %d, want 0", len(flows))
	}
}

func TestCommitBatchExactRetryIsIdempotent(t *testing.T) {
	store, _ := migratedTestStore(t)
	batch := firstBatch()
	commitBatch(t, store, batch)

	result, err := store.CommitBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("retry CommitBatch() error = %v", err)
	}
	if !result.AlreadyCommitted {
		t.Error("retry CommitBatch().AlreadyCommitted = false")
	}
	assertCommittedBatchState(t, store, batch)
	assertQualityEventCount(t, store, batch.BatchID, 1)
}

func TestCommitBatchConcurrentDuplicateIsIdempotent(t *testing.T) {
	store, _ := migratedTestStore(t)
	batch := firstBatch()
	start := make(chan struct{})
	type outcome struct {
		result storage.CommitResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := store.CommitBatch(context.Background(), batch)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(outcomes)

	alreadyCommitted := 0
	newlyCommitted := 0
	for outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("concurrent CommitBatch() error = %v", outcome.err)
		}
		if outcome.result.AlreadyCommitted {
			alreadyCommitted++
		} else {
			newlyCommitted++
		}
	}
	if newlyCommitted != 1 || alreadyCommitted != 1 {
		t.Errorf("concurrent results = new:%d already:%d", newlyCommitted, alreadyCommitted)
	}
	assertCommittedBatchState(t, store, batch)
	assertQualityEventCount(t, store, batch.BatchID, 1)
}

func TestCommitBatchRejectsStateConflictsWithoutMutation(t *testing.T) {
	tests := []struct {
		name      string
		setup     bool
		wantError error
		mutate    func(*storage.Batch)
	}{
		{name: "wrong old totals", setup: true, wantError: storage.ErrStateConflict, mutate: func(batch *storage.Batch) {
			batch.ExpectedOldTotals = &storage.ByteTotals{Upload: 1, Download: 2}
		}},
		{name: "nil expected totals with state", setup: true, wantError: storage.ErrStateConflict, mutate: func(batch *storage.Batch) {
			batch.ExpectedOldTotals = nil
		}},
		{name: "present expected totals without state", wantError: storage.ErrStateConflict, mutate: func(*storage.Batch) {}},
		{name: "timezone mismatch", setup: true, wantError: storage.ErrTimezoneMismatch, mutate: func(batch *storage.Batch) {
			batch.NewState.BucketTimezone = "UTC"
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, _ := migratedTestStore(t)
			var batch storage.Batch
			if test.setup {
				commitBatch(t, store, firstBatch())
				batch = nextBatch()
			} else {
				batch = firstBatch()
				expected := storage.ByteTotals{}
				batch.ExpectedOldTotals = &expected
			}
			test.mutate(&batch)

			if _, err := store.CommitBatch(context.Background(), batch); !errors.Is(err, test.wantError) {
				t.Fatalf("CommitBatch() error = %v, want %v", err, test.wantError)
			}
			if test.setup {
				assertFirstStateAndSecondBatchAbsent(t, store, batch.BatchID)
			} else {
				assertStoreHasNoCommittedBatch(t, store)
			}
		})
	}
}

func TestCommitBatchReplacesExistingBucketWithCompleteValues(t *testing.T) {
	store, _ := migratedTestStore(t)
	first := firstBatch()
	commitBatch(t, store, first)
	replacement := nextBatch()
	replacement.Global.BucketStart = first.Global.BucketStart
	replacement.Global.BucketEnd = first.Global.BucketEnd
	replacement.NewState.LastSampleAt = first.NewState.LastSampleAt
	replacement.Global.UploadBytes = 200
	replacement.Global.DownloadBytes = 600
	replacement.Global.UnattributedUploadBytes = 0
	replacement.Global.UnattributedDownloadBytes = 0
	replacement.Flows = []storage.FlowRollup{{
		Dimension:            attributedDimension(),
		UploadBytes:          200,
		DownloadBytes:        600,
		FlowObservationCount: 2,
	}}

	commitBatch(t, store, replacement)
	global, found, err := store.TrafficRollup(context.Background(), 10, firstBucketAt)
	if err != nil || !found {
		t.Fatalf("TrafficRollup() = %#v, %t, %v", global, found, err)
	}
	if !reflect.DeepEqual(global, replacement.Global) {
		t.Errorf("replacement global = %#v, want %#v", global, replacement.Global)
	}
	flows, err := store.FlowRollups(context.Background(), 10, firstBucketAt)
	if err != nil {
		t.Fatalf("FlowRollups() error = %v", err)
	}
	if len(flows) != 1 || flows[0].UploadBytes != 200 || flows[0].DownloadBytes != 600 {
		t.Errorf("replacement flows = %#v", flows)
	}
}

func TestCommitBatchTransitionsRuntimeSessionsAtomically(t *testing.T) {
	store, _ := migratedTestStore(t)
	first := firstBatch()
	commitBatch(t, store, first)
	batch := nextBatch()
	batch.NewState.RuntimeSessionID = secondSessionID
	batch.NewState.LastTotals = storage.ByteTotals{Upload: 50, Download: 200}
	batch.Global.UploadBytes = 50
	batch.Global.DownloadBytes = 200
	batch.Global.UnattributedUploadBytes = 0
	batch.Global.UnattributedDownloadBytes = 0
	batch.Flows = []storage.FlowRollup{{
		Dimension:            attributedDimension(),
		UploadBytes:          50,
		DownloadBytes:        200,
		FlowObservationCount: 1,
	}}
	batch.EndRuntimeSession = &storage.RuntimeSessionEnd{
		ID:        firstSessionID,
		EndedAt:   batch.Global.BucketStart,
		EndReason: "counter_reset",
	}
	batch.NewRuntimeSession = &storage.RuntimeSessionStart{
		ID:                   secondSessionID,
		StartedAt:            batch.Global.BucketStart,
		StartReason:          "counter_reset",
		SingBoxVersion:       "sing-box 1.12.0-fixture",
		DataGapBeforeSeconds: 0,
	}

	commitBatch(t, store, batch)
	oldSession, found, err := store.RuntimeSession(context.Background(), firstSessionID)
	if err != nil || !found {
		t.Fatalf("old RuntimeSession() = %#v, %t, %v", oldSession, found, err)
	}
	if oldSession.EndedAt == nil || *oldSession.EndedAt != batch.EndRuntimeSession.EndedAt ||
		oldSession.EndReason == nil || *oldSession.EndReason != batch.EndRuntimeSession.EndReason {
		t.Errorf("old RuntimeSession() = %#v", oldSession)
	}
	newSession, found, err := store.RuntimeSession(context.Background(), secondSessionID)
	if err != nil || !found {
		t.Fatalf("new RuntimeSession() = %#v, %t, %v", newSession, found, err)
	}
	if newSession.EndedAt != nil || newSession.LastTotals != batch.NewState.LastTotals ||
		newSession.LastSeenAt != batch.NewState.LastSampleAt {
		t.Errorf("new RuntimeSession() = %#v", newSession)
	}
	assertCommittedBatchState(t, store, batch)
}

func TestCommitBatchRollsBackInvalidDurableReferences(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*storage.Batch)
	}{
		{name: "missing current session", mutate: func(batch *storage.Batch) {
			batch.NewState.RuntimeSessionID = "missing-runtime-session"
		}},
		{name: "nonexistent session end", mutate: func(batch *storage.Batch) {
			batch.EndRuntimeSession = &storage.RuntimeSessionEnd{
				ID: "missing-runtime-session", EndedAt: batch.Global.BucketStart, EndReason: "fixture_end",
			}
		}},
		{name: "duplicate session id", mutate: func(batch *storage.Batch) {
			batch.NewRuntimeSession = &storage.RuntimeSessionStart{
				ID: firstSessionID, StartedAt: batch.Global.BucketStart, StartReason: "duplicate",
				SingBoxVersion: "sing-box 1.12.0-fixture",
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, _ := migratedTestStore(t)
			commitBatch(t, store, firstBatch())
			batch := nextBatch()
			test.mutate(&batch)

			_, err := store.CommitBatch(context.Background(), batch)
			if err == nil {
				t.Fatal("CommitBatch() error = nil")
			}
			for _, forbidden := range []string{batch.BatchID, batch.NewState.RuntimeSessionID, "fixture-only detail"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Errorf("CommitBatch() error contains row content %q: %v", forbidden, err)
				}
			}
			assertFirstStateAndSecondBatchAbsent(t, store, batch.BatchID)
		})
	}
}

func TestCommitBatchRollsBackEveryTransactionStageAndRetriesExactlyOnce(t *testing.T) {
	tests := []struct {
		table     string
		operation string
	}{
		{table: "traffic_rollup", operation: "INSERT"},
		{table: "flow_rollup", operation: "INSERT"},
		{table: "quality_event", operation: "INSERT"},
		{table: "runtime_session", operation: "UPDATE"},
		{table: "collector_state", operation: "UPDATE"},
	}

	for _, test := range tests {
		t.Run(test.table, func(t *testing.T) {
			store, databasePath := migratedTestStore(t)
			commitBatch(t, store, firstBatch())
			database := openRawDatabase(t, databasePath)
			dropTrigger := installAbortTrigger(t, database, test.table, test.operation)
			batch := nextBatch()

			_, err := store.CommitBatch(context.Background(), batch)
			if err == nil {
				t.Fatal("CommitBatch() error = nil with abort trigger")
			}
			if strings.Contains(err.Error(), "fixture-only abort") {
				t.Errorf("CommitBatch() leaked trigger content: %v", err)
			}
			assertFirstStateAndSecondBatchAbsent(t, store, batch.BatchID)

			dropTrigger()
			result, err := store.CommitBatch(context.Background(), batch)
			if err != nil {
				t.Fatalf("retry CommitBatch() error = %v", err)
			}
			if result.AlreadyCommitted {
				t.Error("first successful retry AlreadyCommitted = true")
			}
			assertCommittedBatchState(t, store, batch)
			assertQualityEventCount(t, store, batch.BatchID, 1)

			result, err = store.CommitBatch(context.Background(), batch)
			if err != nil {
				t.Fatalf("idempotent retry CommitBatch() error = %v", err)
			}
			if !result.AlreadyCommitted {
				t.Error("idempotent retry AlreadyCommitted = false")
			}
			assertQualityEventCount(t, store, batch.BatchID, 1)
		})
	}
}

func commitBatch(t *testing.T, store *storage.Store, batch storage.Batch) {
	t.Helper()
	if _, err := store.CommitBatch(context.Background(), batch); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
}

func assertCommittedBatchState(t *testing.T, store *storage.Store, batch storage.Batch) {
	t.Helper()
	state, found, err := store.LoadCollectorState(context.Background())
	if err != nil || !found {
		t.Fatalf("LoadCollectorState() = %#v, %t, %v", state, found, err)
	}
	want := storage.CollectorState{CollectorCursor: batch.NewState, LastBatchID: batch.BatchID}
	if state != want {
		t.Errorf("LoadCollectorState() = %#v, want %#v", state, want)
	}
	global, found, err := store.TrafficRollup(context.Background(), batch.Global.ResolutionSec, batch.Global.BucketStart)
	if err != nil || !found || !reflect.DeepEqual(global, batch.Global) {
		t.Errorf("TrafficRollup() = %#v, %t, %v; want %#v", global, found, err, batch.Global)
	}
	session, found, err := store.RuntimeSession(context.Background(), batch.NewState.RuntimeSessionID)
	if err != nil || !found {
		t.Fatalf("RuntimeSession() = %#v, %t, %v", session, found, err)
	}
	if session.LastTotals != batch.NewState.LastTotals || session.LastSeenAt != batch.NewState.LastSampleAt {
		t.Errorf("RuntimeSession() = %#v", session)
	}
}

func assertQualityEventCount(t *testing.T, store *storage.Store, batchID string, want int64) {
	t.Helper()
	count, err := store.QualityEventCount(context.Background(), batchID)
	if err != nil {
		t.Fatalf("QualityEventCount() error = %v", err)
	}
	if count != want {
		t.Errorf("QualityEventCount() = %d, want %d", count, want)
	}
}

func assertFirstStateAndSecondBatchAbsent(t *testing.T, store *storage.Store, failedBatchID string) {
	t.Helper()
	first := firstBatch()
	state, found, err := store.LoadCollectorState(context.Background())
	if err != nil || !found {
		t.Fatalf("LoadCollectorState() = %#v, %t, %v", state, found, err)
	}
	wantState := storage.CollectorState{CollectorCursor: first.NewState, LastBatchID: first.BatchID}
	if state != wantState {
		t.Errorf("LoadCollectorState() = %#v, want %#v", state, wantState)
	}
	if rollup, found, err := store.TrafficRollup(context.Background(), 10, firstBucketAt+10); err != nil || found {
		t.Errorf("failed TrafficRollup() = %#v, %t, %v", rollup, found, err)
	}
	if flows, err := store.FlowRollups(context.Background(), 10, firstBucketAt+10); err != nil || len(flows) != 0 {
		t.Errorf("failed FlowRollups() = %#v, %v", flows, err)
	}
	assertQualityEventCount(t, store, failedBatchID, 0)
	session, found, err := store.RuntimeSession(context.Background(), firstSessionID)
	if err != nil || !found {
		t.Fatalf("RuntimeSession() = %#v, %t, %v", session, found, err)
	}
	if session.EndedAt != nil || session.LastTotals != first.NewState.LastTotals || session.LastSeenAt != first.NewState.LastSampleAt {
		t.Errorf("RuntimeSession() changed after rollback: %#v", session)
	}
}

func assertStoreHasNoCommittedBatch(t *testing.T, store *storage.Store) {
	t.Helper()
	if state, found, err := store.LoadCollectorState(context.Background()); err != nil || found {
		t.Fatalf("LoadCollectorState() after rejection = %#v, %t, %v", state, found, err)
	}
	if rollup, found, err := store.TrafficRollup(context.Background(), 10, firstBucketAt); err != nil || found {
		t.Fatalf("TrafficRollup() after rejection = %#v, %t, %v", rollup, found, err)
	}
}

func migratedTestStore(t *testing.T) (*storage.Store, string) {
	t.Helper()
	store, databasePath := openTestStore(t)
	if _, err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return store, databasePath
}

func firstBatch() storage.Batch {
	return storage.Batch{
		BatchID: firstBatchID,
		NewState: storage.CollectorCursor{
			RuntimeSessionID: firstSessionID,
			LastTotals:       storage.ByteTotals{Upload: 1100, Download: 4400},
			LastSampleAt:     firstBucketAt + 9,
			BucketTimezone:   "Asia/Shanghai",
		},
		Global: storage.TrafficRollup{
			ResolutionSec:              10,
			BucketStart:                firstBucketAt,
			BucketEnd:                  firstBucketAt + 10,
			UploadBytes:                100,
			DownloadBytes:              400,
			SpeedUploadSampleSum:       100,
			SpeedDownloadSampleSum:     400,
			SpeedSampleCount:           1,
			PeakUploadBytesPerSecond:   100,
			PeakDownloadBytesPerSecond: 400,
			CounterObservedSeconds:     10,
			AttributionObservedSeconds: 10,
			ActiveConnectionsSum:       2,
			ActiveConnectionsSamples:   1,
			ActiveConnectionsMax:       2,
			MemoryBytesSum:             67108864,
			MemorySamples:              1,
			MemoryBytesMax:             67108864,
			UnattributedUploadBytes:    30,
			UnattributedDownloadBytes:  120,
		},
		Flows: []storage.FlowRollup{
			{
				Dimension:            attributedDimension(),
				UploadBytes:          60,
				DownloadBytes:        240,
				FlowObservationCount: 1,
			},
			{
				Dimension:            specialDimension(2),
				UploadBytes:          10,
				DownloadBytes:        40,
				FlowObservationCount: 1,
			},
			{
				Dimension:            specialDimension(3),
				UploadBytes:          30,
				DownloadBytes:        120,
				FlowObservationCount: 1,
			},
		},
		NewRuntimeSession: &storage.RuntimeSessionStart{
			ID:                   firstSessionID,
			StartedAt:            firstBucketAt,
			StartReason:          "startup",
			SingBoxVersion:       "sing-box 1.12.0-fixture",
			DataGapBeforeSeconds: 0,
		},
		QualityEvents: []storage.QualityEvent{{
			Code:      "fixture_gap",
			StartedAt: firstBucketAt,
			Flags:     1,
			Detail:    "fixture-only detail",
		}},
	}
}

func nextBatch() storage.Batch {
	batch := firstBatch()
	batch.BatchID = secondBatchID
	expected := batch.NewState.LastTotals
	batch.ExpectedOldTotals = &expected
	batch.NewState.LastTotals = storage.ByteTotals{Upload: 1200, Download: 4800}
	batch.NewState.LastSampleAt += 10
	batch.Global.BucketStart += 10
	batch.Global.BucketEnd += 10
	batch.NewRuntimeSession = nil
	batch.EndRuntimeSession = nil
	for index := range batch.QualityEvents {
		batch.QualityEvents[index].StartedAt += 10
		if batch.QualityEvents[index].EndedAt != nil {
			value := *batch.QualityEvents[index].EndedAt + 10
			batch.QualityEvents[index].EndedAt = &value
		}
	}
	return batch
}

func installAbortTrigger(t *testing.T, database *sql.DB, table, operation string) func() {
	t.Helper()
	name := "fixture_abort_" + table
	statement := fmt.Sprintf(`
		CREATE TRIGGER %s BEFORE %s ON %s
		BEGIN
			SELECT RAISE(ABORT, 'fixture-only abort');
		END
	`, name, operation, table)
	if _, err := database.Exec(statement); err != nil {
		t.Fatalf("install trigger on %s: %v", table, err)
	}
	drop := func() {
		if _, err := database.Exec("DROP TRIGGER IF EXISTS " + name); err != nil {
			t.Errorf("drop trigger on %s: %v", table, err)
		}
	}
	t.Cleanup(drop)
	return drop
}

func attributedDimension() storage.FlowDimension {
	source := netip.MustParseAddr("192.0.2.10").As4()
	destination := netip.MustParseAddr("198.51.100.20").As4()
	return storage.FlowDimension{
		SourceFamily:       4,
		SourceNetwork:      append([]byte(nil), source[:]...),
		SourcePrefixLen:    24,
		DestinationFamily:  4,
		DestinationIP:      append([]byte(nil), destination[:]...),
		DestinationPort:    443,
		Host:               "api.example.test",
		NetworkCode:        1,
		ClassificationCode: 1,
	}
}

func specialDimension(classification int64) storage.FlowDimension {
	return storage.FlowDimension{
		SourceNetwork:      []byte{},
		DestinationIP:      []byte{},
		DestinationPort:    -1,
		ClassificationCode: classification,
	}
}

func assertFlowClasses(t *testing.T, flows []storage.FlowRollup, want map[int64]storage.ByteTotals) {
	t.Helper()
	if len(flows) != len(want) {
		t.Fatalf("len(FlowRollups()) = %d, want %d", len(flows), len(want))
	}
	for _, flow := range flows {
		classification := flow.Dimension.ClassificationCode
		got := storage.ByteTotals{Upload: flow.UploadBytes, Download: flow.DownloadBytes}
		if got != want[classification] {
			t.Errorf("classification %d totals = %#v, want %#v", classification, got, want[classification])
		}
	}
}
