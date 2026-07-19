package collector

import (
	"errors"
	"math"
	"sort"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/storage"
)

const (
	QualityFlagGap                      int64 = 1
	QualityFlagCounterReset             int64 = 2
	QualityFlagAttributionIncomplete    int64 = 8
	QualityFlagRecoveredTimeApproximate int64 = 128
	QualityFlagAttributionClipped       int64 = 256
)

// ErrInvalidBucket means an observation cannot be represented exactly.
var ErrInvalidBucket = errors.New("invalid FlowLens global bucket observation")

// GlobalBucket accumulates complete values for one aligned 10-second bucket.
type GlobalBucket struct {
	rollup             storage.TrafficRollup
	observedSeconds    map[int64]struct{}
	attributionSeconds map[int64]struct{}
	flowCandidates     map[string]storage.FlowRollup
	topK               int
}

// NewGlobalBucket creates one empty aligned bucket.
func NewGlobalBucket(start int64, topKValues ...int) (*GlobalBucket, error) {
	topK := 20
	if len(topKValues) == 1 {
		topK = topKValues[0]
	}
	if len(topKValues) > 1 || start <= 0 || start%10 != 0 || topK < 1 || topK > 100 {
		return nil, ErrInvalidBucket
	}
	return &GlobalBucket{rollup: storage.TrafficRollup{
		ResolutionSec: 10,
		BucketStart:   start,
		BucketEnd:     start + 10,
	}, observedSeconds: make(map[int64]struct{}), attributionSeconds: make(map[int64]struct{}),
		flowCandidates: make(map[string]storage.FlowRollup), topK: topK}, nil
}

// ObserveCounter adds one exact cumulative-counter transition.
func (b *GlobalBucket) ObserveCounter(at int64, observation CounterObservation, activeConnections int64) error {
	return b.ObserveConnections(at, observation, activeConnections, attribution.Contribution{
		Unattributed: storage.ByteTotals{
			Upload: observation.Delta.Upload, Download: observation.Delta.Download,
		},
	})
}

// ObserveConnections atomically adds one global transition and its complete
// attribution contribution.
func (b *GlobalBucket) ObserveConnections(
	at int64,
	observation CounterObservation,
	activeConnections int64,
	contribution attribution.Contribution,
) error {
	if at < b.rollup.BucketStart || at >= b.rollup.BucketEnd || observation.Delta.Upload < 0 ||
		observation.Delta.Download < 0 || activeConnections < 0 || contribution.Unattributed.Upload < 0 ||
		contribution.Unattributed.Download < 0 {
		return ErrInvalidBucket
	}
	next := b.rollup
	nextFlows := cloneFlowMap(b.flowCandidates)
	nextObservedSeconds := cloneSeconds(b.observedSeconds)
	nextAttributionSeconds := cloneSeconds(b.attributionSeconds)
	flowUpload := contribution.Unattributed.Upload
	flowDownload := contribution.Unattributed.Download
	for _, flow := range contribution.Flows {
		if flow.Dimension.ClassificationCode != 1 || flow.UploadBytes < 0 || flow.DownloadBytes < 0 ||
			flow.FlowObservationCount < 0 || !add(&flowUpload, flow.UploadBytes) || !add(&flowDownload, flow.DownloadBytes) {
			return ErrInvalidBucket
		}
		key := attribution.DimensionKey(flow.Dimension)
		merged := nextFlows[key]
		if merged.Dimension.ClassificationCode == 0 {
			merged.Dimension = cloneFlowDimension(flow.Dimension)
		}
		if !add(&merged.UploadBytes, flow.UploadBytes) || !add(&merged.DownloadBytes, flow.DownloadBytes) ||
			!add(&merged.FlowObservationCount, flow.FlowObservationCount) {
			return ErrInvalidBucket
		}
		nextFlows[key] = merged
	}
	if flowUpload != observation.Delta.Upload || flowDownload != observation.Delta.Download {
		return ErrInvalidBucket
	}
	if !add(&next.UploadBytes, observation.Delta.Upload) ||
		!add(&next.DownloadBytes, observation.Delta.Download) ||
		!add(&next.UnattributedUploadBytes, contribution.Unattributed.Upload) ||
		!add(&next.UnattributedDownloadBytes, contribution.Unattributed.Download) ||
		!add(&next.ActiveConnectionsSum, activeConnections) ||
		!increment(&next.ActiveConnectionsSamples) {
		return ErrInvalidBucket
	}
	if _, exists := nextObservedSeconds[at]; !exists {
		if !increment(&next.CounterObservedSeconds) {
			return ErrInvalidBucket
		}
		nextObservedSeconds[at] = struct{}{}
	}
	if contribution.Observed {
		if _, exists := nextAttributionSeconds[at]; !exists {
			if !increment(&next.AttributionObservedSeconds) {
				return ErrInvalidBucket
			}
			nextAttributionSeconds[at] = struct{}{}
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
	if contribution.Unattributed.Upload != 0 || contribution.Unattributed.Download != 0 {
		next.QualityFlags |= QualityFlagAttributionIncomplete
	}
	if contribution.Clipped {
		next.QualityFlags |= QualityFlagAttributionClipped
	}
	b.rollup = next
	b.flowCandidates = nextFlows
	b.observedSeconds = nextObservedSeconds
	b.attributionSeconds = nextAttributionSeconds
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

// Flows returns deterministic Top K concrete rows plus explicit other and
// unattributed conservation rows for every nonzero global bucket.
func (b *GlobalBucket) Flows() []storage.FlowRollup {
	if b.rollup.UploadBytes == 0 && b.rollup.DownloadBytes == 0 {
		return nil
	}
	concrete := make([]storage.FlowRollup, 0, len(b.flowCandidates))
	for _, flow := range b.flowCandidates {
		flow.Dimension = cloneFlowDimension(flow.Dimension)
		concrete = append(concrete, flow)
	}
	sort.Slice(concrete, func(left, right int) bool {
		leftTotal := uint64(concrete[left].UploadBytes) + uint64(concrete[left].DownloadBytes)
		rightTotal := uint64(concrete[right].UploadBytes) + uint64(concrete[right].DownloadBytes)
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		return attribution.DimensionKey(concrete[left].Dimension) < attribution.DimensionKey(concrete[right].Dimension)
	})
	keep := len(concrete)
	if keep > b.topK {
		keep = b.topK
	}
	result := make([]storage.FlowRollup, 0, keep+2)
	result = append(result, concrete[:keep]...)
	other := storage.FlowRollup{Dimension: specialDimension(2)}
	for _, flow := range concrete[keep:] {
		if !add(&other.UploadBytes, flow.UploadBytes) || !add(&other.DownloadBytes, flow.DownloadBytes) ||
			!add(&other.FlowObservationCount, flow.FlowObservationCount) {
			return nil
		}
	}
	result = append(result, other, storage.FlowRollup{
		Dimension:   specialDimension(3),
		UploadBytes: b.rollup.UnattributedUploadBytes, DownloadBytes: b.rollup.UnattributedDownloadBytes,
		FlowObservationCount: b.rollup.CounterObservedSeconds,
	})
	return result
}

func specialDimension(classification int64) storage.FlowDimension {
	return storage.FlowDimension{
		SourceNetwork: []byte{}, DestinationIP: []byte{}, DestinationPort: -1,
		ClassificationCode: classification,
	}
}

func cloneFlowMap(values map[string]storage.FlowRollup) map[string]storage.FlowRollup {
	cloned := make(map[string]storage.FlowRollup, len(values))
	for key, value := range values {
		value.Dimension = cloneFlowDimension(value.Dimension)
		cloned[key] = value
	}
	return cloned
}

func cloneFlowDimension(value storage.FlowDimension) storage.FlowDimension {
	value.SourceNetwork = append([]byte(nil), value.SourceNetwork...)
	value.DestinationIP = append([]byte(nil), value.DestinationIP...)
	return value
}

func cloneSeconds(values map[int64]struct{}) map[int64]struct{} {
	cloned := make(map[int64]struct{}, len(values))
	for value := range values {
		cloned[value] = struct{}{}
	}
	return cloned
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
