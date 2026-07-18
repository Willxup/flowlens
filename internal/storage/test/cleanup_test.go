package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestCleanupTrafficDeletesOnlyAfterEveryRequiredTargetExists(t *testing.T) {
	store, _ := migratedTestStore(t)
	seedTenSecondRollups(t, store)
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}

	result, err := store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{
		TenSecondBefore: firstBucketAt + 60,
	}, location)
	if err != nil || result != (storage.CleanupResult{}) {
		t.Fatalf("premature CleanupTraffic() = %#v, %v", result, err)
	}
	assertTrafficRollupExists(t, store, rollup.ResolutionTenSeconds, firstBucketAt, true)

	minute := mustWindow(t, firstBucketAt+1, rollup.ResolutionMinute, time.UTC)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, minute); err != nil {
		t.Fatalf("minute RollupTraffic() error = %v", err)
	}
	result, err = store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{
		TenSecondBefore: firstBucketAt + 60,
	}, location)
	if err != nil || result.DeletedTenSecond != 6 {
		t.Fatalf("ten-second CleanupTraffic() = %#v, %v", result, err)
	}
	assertTrafficRollupExists(t, store, rollup.ResolutionTenSeconds, firstBucketAt, false)
	if flows, err := store.FlowRollups(context.Background(), rollup.ResolutionTenSeconds, firstBucketAt); err != nil || len(flows) != 0 {
		t.Fatalf("deleted FlowRollups() = %#v, %v", flows, err)
	}

	result, err = store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{
		MinuteBefore: firstBucketAt + 60,
	}, location)
	if err != nil || result.DeletedMinute != 0 {
		t.Fatalf("minute without targets CleanupTraffic() = %#v, %v", result, err)
	}
	halfHour := mustWindow(t, firstBucketAt+1, rollup.ResolutionHalfHour, time.UTC)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionMinute, halfHour); err != nil {
		t.Fatalf("half-hour RollupTraffic() error = %v", err)
	}
	result, err = store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{
		MinuteBefore: firstBucketAt + 60,
	}, location)
	if err != nil || result.DeletedMinute != 0 {
		t.Fatalf("minute without day CleanupTraffic() = %#v, %v", result, err)
	}
	day := mustWindow(t, firstBucketAt, rollup.ResolutionDay, location)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionMinute, day); err != nil {
		t.Fatalf("day RollupTraffic() error = %v", err)
	}
	result, err = store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{
		MinuteBefore: firstBucketAt + 60,
	}, location)
	if err != nil || result.DeletedMinute != 1 {
		t.Fatalf("minute CleanupTraffic() = %#v, %v", result, err)
	}
	assertTrafficRollupExists(t, store, rollup.ResolutionMinute, firstBucketAt, false)

	result, err = store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{
		HalfHourBefore: halfHour.BucketEnd,
	}, location)
	if err != nil || result.DeletedHalfHour != 0 {
		t.Fatalf("half-hour without hour CleanupTraffic() = %#v, %v", result, err)
	}
	hour := mustWindow(t, firstBucketAt+1, rollup.ResolutionHour, time.UTC)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionHalfHour, hour); err != nil {
		t.Fatalf("hour RollupTraffic() error = %v", err)
	}
	result, err = store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{
		HalfHourBefore: halfHour.BucketEnd,
		HourBefore:     hour.BucketEnd,
	}, location)
	if err != nil || result.DeletedHalfHour != 1 || result.DeletedHour != 1 {
		t.Fatalf("higher CleanupTraffic() = %#v, %v", result, err)
	}
	assertTrafficRollupExists(t, store, rollup.ResolutionHalfHour, halfHour.BucketStart, false)
	assertTrafficRollupExists(t, store, rollup.ResolutionHour, hour.BucketStart, false)
	assertTrafficRollupExists(t, store, rollup.ResolutionDay, day.BucketStart, true)
}

func TestCleanupTrafficRollsBackAllSourceDeletionOnFailure(t *testing.T) {
	store, databasePath := migratedTestStore(t)
	seedTenSecondRollups(t, store)
	minute := mustWindow(t, firstBucketAt+1, rollup.ResolutionMinute, time.UTC)
	if _, err := store.RollupTraffic(context.Background(), rollup.ResolutionTenSeconds, minute); err != nil {
		t.Fatalf("RollupTraffic() error = %v", err)
	}
	database := openRawDatabase(t, databasePath)
	dropTrigger := installAbortTrigger(t, database, "traffic_rollup", "DELETE")

	_, err := store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{
		TenSecondBefore: firstBucketAt + 60,
	}, time.UTC)
	if err == nil {
		t.Fatal("CleanupTraffic() error = nil with abort trigger")
	}
	assertTrafficRollupExists(t, store, rollup.ResolutionTenSeconds, firstBucketAt, true)
	if flows, err := store.FlowRollups(context.Background(), rollup.ResolutionTenSeconds, firstBucketAt); err != nil || len(flows) != 1 {
		t.Fatalf("rollback FlowRollups() = %#v, %v", flows, err)
	}

	dropTrigger()
	result, err := store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{
		TenSecondBefore: firstBucketAt + 60,
	}, time.UTC)
	if err != nil || result.DeletedTenSecond != 6 {
		t.Fatalf("retry CleanupTraffic() = %#v, %v", result, err)
	}
}

func TestCleanupTrafficRejectsInvalidCutoffsAndTimezone(t *testing.T) {
	store, _ := migratedTestStore(t)
	if _, err := store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{
		TenSecondBefore: -1,
	}, time.UTC); !errors.Is(err, storage.ErrInvalidRetention) {
		t.Fatalf("negative CleanupTraffic() error = %v", err)
	}
	if _, err := store.CleanupTraffic(context.Background(), storage.RetentionCutoffs{}, nil); !errors.Is(err, storage.ErrInvalidRetention) {
		t.Fatalf("nil-timezone CleanupTraffic() error = %v", err)
	}
}

func assertTrafficRollupExists(
	t *testing.T,
	store *storage.Store,
	resolution int64,
	bucketStart int64,
	want bool,
) {
	t.Helper()
	_, found, err := store.TrafficRollup(context.Background(), resolution, bucketStart)
	if err != nil || found != want {
		t.Fatalf("TrafficRollup(%d,%d) found = %t, error = %v; want %t", resolution, bucketStart, found, err, want)
	}
}
