package storage_test

import (
	"context"
	"testing"

	"github.com/Willxup/flowlens/internal/storage"
)

func TestQualityEventsReturnsOnlyPublicRangeFields(t *testing.T) {
	store, _ := migratedTestStore(t)
	commitBatch(t, store, firstBatch())

	events, err := store.QualityEvents(context.Background(), firstBucketAt, firstBucketAt+10)
	if err != nil {
		t.Fatalf("QualityEvents() error = %v", err)
	}
	if len(events) != 1 || events[0] != (storage.QualityEventRecord{
		Code:      "fixture_gap",
		StartedAt: firstBucketAt,
		Flags:     1,
	}) {
		t.Fatalf("QualityEvents() = %#v", events)
	}
	outside, err := store.QualityEvents(context.Background(), firstBucketAt+20, firstBucketAt+30)
	if err != nil || len(outside) != 0 {
		t.Fatalf("outside QualityEvents() = %#v, %v", outside, err)
	}
}

func TestQualityEventsRejectsInvalidRange(t *testing.T) {
	store, _ := migratedTestStore(t)
	if _, err := store.QualityEvents(context.Background(), firstBucketAt, firstBucketAt); err == nil {
		t.Fatal("QualityEvents() accepted empty range")
	}
}

func TestCleanupQualityEventsRemovesOnlyRowsOlderThanThreeYearCutoff(t *testing.T) {
	store, databasePath := migratedTestStore(t)
	database := openRawDatabase(t, databasePath)
	cutoff := int64(1767225600)
	for index, startedAt := range []int64{cutoff - 1, cutoff, cutoff + 1} {
		if _, err := database.Exec(`
			INSERT INTO quality_event(batch_id, code, started_at, ended_at, flags, detail)
			VALUES (?, 'fixture_quality', ?, NULL, 1, '')
		`, "quality-batch-"+string(rune('a'+index)), startedAt); err != nil {
			t.Fatalf("insert quality event %d: %v", startedAt, err)
		}
	}

	deleted, err := store.CleanupQualityEvents(context.Background(), cutoff)
	if err != nil {
		t.Fatalf("CleanupQualityEvents() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("CleanupQualityEvents() deleted = %d, want 1", deleted)
	}
	events, err := store.QualityEvents(context.Background(), cutoff-10, cutoff+10)
	if err != nil {
		t.Fatalf("QualityEvents() error = %v", err)
	}
	if len(events) != 2 || events[0].StartedAt != cutoff || events[1].StartedAt != cutoff+1 {
		t.Fatalf("remaining QualityEvents() = %#v", events)
	}
}
