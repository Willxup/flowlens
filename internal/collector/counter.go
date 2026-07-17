package collector

import "errors"

// ByteTotals contains exact cumulative upload and download bytes.
type ByteTotals struct {
	Upload   int64
	Download int64
}

// CounterObservation describes one pure cumulative-counter transition.
type CounterObservation struct {
	Previous        ByteTotals
	Current         ByteTotals
	Delta           ByteTotals
	Baseline        bool
	NewSession      bool
	AfterGap        bool
	TimeApproximate bool
}

// CounterTracker holds the last accepted global cumulative totals.
// It is intended to be owned by one collector goroutine.
type CounterTracker struct {
	last    ByteTotals
	hasLast bool
}

// NewCounterTracker constructs a tracker from an optional persisted cursor.
func NewCounterTracker(persisted *ByteTotals) (*CounterTracker, error) {
	tracker := &CounterTracker{}
	if persisted == nil {
		return tracker, nil
	}
	if err := validateByteTotals(*persisted); err != nil {
		return nil, errors.New("persisted byte totals are invalid")
	}
	tracker.last = *persisted
	tracker.hasLast = true
	return tracker, nil
}

// Observe accepts one nonnegative cumulative total and computes its exact
// transition from the previous accepted value.
func (t *CounterTracker) Observe(current ByteTotals, afterGap bool) (CounterObservation, error) {
	if err := validateByteTotals(current); err != nil {
		return CounterObservation{}, err
	}
	if !t.hasLast {
		t.last = current
		t.hasLast = true
		return CounterObservation{
			Current:  current,
			Baseline: true,
		}, nil
	}

	observation := CounterObservation{
		Previous: t.last,
		Current:  current,
		AfterGap: afterGap,
	}
	if current.Upload < t.last.Upload || current.Download < t.last.Download {
		observation.Delta = current
		observation.NewSession = true
	} else {
		observation.Delta = ByteTotals{
			Upload:   current.Upload - t.last.Upload,
			Download: current.Download - t.last.Download,
		}
	}
	observation.TimeApproximate = afterGap &&
		(observation.Delta.Upload > 0 || observation.Delta.Download > 0)

	t.last = current
	return observation, nil
}

// Last returns the last accepted total and whether a value is present.
func (t *CounterTracker) Last() (ByteTotals, bool) {
	return t.last, t.hasLast
}

func validateByteTotals(totals ByteTotals) error {
	if totals.Upload < 0 || totals.Download < 0 {
		return errors.New("byte totals must be nonnegative")
	}
	return nil
}
