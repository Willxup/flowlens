package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Willxup/flowlens/internal/storage"
)

func TestServiceLabelsCRUDAndStableOrdering(t *testing.T) {
	store, _ := migratedTestStore(t)
	createdEndpoint, err := store.CreateLabel(context.Background(), storage.ServiceLabel{
		LabelType: "endpoint", MatchValue: "198.51.100.2:443", DisplayName: "API", CreatedAt: 10, UpdatedAt: 10,
	})
	if err != nil || createdEndpoint.ID <= 0 {
		t.Fatalf("CreateLabel(endpoint) = %#v, %v", createdEndpoint, err)
	}
	createdHost, err := store.CreateLabel(context.Background(), storage.ServiceLabel{
		LabelType: "host", MatchValue: "198.51.100.1", DisplayName: "Host", CreatedAt: 11, UpdatedAt: 11,
	})
	if err != nil {
		t.Fatalf("CreateLabel(host) error = %v", err)
	}
	if _, err := store.CreateLabel(context.Background(), storage.ServiceLabel{
		LabelType: "host", MatchValue: "198.51.100.1", DisplayName: "Duplicate", CreatedAt: 12, UpdatedAt: 12,
	}); !errors.Is(err, storage.ErrLabelConflict) {
		t.Fatalf("duplicate CreateLabel() error = %v", err)
	}
	labels, err := store.Labels(context.Background())
	if err != nil || len(labels) != 2 || labels[0].LabelType != "endpoint" || labels[1].LabelType != "host" {
		t.Fatalf("Labels() = %#v, %v", labels, err)
	}
	updated, err := store.UpdateLabel(context.Background(), createdHost.ID, "Renamed", 20)
	if err != nil || updated.DisplayName != "Renamed" || updated.MatchValue != createdHost.MatchValue || updated.UpdatedAt != 20 {
		t.Fatalf("UpdateLabel() = %#v, %v", updated, err)
	}
	deleted, err := store.DeleteLabel(context.Background(), createdEndpoint.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteLabel() = %t, %v", deleted, err)
	}
	deleted, err = store.DeleteLabel(context.Background(), createdEndpoint.ID)
	if err != nil || deleted {
		t.Fatalf("second DeleteLabel() = %t, %v", deleted, err)
	}
}
