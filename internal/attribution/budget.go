package attribution

import (
	"errors"
	"math"
	"math/bits"
	"sort"

	"github.com/Willxup/flowlens/internal/storage"
)

// ErrAttribution means a connection snapshot cannot be represented safely.
var ErrAttribution = errors.New("invalid FlowLens attribution")

// Candidate is one trusted per-connection delta kept only during preparation.
type Candidate struct {
	UUID      string
	Dimension storage.FlowDimension
	Raw       storage.ByteTotals
}

// Allocation is the exact bounded result for one global delta budget.
type Allocation struct {
	Flows        []storage.FlowRollup
	Unattributed storage.ByteTotals
	Clipped      bool
}

// Allocate clips connection deltas independently to the global upload and
// download budgets, then merges equal durable dimensions.
func Allocate(candidates []Candidate, budget storage.ByteTotals) (Allocation, error) {
	if budget.Upload < 0 || budget.Download < 0 {
		return Allocation{}, ErrAttribution
	}
	ordered := append([]Candidate(nil), candidates...)
	for index := range ordered {
		if ordered[index].Raw.Upload < 0 || ordered[index].Raw.Download < 0 {
			return Allocation{}, ErrAttribution
		}
		ordered[index].Dimension = cloneDimension(ordered[index].Dimension)
	}
	sort.Slice(ordered, func(left, right int) bool {
		leftKey := DimensionKey(ordered[left].Dimension)
		rightKey := DimensionKey(ordered[right].Dimension)
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		return ordered[left].UUID < ordered[right].UUID
	})
	upload, uploadMissing, uploadClipped, err := allocateDirection(ordered, budget.Upload, true)
	if err != nil {
		return Allocation{}, err
	}
	download, downloadMissing, downloadClipped, err := allocateDirection(ordered, budget.Download, false)
	if err != nil {
		return Allocation{}, err
	}
	type mergedFlow struct {
		dimension storage.FlowDimension
		upload    int64
		download  int64
		count     int64
	}
	merged := make(map[string]*mergedFlow)
	for index, candidate := range ordered {
		if upload[index] == 0 && download[index] == 0 {
			continue
		}
		key := DimensionKey(candidate.Dimension)
		flow := merged[key]
		if flow == nil {
			flow = &mergedFlow{dimension: cloneDimension(candidate.Dimension)}
			merged[key] = flow
		}
		if !safeInt64Add(&flow.upload, upload[index]) || !safeInt64Add(&flow.download, download[index]) ||
			!safeInt64Add(&flow.count, 1) {
			return Allocation{}, ErrAttribution
		}
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	flows := make([]storage.FlowRollup, 0, len(keys))
	for _, key := range keys {
		flow := merged[key]
		flows = append(flows, storage.FlowRollup{
			Dimension: flow.dimension, UploadBytes: flow.upload, DownloadBytes: flow.download,
			FlowObservationCount: flow.count,
		})
	}
	return Allocation{
		Flows: flows,
		Unattributed: storage.ByteTotals{
			Upload: uploadMissing, Download: downloadMissing,
		},
		Clipped: uploadClipped || downloadClipped,
	}, nil
}

func allocateDirection(candidates []Candidate, budget int64, upload bool) ([]int64, int64, bool, error) {
	total := int64(0)
	values := make([]int64, len(candidates))
	for index, candidate := range candidates {
		value := candidate.Raw.Download
		if upload {
			value = candidate.Raw.Upload
		}
		values[index] = value
		if !safeInt64Add(&total, value) {
			return nil, 0, false, ErrAttribution
		}
	}
	allocated := make([]int64, len(values))
	if total <= budget {
		copy(allocated, values)
		return allocated, budget - total, false, nil
	}
	floorSum := int64(0)
	for index, value := range values {
		share, ok := mulDivNonnegative(value, budget, total)
		if !ok || share > value || !safeInt64Add(&floorSum, share) {
			return nil, 0, false, ErrAttribution
		}
		allocated[index] = share
	}
	remainder := budget - floorSum
	for remainder > 0 {
		advanced := false
		for index, value := range values {
			if allocated[index] >= value {
				continue
			}
			allocated[index]++
			remainder--
			advanced = true
			if remainder == 0 {
				break
			}
		}
		if !advanced {
			return nil, 0, false, ErrAttribution
		}
	}
	return allocated, 0, true, nil
}

func mulDivNonnegative(value, multiplier, divisor int64) (int64, bool) {
	if value < 0 || multiplier < 0 || divisor <= 0 {
		return 0, false
	}
	high, low := bits.Mul64(uint64(value), uint64(multiplier))
	if high >= uint64(divisor) {
		return 0, false
	}
	quotient, _ := bits.Div64(high, low, uint64(divisor))
	if quotient > math.MaxInt64 {
		return 0, false
	}
	return int64(quotient), true
}

func safeInt64Add(target *int64, value int64) bool {
	if value < 0 || *target > math.MaxInt64-value {
		return false
	}
	*target += value
	return true
}

func cloneDimension(value storage.FlowDimension) storage.FlowDimension {
	value.SourceNetwork = append([]byte(nil), value.SourceNetwork...)
	value.DestinationIP = append([]byte(nil), value.DestinationIP...)
	return value
}
