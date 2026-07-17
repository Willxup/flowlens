package httpapi_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/httpapi"
)

func TestSessionStoreCreatesUniqueUnpredictableIDs(t *testing.T) {
	store, err := httpapi.NewSessionStore(time.Hour)
	if err != nil {
		t.Fatalf("NewSessionStore() error = %v", err)
	}
	now := time.Unix(1_700_000_000, 0)
	first, err := store.Create(now)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	second, err := store.Create(now)
	if err != nil {
		t.Fatalf("second Create() error = %v", err)
	}
	if first == second || len(first) < 43 || len(second) < 43 {
		t.Errorf("session ids have unexpected length or equality: %q %q", first, second)
	}
	if !store.Valid(first, now.Add(59*time.Minute)) || !store.Valid(second, now) {
		t.Error("new sessions are not valid")
	}
}

func TestSessionStoreUsesAbsoluteExpiryAndDeletion(t *testing.T) {
	store, _ := httpapi.NewSessionStore(time.Minute)
	now := time.Unix(1_700_000_000, 0)
	id, _ := store.Create(now)
	for range 10 {
		if !store.Valid(id, now.Add(59*time.Second)) {
			t.Fatal("Valid() unexpectedly expired or extended the session")
		}
	}
	if store.Valid(id, now.Add(time.Minute)) {
		t.Error("Valid() accepted a session at its absolute expiry")
	}
	second, _ := store.Create(now)
	store.Delete(second)
	if store.Valid(second, now) {
		t.Error("Delete() left the session valid")
	}
}

func TestSessionStoreCapsAt64AndEvictsOldestCreated(t *testing.T) {
	store, _ := httpapi.NewSessionStore(24 * time.Hour)
	base := time.Unix(1_700_000_000, 0)
	ids := make([]string, 65)
	for index := range ids {
		ids[index], _ = store.Create(base.Add(time.Duration(index) * time.Second))
	}
	if got := store.Len(base.Add(65 * time.Second)); got != 64 {
		t.Fatalf("Len() = %d, want 64", got)
	}
	if store.Valid(ids[0], base.Add(65*time.Second)) {
		t.Error("oldest session was not evicted")
	}
	if !store.Valid(ids[64], base.Add(65*time.Second)) {
		t.Error("newest session was evicted")
	}
}

func TestSessionStoreRemovesExpiredBeforeEviction(t *testing.T) {
	store, _ := httpapi.NewSessionStore(time.Minute)
	base := time.Unix(1_700_000_000, 0)
	for range 64 {
		_, _ = store.Create(base)
	}
	newID, err := store.Create(base.Add(2 * time.Minute))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := store.Len(base.Add(2 * time.Minute)); got != 1 || !store.Valid(newID, base.Add(2*time.Minute)) {
		t.Errorf("expired cleanup left len=%d valid=%t", got, store.Valid(newID, base.Add(2*time.Minute)))
	}
}

func TestSessionStoreFormattingRedactsIDs(t *testing.T) {
	store, _ := httpapi.NewSessionStore(time.Hour)
	id, _ := store.Create(time.Now())
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if formatted := fmt.Sprintf(format, store); strings.Contains(formatted, id) {
			t.Errorf("fmt.Sprintf(%q) leaked session id: %s", format, formatted)
		}
	}
}

func TestSessionStoreSupportsConcurrentAccess(t *testing.T) {
	store, _ := httpapi.NewSessionStore(time.Hour)
	now := time.Now()
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 50 {
				id, err := store.Create(now)
				if err != nil {
					t.Errorf("Create() error = %v", err)
					return
				}
				_ = store.Valid(id, now)
				store.Delete(id)
			}
		}()
	}
	wait.Wait()
}

func TestNewSessionStoreRejectsInvalidTTL(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second} {
		if store, err := httpapi.NewSessionStore(ttl); err == nil || store != nil {
			t.Errorf("NewSessionStore(%v) = %#v, %v", ttl, store, err)
		}
	}
}
