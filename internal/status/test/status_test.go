package status_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/Willxup/flowlens/internal/status"
)

func TestTrackerStartsDegradedAndNotReady(t *testing.T) {
	tracker := status.NewTracker()
	want := status.Snapshot{Level: status.LevelDegraded, Reason: "starting", Ready: false}
	if got := tracker.Snapshot(); got != want {
		t.Errorf("Snapshot() = %#v, want %#v", got, want)
	}
}

func TestTrackerAcceptsValidTransitionsAndReturnsCopies(t *testing.T) {
	tracker := status.NewTracker()
	if err := tracker.Set(status.LevelOK, "ready", true); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	first := tracker.Snapshot()
	first.Reason = "mutated"
	want := status.Snapshot{Level: status.LevelOK, Reason: "ready", Ready: true}
	if got := tracker.Snapshot(); got != want {
		t.Errorf("Snapshot() = %#v, want %#v", got, want)
	}
}

func TestTrackerKeepsComponentFailuresIndependent(t *testing.T) {
	tracker := status.NewTracker()
	setter, ok := any(tracker).(interface {
		SetComponent(string, status.Level, string, bool) error
	})
	if !ok {
		t.Fatal("Tracker does not support component status")
	}
	if err := setter.SetComponent("runtime", status.LevelOK, "ready", true); err != nil {
		t.Fatalf("runtime ready error = %v", err)
	}
	if err := setter.SetComponent("maintenance", status.LevelDegraded, "maintenance_failed", true); err != nil {
		t.Fatalf("maintenance failure error = %v", err)
	}
	if err := setter.SetComponent("runtime", status.LevelOK, "ready", true); err != nil {
		t.Fatalf("runtime recovery error = %v", err)
	}
	if got := tracker.Snapshot(); got != (status.Snapshot{
		Level: status.LevelDegraded, Reason: "maintenance_failed", Ready: true,
	}) {
		t.Fatalf("Snapshot() after runtime recovery = %#v", got)
	}
	if err := setter.SetComponent("runtime", status.LevelDegraded, "clash_unavailable", true); err != nil {
		t.Fatalf("runtime failure error = %v", err)
	}
	if err := setter.SetComponent("maintenance", status.LevelOK, "ready", true); err != nil {
		t.Fatalf("maintenance recovery error = %v", err)
	}
	if got := tracker.Snapshot(); got != (status.Snapshot{
		Level: status.LevelDegraded, Reason: "clash_unavailable", Ready: true,
	}) {
		t.Fatalf("Snapshot() after maintenance recovery = %#v", got)
	}
}

func TestTrackerRejectsInvalidSnapshotWithoutChangingState(t *testing.T) {
	tracker := status.NewTracker()
	want := tracker.Snapshot()
	for name, test := range map[string]struct {
		level  status.Level
		reason string
	}{
		"invalid level": {level: "unknown", reason: "ready"},
		"empty reason":  {level: status.LevelFailed, reason: ""},
		"uppercase":     {level: status.LevelFailed, reason: "Bad"},
		"punctuation":   {level: status.LevelFailed, reason: "bad-reason"},
		"too long":      {level: status.LevelFailed, reason: "a1234567890123456789012345678901234567890123456789012345678901234"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := tracker.Set(test.level, test.reason, false); !errors.Is(err, status.ErrInvalidSnapshot) {
				t.Errorf("Set() error = %v, want ErrInvalidSnapshot", err)
			}
			if got := tracker.Snapshot(); got != want {
				t.Errorf("Snapshot() changed to %#v, want %#v", got, want)
			}
		})
	}
}

func TestTrackerSupportsConcurrentReadersAndWriters(t *testing.T) {
	tracker := status.NewTracker()
	var wait sync.WaitGroup
	for worker := range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := range 100 {
				if (worker+iteration)%2 == 0 {
					_ = tracker.Set(status.LevelOK, "ready", true)
				} else {
					_ = tracker.Set(status.LevelDegraded, "clash_unavailable", false)
				}
				_ = tracker.Snapshot()
			}
		}()
	}
	wait.Wait()
	got := tracker.Snapshot()
	if got.Level != status.LevelOK && got.Level != status.LevelDegraded {
		t.Errorf("Snapshot().Level = %q", got.Level)
	}
}
