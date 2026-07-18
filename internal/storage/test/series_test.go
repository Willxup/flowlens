package storage_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestTrafficSeriesReadsNonOverlappingSegmentsChronologically(t *testing.T) {
	store, _ := migratedTestStore(t)
	seedTenSecondRollupCount(t, store, 12)
	minute := mustWindow(t, firstBucketAt+1, rollup.ResolutionMinute, nil)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, minute); err != nil {
		t.Fatalf("RollupTraffic() error = %v", err)
	}
	segments := []rollup.Segment{
		{ResolutionSec: rollup.ResolutionMinute, From: firstBucketAt, To: firstBucketAt + 60},
		{ResolutionSec: rollup.ResolutionTenSeconds, From: firstBucketAt + 60, To: firstBucketAt + 120},
	}

	rows, err := store.TrafficSeries(context.Background(), segments)
	if err != nil {
		t.Fatalf("TrafficSeries() error = %v", err)
	}
	if len(rows) != 7 || rows[0].ResolutionSec != rollup.ResolutionMinute ||
		rows[0].BucketStart != firstBucketAt || rows[1].BucketStart != firstBucketAt+60 {
		t.Fatalf("TrafficSeries() = %#v", rows)
	}
	for index := 1; index < len(rows); index++ {
		if rows[index-1].BucketEnd > rows[index].BucketStart {
			t.Fatalf("TrafficSeries() overlaps at %d: %#v", index, rows)
		}
	}
}

func TestTrafficSeriesPreservesLargeIntegersAndRejectsOverlap(t *testing.T) {
	store, _ := migratedTestStore(t)
	batch := firstBatch()
	largeUpload := int64(1<<54 + 12345)
	largeDownload := int64(1<<55 + 67890)
	batch.NewState.LastTotals = storage.ByteTotals{Upload: largeUpload, Download: largeDownload}
	batch.Global.UploadBytes = largeUpload
	batch.Global.DownloadBytes = largeDownload
	batch.Global.UnattributedUploadBytes = largeUpload
	batch.Global.UnattributedDownloadBytes = largeDownload
	batch.Flows = []storage.FlowRollup{{
		Dimension:            specialDimension(3),
		UploadBytes:          largeUpload,
		DownloadBytes:        largeDownload,
		FlowObservationCount: 1,
	}}
	commitBatch(t, store, batch)

	rows, err := store.TrafficSeries(context.Background(), []rollup.Segment{{
		ResolutionSec: rollup.ResolutionTenSeconds,
		From:          firstBucketAt,
		To:            firstBucketAt + 10,
	}})
	if err != nil || len(rows) != 1 || rows[0].UploadBytes != largeUpload || rows[0].DownloadBytes != largeDownload {
		t.Fatalf("large TrafficSeries() = %#v, %v", rows, err)
	}
	if largeUpload <= math.MaxInt64/1024 {
		t.Fatal("fixture is not large")
	}

	_, err = store.TrafficSeries(context.Background(), []rollup.Segment{
		{ResolutionSec: rollup.ResolutionMinute, From: firstBucketAt, To: firstBucketAt + 60},
		{ResolutionSec: rollup.ResolutionTenSeconds, From: firstBucketAt + 50, To: firstBucketAt + 70},
	})
	if !errors.Is(err, storage.ErrInvalidQuery) {
		t.Fatalf("overlapping TrafficSeries() error = %v", err)
	}
}
