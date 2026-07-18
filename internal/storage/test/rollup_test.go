package storage_test

import (
	"context"
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
