package storage_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestNextTrafficRollupWindowReturnsOldestCompleteMissingWindow(t *testing.T) {
	store, _ := migratedTestStore(t)
	seedTenSecondRollupCount(t, store, 13)

	if window, found, err := store.NextTrafficRollupWindow(
		context.Background(), rollup.ResolutionTenSeconds, rollup.ResolutionMinute,
		firstBucketAt+59, time.UTC,
	); err != nil || found {
		t.Fatalf("incomplete NextTrafficRollupWindow() = %#v, %t, %v", window, found, err)
	}

	first := mustWindow(t, firstBucketAt+1, rollup.ResolutionMinute, time.UTC)
	window, found, err := store.NextTrafficRollupWindow(
		context.Background(), rollup.ResolutionTenSeconds, rollup.ResolutionMinute,
		firstBucketAt+120, time.UTC,
	)
	if err != nil || !found || window != first {
		t.Fatalf("first NextTrafficRollupWindow() = %#v, %t, %v; want %#v", window, found, err, first)
	}
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, first); err != nil {
		t.Fatalf("first RollupTraffic() error = %v", err)
	}

	second := mustWindow(t, firstBucketAt+61, rollup.ResolutionMinute, time.UTC)
	window, found, err = store.NextTrafficRollupWindow(
		context.Background(), rollup.ResolutionTenSeconds, rollup.ResolutionMinute,
		firstBucketAt+120, time.UTC,
	)
	if err != nil || !found || window != second {
		t.Fatalf("second NextTrafficRollupWindow() = %#v, %t, %v; want %#v", window, found, err, second)
	}
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, second); err != nil {
		t.Fatalf("second RollupTraffic() error = %v", err)
	}
	if window, found, err = store.NextTrafficRollupWindow(
		context.Background(), rollup.ResolutionTenSeconds, rollup.ResolutionMinute,
		firstBucketAt+120, time.UTC,
	); err != nil || found {
		t.Fatalf("complete NextTrafficRollupWindow() = %#v, %t, %v", window, found, err)
	}
}

func TestNextTrafficRollupWindowWaitsForDurableCollectorWatermark(t *testing.T) {
	store, _ := migratedTestStore(t)
	seedTenSecondRollupCount(t, store, 6)

	window, found, err := store.NextTrafficRollupWindow(
		context.Background(), rollup.ResolutionTenSeconds, rollup.ResolutionMinute,
		firstBucketAt+60, time.UTC,
	)
	if err != nil || found || window != (rollup.Window{}) {
		t.Fatalf("unstable NextTrafficRollupWindow() = %#v, %t, %v", window, found, err)
	}

	stableStore, _ := migratedTestStore(t)
	seedTenSecondRollupCount(t, stableStore, 7)
	want := mustWindow(t, firstBucketAt+1, rollup.ResolutionMinute, time.UTC)
	window, found, err = stableStore.NextTrafficRollupWindow(
		context.Background(), rollup.ResolutionTenSeconds, rollup.ResolutionMinute,
		firstBucketAt+60, time.UTC,
	)
	if err != nil || !found || window != want {
		t.Fatalf("stable NextTrafficRollupWindow() = %#v, %t, %v; want %#v", window, found, err, want)
	}
}

func TestNextTrafficRollupWindowUsesConfiguredNaturalDay(t *testing.T) {
	store, databasePath := migratedTestStore(t)
	seedTenSecondRollups(t, store)
	minute := mustWindow(t, firstBucketAt+1, rollup.ResolutionMinute, time.UTC)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, minute); err != nil {
		t.Fatalf("minute RollupTraffic() error = %v", err)
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	want := mustWindow(t, firstBucketAt, rollup.ResolutionDay, location)
	database := openRawDatabase(t, databasePath)
	if _, err := database.Exec(`UPDATE collector_state SET last_sample_at = ? WHERE id = 1`, want.BucketEnd); err != nil {
		t.Fatalf("advance collector watermark: %v", err)
	}

	window, found, err := store.NextTrafficRollupWindow(
		context.Background(), rollup.ResolutionMinute, rollup.ResolutionDay,
		want.BucketEnd, location,
	)
	if err != nil || !found || window != want {
		t.Fatalf("NextTrafficRollupWindow() = %#v, %t, %v; want %#v", window, found, err, want)
	}
}

func TestNextTrafficRollupWindowRejectsInvalidEdgeAndHonorsCancellation(t *testing.T) {
	store, _ := migratedTestStore(t)
	window, found, err := store.NextTrafficRollupWindow(
		context.Background(), rollup.ResolutionMinute, rollup.ResolutionMinute,
		firstBucketAt+60, time.UTC,
	)
	if !errors.Is(err, storage.ErrInvalidRollup) || found || window != (rollup.Window{}) {
		t.Fatalf("invalid NextTrafficRollupWindow() = %#v, %t, %v", window, found, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	window, found, err = store.NextTrafficRollupWindow(
		ctx, rollup.ResolutionTenSeconds, rollup.ResolutionMinute,
		firstBucketAt+60, time.UTC,
	)
	if !errors.Is(err, context.Canceled) || found || !reflect.DeepEqual(window, rollup.Window{}) {
		t.Fatalf("canceled NextTrafficRollupWindow() = %#v, %t, %v", window, found, err)
	}
}

func mustWindow(t *testing.T, second, resolution int64, location *time.Location) rollup.Window {
	t.Helper()
	window, err := rollup.WindowAt(time.Unix(second, 0), resolution, location)
	if err != nil {
		t.Fatalf("WindowAt() error = %v", err)
	}
	return window
}
