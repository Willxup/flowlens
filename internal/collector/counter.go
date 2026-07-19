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

// Preview computes one transition without advancing the accepted cumulative
// cursor. Call Commit only after every consumer accepts the observation.
func (t *CounterTracker) Preview(current ByteTotals, afterGap bool) (CounterObservation, error) {
	if err := validateByteTotals(current); err != nil {
		return CounterObservation{}, err
	}
	if !t.hasLast {
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

	return observation, nil
}

// Commit advances the cumulative cursor to a previously accepted preview.
func (t *CounterTracker) Commit(observation CounterObservation) {
	t.last = observation.Current
	t.hasLast = true
}

// Observe is the compatibility wrapper for callers that do not need atomic
// coordination with another state machine.
func (t *CounterTracker) Observe(current ByteTotals, afterGap bool) (CounterObservation, error) {
	observation, err := t.Preview(current, afterGap)
	if err != nil {
		return CounterObservation{}, err
	}
	t.Commit(observation)
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
