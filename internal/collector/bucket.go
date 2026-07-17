package collector

import (
	"errors"
	"math"

	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/storage"
)

const (
	QualityFlagGap                      int64 = 1
	QualityFlagCounterReset             int64 = 2
	QualityFlagAttributionIncomplete    int64 = 8
	QualityFlagRecoveredTimeApproximate int64 = 128
)

// ErrInvalidBucket means an observation cannot be represented exactly.
var ErrInvalidBucket = errors.New("invalid FlowLens global bucket observation")

// GlobalBucket accumulates complete values for one aligned 10-second bucket.
type GlobalBucket struct {
	rollup          storage.TrafficRollup
	observedSeconds map[int64]struct{}
}

// NewGlobalBucket creates one empty aligned bucket.
func NewGlobalBucket(start int64) (*GlobalBucket, error) {
	if start <= 0 || start%10 != 0 {
		return nil, ErrInvalidBucket
	}
	return &GlobalBucket{rollup: storage.TrafficRollup{
		ResolutionSec: 10,
		BucketStart:   start,
		BucketEnd:     start + 10,
	}, observedSeconds: make(map[int64]struct{})}, nil
}

// ObserveCounter adds one exact cumulative-counter transition.
func (b *GlobalBucket) ObserveCounter(at int64, observation CounterObservation, activeConnections int64) error {
	if at < b.rollup.BucketStart || at >= b.rollup.BucketEnd || observation.Delta.Upload < 0 ||
		observation.Delta.Download < 0 || activeConnections < 0 {
		return ErrInvalidBucket
	}
	next := b.rollup
	if !add(&next.UploadBytes, observation.Delta.Upload) ||
		!add(&next.DownloadBytes, observation.Delta.Download) ||
		!add(&next.ActiveConnectionsSum, activeConnections) ||
		!increment(&next.ActiveConnectionsSamples) {
		return ErrInvalidBucket
	}
	if _, exists := b.observedSeconds[at]; !exists {
		if !increment(&next.CounterObservedSeconds) {
			return ErrInvalidBucket
		}
	}
	if activeConnections > next.ActiveConnectionsMax {
		next.ActiveConnectionsMax = activeConnections
	}
	if observation.TimeApproximate {
		if !add(&next.RecoveredUploadBytes, observation.Delta.Upload) ||
			!add(&next.RecoveredDownloadBytes, observation.Delta.Download) {
			return ErrInvalidBucket
		}
		next.QualityFlags |= QualityFlagRecoveredTimeApproximate
	}
	if observation.AfterGap {
		next.QualityFlags |= QualityFlagGap
	}
	if observation.NewSession {
		if !increment(&next.ResetCount) {
			return ErrInvalidBucket
		}
		next.QualityFlags |= QualityFlagCounterReset
	}
	if observation.Delta.Upload != 0 || observation.Delta.Download != 0 {
		next.QualityFlags |= QualityFlagAttributionIncomplete
	}
	next.UnattributedUploadBytes = next.UploadBytes
	next.UnattributedDownloadBytes = next.DownloadBytes
	b.rollup = next
	b.observedSeconds[at] = struct{}{}
	return nil
}

// ObserveTraffic adds one bytes-per-second sample owned only by /traffic.
func (b *GlobalBucket) ObserveTraffic(at int64, sample clashapi.TrafficSample) error {
	if at < b.rollup.BucketStart || at >= b.rollup.BucketEnd || sample.Up < 0 || sample.Down < 0 {
		return ErrInvalidBucket
	}
	next := b.rollup
	wasEmpty := next.SpeedSampleCount == 0
	if !add(&next.SpeedUploadSampleSum, sample.Up) || !add(&next.SpeedDownloadSampleSum, sample.Down) ||
		!increment(&next.SpeedSampleCount) {
		return ErrInvalidBucket
	}
	if wasEmpty || sample.Up > next.PeakUploadBytesPerSecond {
		next.PeakUploadBytesPerSecond = sample.Up
		value := at
		next.PeakUploadAt = &value
	}
	if wasEmpty || sample.Down > next.PeakDownloadBytesPerSecond {
		next.PeakDownloadBytesPerSecond = sample.Down
		value := at
		next.PeakDownloadAt = &value
	}
	b.rollup = next
	return nil
}

// ObserveMemory adds one optional memory sample.
func (b *GlobalBucket) ObserveMemory(sample clashapi.MemorySample) error {
	if sample.Inuse < 0 {
		return ErrInvalidBucket
	}
	next := b.rollup
	if !add(&next.MemoryBytesSum, sample.Inuse) || !increment(&next.MemorySamples) {
		return ErrInvalidBucket
	}
	if sample.Inuse > next.MemoryBytesMax {
		next.MemoryBytesMax = sample.Inuse
	}
	b.rollup = next
	return nil
}

// Rollup returns a complete value copy.
func (b *GlobalBucket) Rollup() storage.TrafficRollup {
	copy := b.rollup
	copy.PeakUploadAt = copyInt64(b.rollup.PeakUploadAt)
	copy.PeakDownloadAt = copyInt64(b.rollup.PeakDownloadAt)
	return copy
}

// Flows returns the exact Stage 1 unattributed conservation row when needed.
func (b *GlobalBucket) Flows() []storage.FlowRollup {
	if b.rollup.UploadBytes == 0 && b.rollup.DownloadBytes == 0 {
		return nil
	}
	return []storage.FlowRollup{{
		Dimension: storage.FlowDimension{
			SourceNetwork:      []byte{},
			DestinationIP:      []byte{},
			DestinationPort:    -1,
			ClassificationCode: 3,
		},
		UploadBytes:          b.rollup.UploadBytes,
		DownloadBytes:        b.rollup.DownloadBytes,
		FlowObservationCount: b.rollup.CounterObservedSeconds,
	}}
}

func add(target *int64, value int64) bool {
	if value < 0 || *target > math.MaxInt64-value {
		return false
	}
	*target += value
	return true
}

func increment(target *int64) bool {
	return add(target, 1)
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
