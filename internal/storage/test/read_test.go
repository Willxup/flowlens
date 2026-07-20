package storage_test

import (
	"context"
	"fmt"
	"testing"
)

func TestRuntimeSessionsReturnsNewestHundred(t *testing.T) {
	store, databasePath := migratedTestStore(t)
	database := openRawDatabase(t, databasePath)
	transaction, err := database.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	for index := int64(1); index <= 105; index++ {
		if _, err := transaction.Exec(`
			INSERT INTO runtime_session(
				id, started_at, start_reason,
				last_upload_total, last_download_total, last_seen_at,
				sing_box_version, data_gap_before_seconds
			) VALUES (?, ?, 'startup', 0, 0, ?, 'fixture', 0)
		`, fmt.Sprintf("fixture-session-%03d", index), index, index); err != nil {
			_ = transaction.Rollback()
			t.Fatalf("insert runtime session %d: %v", index, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	sessions, err := store.RuntimeSessions(context.Background(), 100)
	if err != nil {
		t.Fatalf("RuntimeSessions() error = %v", err)
	}
	if len(sessions) != 100 {
		t.Fatalf("RuntimeSessions() length = %d", len(sessions))
	}
	if sessions[0].StartedAt != 105 || sessions[99].StartedAt != 6 {
		t.Fatalf("RuntimeSessions() first:%#v last:%#v", sessions[0], sessions[99])
	}
	for _, limit := range []int{0, 101} {
		if sessions, err := store.RuntimeSessions(context.Background(), limit); err == nil || sessions != nil {
			t.Fatalf("RuntimeSessions(%d) = %#v, %v", limit, sessions, err)
		}
	}
}
