package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestRollupTrafficRecomputesMinuteFromCompleteTenSecondValues(t *testing.T) {
	store, _ := migratedTestStore(t)
	seedTenSecondRollups(t, store)
	window, err := rollup.WindowAt(
		time.Unix(firstBucketAt+15, 0), rollup.ResolutionMinute, time.UTC,
	)
	if err != nil {
		t.Fatalf("WindowAt() error = %v", err)
	}

	got, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, window)
	if err != nil {
		t.Fatalf("RollupTraffic() error = %v", err)
	}
	wantPeakUploadAt := firstBucketAt + 51
	wantPeakDownloadAt := firstBucketAt + 2
	want := storage.TrafficRollup{
		ResolutionSec:              rollup.ResolutionMinute,
		BucketStart:                firstBucketAt,
		BucketEnd:                  firstBucketAt + 60,
		UploadBytes:                615,
		DownloadBytes:              1215,
		RecoveredUploadBytes:       15,
		RecoveredDownloadBytes:     21,
		SpeedUploadSampleSum:       6015,
		SpeedDownloadSampleSum:     12015,
		SpeedSampleCount:           6,
		PeakUploadBytesPerSecond:   15,
		PeakDownloadBytesPerSecond: 20,
		PeakUploadAt:               &wantPeakUploadAt,
		PeakDownloadAt:             &wantPeakDownloadAt,
		CounterObservedSeconds:     60,
		AttributionObservedSeconds: 54,
		ActiveConnectionsSum:       27,
		ActiveConnectionsSamples:   6,
		ActiveConnectionsMax:       7,
		MemoryBytesSum:             6015,
		MemorySamples:              6,
		MemoryBytesMax:             1005,
		UnattributedUploadBytes:    615,
		UnattributedDownloadBytes:  1215,
		ResetCount:                 3,
		QualityFlags:               63,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RollupTraffic() = %#v, want %#v", got, want)
	}

	second, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, window)
	if err != nil {
		t.Fatalf("second RollupTraffic() error = %v", err)
	}
	if !reflect.DeepEqual(second, want) {
		t.Fatalf("second RollupTraffic() = %#v, want %#v", second, want)
	}
	persisted, found, err := store.TrafficRollup(context.Background(), rollup.ResolutionMinute, firstBucketAt)
	if err != nil || !found || !reflect.DeepEqual(persisted, want) {
		t.Fatalf("TrafficRollup() = %#v, %t, %v; want %#v", persisted, found, err, want)
	}
}

func TestRollupTrafficRecomputesMultidimensionalTopK(t *testing.T) {
	store, _ := migratedTestStore(t)
	dimensionA := storage.FlowDimension{DestinationFamily: 4, DestinationIP: []byte{198, 51, 100, 1}, DestinationPort: 443, NetworkCode: 1, ClassificationCode: 1}
	dimensionB := storage.FlowDimension{DestinationFamily: 4, DestinationIP: []byte{198, 51, 100, 2}, DestinationPort: 80, NetworkCode: 1, ClassificationCode: 1}
	sessionID := "stage3-rollup-session"
	previous := storage.ByteTotals{Upload: 100, Download: 100}
	for index, flows := range [][]storage.FlowRollup{
		{
			{Dimension: dimensionA, UploadBytes: 6, DownloadBytes: 2, FlowObservationCount: 1},
			{Dimension: dimensionB, UploadBytes: 4, DownloadBytes: 8, FlowObservationCount: 1},
			{Dimension: specialDimension(2)}, {Dimension: specialDimension(3)},
		},
		{
			{Dimension: dimensionA, UploadBytes: 1, DownloadBytes: 2, FlowObservationCount: 1},
			{Dimension: dimensionB, UploadBytes: 9, DownloadBytes: 8, FlowObservationCount: 1},
			{Dimension: specialDimension(2)}, {Dimension: specialDimension(3)},
		},
	} {
		start := firstBucketAt + int64(index)*10
		current := storage.ByteTotals{Upload: previous.Upload + 10, Download: previous.Download + 10}
		batch := storage.Batch{
			BatchID:  fmt.Sprintf("stage3-rollup-%d", index),
			NewState: storage.CollectorCursor{RuntimeSessionID: sessionID, LastTotals: current, LastSampleAt: start + 9, BucketTimezone: "UTC"},
			Global: storage.TrafficRollup{
				ResolutionSec: 10, BucketStart: start, BucketEnd: start + 10,
				UploadBytes: 10, DownloadBytes: 10, CounterObservedSeconds: 1, AttributionObservedSeconds: 1,
			},
			Flows: flows,
		}
		if index == 0 {
			batch.NewRuntimeSession = &storage.RuntimeSessionStart{ID: sessionID, StartedAt: start, StartReason: "startup", SingBoxVersion: "fixture"}
		} else {
			expected := previous
			batch.ExpectedOldTotals = &expected
		}
		commitBatch(t, store, batch)
		previous = current
	}
	window := storageWindow(t, firstBucketAt, rollup.ResolutionMinute, time.UTC)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, window, 1); err != nil {
		t.Fatalf("RollupTraffic() error = %v", err)
	}
	flows, err := store.FlowRollups(context.Background(), rollup.ResolutionMinute, firstBucketAt)
	if err != nil {
		t.Fatalf("FlowRollups() error = %v", err)
	}
	if len(flows) != 3 || !reflect.DeepEqual(flows[0].Dimension, dimensionB) ||
		flows[0].UploadBytes != 13 || flows[0].DownloadBytes != 16 ||
		flows[1].Dimension.ClassificationCode != 2 || flows[1].UploadBytes != 7 || flows[1].DownloadBytes != 4 ||
		flows[2].Dimension.ClassificationCode != 3 || flows[2].UploadBytes != 0 || flows[2].DownloadBytes != 0 {
		t.Fatalf("minute flows = %#v", flows)
	}
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, window, 1); err != nil {
		t.Fatalf("second RollupTraffic() error = %v", err)
	}
	second, err := store.FlowRollups(context.Background(), rollup.ResolutionMinute, firstBucketAt)
	if err != nil || !reflect.DeepEqual(second, flows) {
		t.Fatalf("second flows = %#v, %v", second, err)
	}
}

func storageWindow(t *testing.T, at, resolution int64, location *time.Location) rollup.Window {
	t.Helper()
	window, err := rollup.WindowAt(time.Unix(at, 0), resolution, location)
	if err != nil {
		t.Fatalf("WindowAt() error = %v", err)
	}
	return window
}

func TestRollupTrafficAcceptsOnlyPlannedResolutionEdges(t *testing.T) {
	store, _ := migratedTestStore(t)
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	tests := []struct {
		name   string
		source int64
		target int64
	}{
		{name: "ten seconds to minute", source: rollup.ResolutionTenSeconds, target: rollup.ResolutionMinute},
		{name: "minute to half hour", source: rollup.ResolutionMinute, target: rollup.ResolutionHalfHour},
		{name: "half hour to hour", source: rollup.ResolutionHalfHour, target: rollup.ResolutionHour},
		{name: "minute to natural day", source: rollup.ResolutionMinute, target: rollup.ResolutionDay},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			window, windowErr := rollup.WindowAt(time.Unix(firstBucketAt, 0), test.target, location)
			if windowErr != nil {
				t.Fatalf("WindowAt() error = %v", windowErr)
			}
			_, rollupErr := store.RollupTraffic(context.Background(), test.source, window)
			if !errors.Is(rollupErr, storage.ErrNoSourceRollups) {
				t.Fatalf("RollupTraffic() error = %v, want ErrNoSourceRollups", rollupErr)
			}
		})
	}

	minute, err := rollup.WindowAt(time.Unix(firstBucketAt, 0), rollup.ResolutionMinute, time.UTC)
	if err != nil {
		t.Fatalf("WindowAt() error = %v", err)
	}
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionMinute, minute); !errors.Is(err, storage.ErrInvalidRollup) {
		t.Fatalf("same-resolution RollupTraffic() error = %v, want ErrInvalidRollup", err)
	}
	minute.BucketEnd++
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, minute); !errors.Is(err, storage.ErrInvalidRollup) {
		t.Fatalf("misaligned RollupTraffic() error = %v, want ErrInvalidRollup", err)
	}
}

func TestRollupTrafficPreservesDimensionsAcrossEveryPlannedEdge(t *testing.T) {
	store, _ := migratedTestStore(t)
	commitBatch(t, store, firstBatch())

	minute := storageWindow(t, firstBucketAt, rollup.ResolutionMinute, time.UTC)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, minute, 20); err != nil {
		t.Fatalf("minute RollupTraffic() error = %v", err)
	}
	halfHour := storageWindow(t, firstBucketAt, rollup.ResolutionHalfHour, time.UTC)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionMinute, halfHour, 20); err != nil {
		t.Fatalf("half-hour RollupTraffic() error = %v", err)
	}
	hour := storageWindow(t, firstBucketAt, rollup.ResolutionHour, time.UTC)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionHalfHour, hour, 20); err != nil {
		t.Fatalf("hour RollupTraffic() error = %v", err)
	}
	day := storageWindow(t, firstBucketAt, rollup.ResolutionDay, time.UTC)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionMinute, day, 20); err != nil {
		t.Fatalf("day RollupTraffic() error = %v", err)
	}

	for _, target := range []rollup.Window{minute, halfHour, hour, day} {
		global, found, err := store.TrafficRollup(context.Background(), target.ResolutionSec, target.BucketStart)
		if err != nil || !found || global.UploadBytes != 100 || global.DownloadBytes != 400 ||
			global.UnattributedUploadBytes != 30 || global.UnattributedDownloadBytes != 120 {
			t.Fatalf("target global %d = %#v, %t, %v", target.ResolutionSec, global, found, err)
		}
		flows, err := store.FlowRollups(context.Background(), target.ResolutionSec, target.BucketStart)
		if err != nil {
			t.Fatalf("target flows %d error = %v", target.ResolutionSec, err)
		}
		assertFlowClasses(t, flows, map[int64]storage.ByteTotals{
			1: {Upload: 60, Download: 240},
			2: {Upload: 10, Download: 40},
			3: {Upload: 30, Download: 120},
		})
	}
}

func TestRollupTrafficHandlesNaturalDayTimezoneBoundaries(t *testing.T) {
	tests := []struct {
		name            string
		location        string
		localTime       time.Time
		expectedSeconds int64
	}{
		{name: "UTC", location: "UTC", localTime: time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC), expectedSeconds: 24 * 60 * 60},
		{name: "Kathmandu", location: "Asia/Kathmandu", localTime: time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC), expectedSeconds: 24 * 60 * 60},
		{name: "New York spring", location: "America/New_York", localTime: time.Date(2026, time.March, 8, 12, 0, 0, 0, time.UTC), expectedSeconds: 23 * 60 * 60},
		{name: "New York fall", location: "America/New_York", localTime: time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC), expectedSeconds: 25 * 60 * 60},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			location, err := time.LoadLocation(test.location)
			if err != nil {
				t.Fatalf("LoadLocation() error = %v", err)
			}
			day, err := rollup.WindowAt(test.localTime.In(location), rollup.ResolutionDay, location)
			if err != nil {
				t.Fatalf("day WindowAt() error = %v", err)
			}
			if duration := day.BucketEnd - day.BucketStart; duration != test.expectedSeconds {
				t.Fatalf("day duration = %d, want %d", duration, test.expectedSeconds)
			}

			store, _ := migratedTestStore(t)
			batch := firstBatch()
			batch.Global.BucketStart = day.BucketStart
			batch.Global.BucketEnd = day.BucketStart + rollup.ResolutionTenSeconds
			batch.NewState.LastSampleAt = batch.Global.BucketEnd - 1
			batch.NewState.BucketTimezone = test.location
			batch.NewRuntimeSession.StartedAt = day.BucketStart
			batch.QualityEvents[0].StartedAt = day.BucketStart
			commitBatch(t, store, batch)

			minute := storageWindow(t, day.BucketStart, rollup.ResolutionMinute, location)
			if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, minute, 20); err != nil {
				t.Fatalf("minute RollupTraffic() error = %v", err)
			}
			target, err := store.RollupTraffic(context.Background(), rollup.ResolutionMinute, day, 20)
			if err != nil || target.BucketStart != day.BucketStart || target.BucketEnd != day.BucketEnd ||
				target.UploadBytes != 100 || target.DownloadBytes != 400 {
				t.Fatalf("day RollupTraffic() = %#v, %v", target, err)
			}
			flows, err := store.FlowRollups(context.Background(), rollup.ResolutionDay, day.BucketStart)
			if err != nil {
				t.Fatalf("day FlowRollups() error = %v", err)
			}
			assertFlowClasses(t, flows, map[int64]storage.ByteTotals{
				1: {Upload: 60, Download: 240},
				2: {Upload: 10, Download: 40},
				3: {Upload: 30, Download: 120},
			})
		})
	}
}

func TestRollupTrafficRollsBackEveryTargetWriteStage(t *testing.T) {
	tests := []struct {
		name      string
		table     string
		operation string
	}{
		{name: "global target write", table: "traffic_rollup", operation: "UPDATE"},
		{name: "flow target delete", table: "flow_rollup", operation: "DELETE"},
		{name: "dimension resolve", table: "flow_dimension", operation: "INSERT"},
		{name: "flow target insert", table: "flow_rollup", operation: "INSERT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, databasePath := migratedTestStore(t)
			commitBatch(t, store, firstBatch())
			minute := storageWindow(t, firstBucketAt, rollup.ResolutionMinute, time.UTC)
			if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, minute, 20); err != nil {
				t.Fatalf("initial RollupTraffic() error = %v", err)
			}
			database := openRawDatabase(t, databasePath)
			advanceRollupSource(t, database, firstBucketAt)
			dropTrigger := installAbortTrigger(t, database, test.table, test.operation)

			if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, minute, 20); err == nil {
				t.Fatal("RollupTraffic() error = nil with abort trigger")
			}
			global, found, err := store.TrafficRollup(context.Background(), rollup.ResolutionMinute, minute.BucketStart)
			if err != nil || !found || global.UploadBytes != 100 || global.DownloadBytes != 400 {
				t.Fatalf("rolled-back global = %#v, %t, %v", global, found, err)
			}
			flows, err := store.FlowRollups(context.Background(), rollup.ResolutionMinute, minute.BucketStart)
			if err != nil {
				t.Fatalf("rolled-back FlowRollups() error = %v", err)
			}
			assertFlowClasses(t, flows, map[int64]storage.ByteTotals{
				1: {Upload: 60, Download: 240},
				2: {Upload: 10, Download: 40},
				3: {Upload: 30, Download: 120},
			})

			dropTrigger()
			updated, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, minute, 20)
			if err != nil || updated.UploadBytes != 101 || updated.DownloadBytes != 401 {
				t.Fatalf("retry RollupTraffic() = %#v, %v", updated, err)
			}
		})
	}
}

func advanceRollupSource(t *testing.T, database *sql.DB, bucketStart int64) {
	t.Helper()
	if _, err := database.Exec(`
		UPDATE traffic_rollup
		SET upload_bytes = upload_bytes + 1, download_bytes = download_bytes + 1
		WHERE resolution_sec = ? AND bucket_start = ?
	`, rollup.ResolutionTenSeconds, bucketStart); err != nil {
		t.Fatalf("advance source global: %v", err)
	}
	if _, err := database.Exec(`
		UPDATE flow_rollup
		SET upload_bytes = upload_bytes + 1, download_bytes = download_bytes + 1
		WHERE resolution_sec = ? AND bucket_start = ? AND dimension_id IN (
			SELECT id FROM flow_dimension WHERE classification_code = 1
		)
	`, rollup.ResolutionTenSeconds, bucketStart); err != nil {
		t.Fatalf("advance source flow: %v", err)
	}
}

func seedTenSecondRollups(t *testing.T, store *storage.Store) {
	seedTenSecondRollupCount(t, store, 6)
}

func seedTenSecondRollupCount(t *testing.T, store *storage.Store, count int64) {
	t.Helper()
	sessionID := "stage2-rollup-session"
	previousTotals := storage.ByteTotals{Upload: 1000, Download: 4000}
	for index := int64(0); index < count; index++ {
		bucketStart := firstBucketAt + index*10
		upload := 100 + index
		download := 200 + index
		newTotals := storage.ByteTotals{
			Upload:   previousTotals.Upload + upload,
			Download: previousTotals.Download + download,
		}
		peakUploadAt := bucketStart + 1
		peakDownloadAt := bucketStart + 2
		batch := storage.Batch{
			BatchID: fmt.Sprintf("stage2-rollup-batch-%d", index),
			NewState: storage.CollectorCursor{
				RuntimeSessionID: sessionID,
				LastTotals:       newTotals,
				LastSampleAt:     bucketStart + 9,
				BucketTimezone:   "Asia/Shanghai",
			},
			Global: storage.TrafficRollup{
				ResolutionSec:              rollup.ResolutionTenSeconds,
				BucketStart:                bucketStart,
				BucketEnd:                  bucketStart + 10,
				UploadBytes:                upload,
				DownloadBytes:              download,
				RecoveredUploadBytes:       index,
				RecoveredDownloadBytes:     index + 1,
				SpeedUploadSampleSum:       1000 + index,
				SpeedDownloadSampleSum:     2000 + index,
				SpeedSampleCount:           1,
				PeakUploadBytesPerSecond:   10 + index,
				PeakDownloadBytesPerSecond: 20 - index,
				PeakUploadAt:               &peakUploadAt,
				PeakDownloadAt:             &peakDownloadAt,
				CounterObservedSeconds:     10,
				AttributionObservedSeconds: 9,
				ActiveConnectionsSum:       2 + index,
				ActiveConnectionsSamples:   1,
				ActiveConnectionsMax:       2 + index,
				MemoryBytesSum:             1000 + index,
				MemorySamples:              1,
				MemoryBytesMax:             1000 + index,
				UnattributedUploadBytes:    upload,
				UnattributedDownloadBytes:  download,
				ResetCount:                 index % 2,
				QualityFlags:               1 << index,
			},
			Flows: []storage.FlowRollup{{
				Dimension:            specialDimension(3),
				UploadBytes:          upload,
				DownloadBytes:        download,
				FlowObservationCount: 1,
			}},
		}
		if index == 0 {
			batch.NewRuntimeSession = &storage.RuntimeSessionStart{
				ID:             sessionID,
				StartedAt:      bucketStart,
				StartReason:    "startup",
				SingBoxVersion: "sing-box 1.12.0-fixture",
			}
		} else {
			expected := previousTotals
			batch.ExpectedOldTotals = &expected
		}
		commitBatch(t, store, batch)
		previousTotals = newTotals
	}
}
