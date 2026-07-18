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
