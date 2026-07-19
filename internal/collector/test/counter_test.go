package collector_test

import (
	"testing"

	"github.com/Willxup/flowlens/internal/collector"
)

func TestCounterTrackerFirstObservationEstablishesBaseline(t *testing.T) {
	tracker := newCounterTracker(t, nil)
	current := collector.ByteTotals{Upload: 1000, Download: 4000}

	observation, err := tracker.Observe(current, false)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	want := collector.CounterObservation{
		Current:  current,
		Baseline: true,
	}
	if observation != want {
		t.Errorf("Observe() = %#v, want %#v", observation, want)
	}
	assertLast(t, tracker, current, true)
}

func TestCounterTrackerPreviewIsPureUntilCommit(t *testing.T) {
	previous := collector.ByteTotals{Upload: 100, Download: 400}
	tracker := newCounterTracker(t, &previous)
	current := collector.ByteTotals{Upload: 125, Download: 475}
	preview, err := tracker.Preview(current, true)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.Delta != (collector.ByteTotals{Upload: 25, Download: 75}) || !preview.AfterGap || !preview.TimeApproximate {
		t.Errorf("Preview() = %#v", preview)
	}
	assertLast(t, tracker, previous, true)
	repeated, err := tracker.Preview(current, true)
	if err != nil || repeated != preview {
		t.Fatalf("repeated Preview() = %#v, %v", repeated, err)
	}
	tracker.Commit(preview)
	assertLast(t, tracker, current, true)
}

func TestCounterTrackerFirstObservationDoesNotRetainGapFlag(t *testing.T) {
	tracker := newCounterTracker(t, nil)

	observation, err := tracker.Observe(collector.ByteTotals{Upload: 5, Download: 7}, true)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if !observation.Baseline || observation.AfterGap || observation.TimeApproximate {
		t.Errorf("Observe() = %#v", observation)
	}
}

func TestCounterTrackerComputesOrdinaryAndZeroDeltas(t *testing.T) {
	tracker := newCounterTracker(t, nil)
	first := collector.ByteTotals{Upload: 1000, Download: 4000}
	if _, err := tracker.Observe(first, false); err != nil {
		t.Fatalf("first Observe() error = %v", err)
	}

	second := collector.ByteTotals{Upload: 1250, Download: 4750}
	observation, err := tracker.Observe(second, false)
	if err != nil {
		t.Fatalf("second Observe() error = %v", err)
	}
	want := collector.CounterObservation{
		Previous: first,
		Current:  second,
		Delta:    collector.ByteTotals{Upload: 250, Download: 750},
	}
	if observation != want {
		t.Errorf("second Observe() = %#v, want %#v", observation, want)
	}

	zero, err := tracker.Observe(second, false)
	if err != nil {
		t.Fatalf("zero Observe() error = %v", err)
	}
	wantZero := collector.CounterObservation{Previous: second, Current: second}
	if zero != wantZero {
		t.Errorf("zero Observe() = %#v, want %#v", zero, wantZero)
	}
}

func TestCounterTrackerRecoversFromPersistedState(t *testing.T) {
	previous := collector.ByteTotals{Upload: 1000, Download: 4000}
	tracker := newCounterTracker(t, &previous)
	current := collector.ByteTotals{Upload: 1250, Download: 4750}

	observation, err := tracker.Observe(current, false)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	want := collector.CounterObservation{
		Previous: previous,
		Current:  current,
		Delta:    collector.ByteTotals{Upload: 250, Download: 750},
	}
	if observation != want {
		t.Errorf("Observe() = %#v, want %#v", observation, want)
	}
}

func TestCounterTrackerTreatsEitherDecreaseAsNewSession(t *testing.T) {
	previous := collector.ByteTotals{Upload: 100, Download: 400}
	tests := map[string]collector.ByteTotals{
		"upload":   {Upload: 90, Download: 450},
		"download": {Upload: 120, Download: 390},
		"both":     {Upload: 80, Download: 300},
	}
	for name, current := range tests {
		t.Run(name, func(t *testing.T) {
			tracker := newCounterTracker(t, &previous)
			observation, err := tracker.Observe(current, false)
			if err != nil {
				t.Fatalf("Observe() error = %v", err)
			}
			want := collector.CounterObservation{
				Previous:   previous,
				Current:    current,
				Delta:      current,
				NewSession: true,
			}
			if observation != want {
				t.Errorf("Observe() = %#v, want %#v", observation, want)
			}
		})
	}
}

func TestCounterTrackerMarksRecoveredGapDeltaAsTimeApproximate(t *testing.T) {
	previous := collector.ByteTotals{Upload: 100, Download: 400}
	tracker := newCounterTracker(t, &previous)
	current := collector.ByteTotals{Upload: 125, Download: 475}

	observation, err := tracker.Observe(current, true)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	want := collector.CounterObservation{
		Previous:        previous,
		Current:         current,
		Delta:           collector.ByteTotals{Upload: 25, Download: 75},
		AfterGap:        true,
		TimeApproximate: true,
	}
	if observation != want {
		t.Errorf("Observe() = %#v, want %#v", observation, want)
	}
}

func TestCounterTrackerMarksGapWithoutDeltaButNotApproximateTime(t *testing.T) {
	previous := collector.ByteTotals{Upload: 100, Download: 400}
	tracker := newCounterTracker(t, &previous)

	observation, err := tracker.Observe(previous, true)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	want := collector.CounterObservation{
		Previous: previous,
		Current:  previous,
		AfterGap: true,
	}
	if observation != want {
		t.Errorf("Observe() = %#v, want %#v", observation, want)
	}
}

func TestCounterTrackerMarksResetAfterGap(t *testing.T) {
	previous := collector.ByteTotals{Upload: 100, Download: 400}
	tracker := newCounterTracker(t, &previous)
	current := collector.ByteTotals{Upload: 10, Download: 40}

	observation, err := tracker.Observe(current, true)
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	want := collector.CounterObservation{
		Previous:        previous,
		Current:         current,
		Delta:           current,
		NewSession:      true,
		AfterGap:        true,
		TimeApproximate: true,
	}
	if observation != want {
		t.Errorf("Observe() = %#v, want %#v", observation, want)
	}
}

func TestNewCounterTrackerRejectsNegativePersistedState(t *testing.T) {
	for name, state := range map[string]collector.ByteTotals{
		"upload":   {Upload: -1},
		"download": {Download: -1},
	} {
		t.Run(name, func(t *testing.T) {
			tracker, err := collector.NewCounterTracker(&state)
			if err == nil {
				t.Fatal("NewCounterTracker() error = nil")
			}
			if tracker != nil {
				t.Errorf("NewCounterTracker() tracker = %#v", tracker)
			}
		})
	}
}

func TestCounterTrackerRejectsNegativeObservationWithoutAdvancing(t *testing.T) {
	previous := collector.ByteTotals{Upload: 100, Download: 400}
	for name, invalid := range map[string]collector.ByteTotals{
		"upload":   {Upload: -1, Download: 400},
		"download": {Upload: 100, Download: -1},
	} {
		t.Run(name, func(t *testing.T) {
			tracker := newCounterTracker(t, &previous)
			if _, err := tracker.Observe(invalid, true); err == nil {
				t.Fatal("invalid Observe() error = nil")
			}
			assertLast(t, tracker, previous, true)

			current := collector.ByteTotals{Upload: 125, Download: 475}
			observation, err := tracker.Observe(current, false)
			if err != nil {
				t.Fatalf("valid Observe() error = %v", err)
			}
			if observation.Delta != (collector.ByteTotals{Upload: 25, Download: 75}) {
				t.Errorf("valid Observe().Delta = %#v", observation.Delta)
			}
		})
	}
}

func TestCounterTrackerCopiesPersistedState(t *testing.T) {
	persisted := collector.ByteTotals{Upload: 100, Download: 400}
	tracker := newCounterTracker(t, &persisted)
	persisted = collector.ByteTotals{Upload: 999, Download: 999}

	assertLast(t, tracker, collector.ByteTotals{Upload: 100, Download: 400}, true)
}

func TestCounterTrackerLastIsAbsentBeforeFirstObservation(t *testing.T) {
	tracker := newCounterTracker(t, nil)
	assertLast(t, tracker, collector.ByteTotals{}, false)
}

func newCounterTracker(t *testing.T, persisted *collector.ByteTotals) *collector.CounterTracker {
	t.Helper()
	tracker, err := collector.NewCounterTracker(persisted)
	if err != nil {
		t.Fatalf("NewCounterTracker() error = %v", err)
	}
	return tracker
}

func assertLast(t *testing.T, tracker *collector.CounterTracker, want collector.ByteTotals, wantOK bool) {
	t.Helper()
	got, ok := tracker.Last()
	if ok != wantOK || got != want {
		t.Errorf("Last() = %#v, %t; want %#v, %t", got, ok, want, wantOK)
	}
}
