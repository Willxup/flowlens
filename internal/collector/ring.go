package collector

import (
	"errors"
	"sync"
	"time"
)

// DefaultRingCapacity retains one sample per second for sixty minutes.
const DefaultRingCapacity = 3600

// SampleStatus describes whether a live speed sample was collected normally or
// while the collector was otherwise degraded.
type SampleStatus string

const (
	SampleStatusOK       SampleStatus = "ok"
	SampleStatusDegraded SampleStatus = "degraded"
)

// SpeedSample is one global bytes-per-second and active-connection sample.
type SpeedSample struct {
	Timestamp              time.Time
	UploadBytesPerSecond   int64
	DownloadBytesPerSecond int64
	ActiveConnections      int64
	Status                 SampleStatus
}

// Ring is a fixed-capacity, thread-safe chronological speed sample buffer.
type Ring struct {
	mu      sync.RWMutex
	samples []SpeedSample
	start   int
	count   int
}

// NewRing constructs an empty fixed-capacity ring.
func NewRing(capacity int) (*Ring, error) {
	if capacity <= 0 {
		return nil, errors.New("ring capacity must be positive")
	}
	return &Ring{samples: make([]SpeedSample, capacity)}, nil
}

// Add appends a validated sample, replacing the oldest sample when full.
func (r *Ring) Add(sample SpeedSample) error {
	if err := validateSpeedSample(sample); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count < len(r.samples) {
		index := (r.start + r.count) % len(r.samples)
		r.samples[index] = sample
		r.count++
		return nil
	}
	r.samples[r.start] = sample
	r.start = (r.start + 1) % len(r.samples)
	return nil
}

// Snapshot returns a chronological copy without exposing internal storage.
func (r *Ring) Snapshot() []SpeedSample {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot := make([]SpeedSample, r.count)
	if r.count == 0 {
		return snapshot
	}
	firstCount := min(r.count, len(r.samples)-r.start)
	copy(snapshot, r.samples[r.start:r.start+firstCount])
	copy(snapshot[firstCount:], r.samples[:r.count-firstCount])
	return snapshot
}

// Len returns the number of retained samples.
func (r *Ring) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

func validateSpeedSample(sample SpeedSample) error {
	if sample.Timestamp.IsZero() {
		return errors.New("speed sample timestamp is required")
	}
	if sample.UploadBytesPerSecond < 0 || sample.DownloadBytesPerSecond < 0 || sample.ActiveConnections < 0 {
		return errors.New("speed sample values must be nonnegative")
	}
	if sample.Status != SampleStatusOK && sample.Status != SampleStatusDegraded {
		return errors.New("speed sample status is invalid")
	}
	return nil
}
