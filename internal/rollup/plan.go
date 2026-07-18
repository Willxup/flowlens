package rollup

import (
	"errors"
	"sort"
	"time"

	"github.com/Willxup/flowlens/internal/config"
)

const MaximumSeriesPoints int64 = 2000

var ErrInvalidSeriesPlan = errors.New("invalid FlowLens series plan")

// Range is one requested half-open UTC Unix-second interval.
type Range struct {
	From int64
	To   int64
}

// Segment is one non-overlapping source-resolution query interval.
type Segment struct {
	ResolutionSec int64
	From          int64
	To            int64
}

// PlanSeries selects the finest retained global resolution and coarsens only
// as needed to keep ordinary chart responses near the point target.
func PlanSeries(
	rangeValue Range,
	now time.Time,
	retention config.Retention,
	location *time.Location,
) ([]Segment, bool, error) {
	if rangeValue.From <= 0 || rangeValue.To <= rangeValue.From || now.Unix() <= 0 ||
		rangeValue.To > now.Unix() || location == nil ||
		retention.TenSecondDays <= 0 || retention.MinuteDays <= 0 ||
		retention.HalfHourDays <= 0 || retention.HourDays <= 0 {
		return nil, false, ErrInvalidSeriesPlan
	}
	localNow := now.In(location)
	cutoffs := map[int64]int64{
		ResolutionTenSeconds: localNow.AddDate(0, 0, -retention.TenSecondDays).Unix(),
		ResolutionMinute:     localNow.AddDate(0, 0, -retention.MinuteDays).Unix(),
		ResolutionHalfHour:   localNow.AddDate(0, 0, -retention.HalfHourDays).Unix(),
		ResolutionHour:       localNow.AddDate(0, 0, -retention.HourDays).Unix(),
	}
	segments := retainedSegments(rangeValue, cutoffs)
	for {
		aligned, approximate, err := alignSegments(segments, rangeValue, location)
		if err != nil {
			return nil, false, err
		}
		planned, tailApproximate, err := preserveUnstableTail(aligned, rangeValue, now, location)
		if err != nil {
			return nil, false, err
		}
		approximate = approximate || tailApproximate
		if estimatedPoints(planned) <= MaximumSeriesPoints || !promoteLargestSegment(segments) {
			return planned, approximate, nil
		}
		segments = mergeSegments(segments)
	}
}

func preserveUnstableTail(
	segments []Segment,
	rangeValue Range,
	now time.Time,
	location *time.Location,
) ([]Segment, bool, error) {
	if len(segments) == 0 || segments[len(segments)-1].ResolutionSec == ResolutionTenSeconds {
		return segments, false, nil
	}
	stableBefore := now.UTC().Truncate(time.Minute).Add(-time.Minute).Unix()
	if rangeValue.To <= stableBefore {
		return segments, false, nil
	}
	last := segments[len(segments)-1]
	tailStart, err := floorBoundary(stableBefore, last.ResolutionSec, location)
	if err != nil {
		return nil, false, err
	}
	if tailStart < last.From {
		tailStart = last.From
	}
	if tailStart >= rangeValue.To {
		return segments, false, nil
	}

	result := append([]Segment(nil), segments[:len(segments)-1]...)
	if last.From < tailStart {
		last.To = tailStart
		result = append(result, last)
	}
	result, err = appendUnstableTail(result, last.ResolutionSec, tailStart, rangeValue.To, stableBefore, location)
	if err != nil {
		return nil, false, err
	}
	result = mergeSegments(result)
	return result, result[len(result)-1].To != rangeValue.To, nil
}

func appendUnstableTail(
	segments []Segment,
	targetResolution int64,
	from int64,
	to int64,
	stableBefore int64,
	location *time.Location,
) ([]Segment, error) {
	sourceResolution := rollupSourceResolution(targetResolution)
	if sourceResolution == 0 {
		return nil, ErrInvalidSeriesPlan
	}
	if sourceResolution == ResolutionTenSeconds {
		end, err := ceilBoundary(to, sourceResolution, location)
		if err != nil {
			return nil, err
		}
		return append(segments, Segment{ResolutionSec: sourceResolution, From: from, To: end}), nil
	}
	split, err := floorBoundary(stableBefore, sourceResolution, location)
	if err != nil {
		return nil, err
	}
	if split < from {
		split = from
	}
	if split > to {
		split = to
	}
	if from < split {
		segments = append(segments, Segment{ResolutionSec: sourceResolution, From: from, To: split})
	}
	if split < to {
		return appendUnstableTail(segments, sourceResolution, split, to, stableBefore, location)
	}
	return segments, nil
}

func rollupSourceResolution(targetResolution int64) int64 {
	switch targetResolution {
	case ResolutionMinute:
		return ResolutionTenSeconds
	case ResolutionHalfHour:
		return ResolutionMinute
	case ResolutionHour:
		return ResolutionHalfHour
	case ResolutionDay:
		return ResolutionMinute
	default:
		return 0
	}
}

func retainedSegments(rangeValue Range, cutoffs map[int64]int64) []Segment {
	boundaries := []int64{rangeValue.From, rangeValue.To}
	for _, cutoff := range cutoffs {
		if cutoff > rangeValue.From && cutoff < rangeValue.To {
			boundaries = append(boundaries, cutoff)
		}
	}
	sort.Slice(boundaries, func(left, right int) bool { return boundaries[left] < boundaries[right] })
	unique := boundaries[:0]
	for _, boundary := range boundaries {
		if len(unique) == 0 || unique[len(unique)-1] != boundary {
			unique = append(unique, boundary)
		}
	}
	segments := make([]Segment, 0, len(unique)-1)
	for index := 0; index+1 < len(unique); index++ {
		from, to := unique[index], unique[index+1]
		segments = append(segments, Segment{
			ResolutionSec: retainedResolution(from, cutoffs),
			From:          from,
			To:            to,
		})
	}
	return mergeSegments(segments)
}

func retainedResolution(at int64, cutoffs map[int64]int64) int64 {
	if at >= cutoffs[ResolutionTenSeconds] {
		return ResolutionTenSeconds
	}
	if at >= cutoffs[ResolutionMinute] {
		return ResolutionMinute
	}
	if at >= cutoffs[ResolutionHalfHour] {
		return ResolutionHalfHour
	}
	if at >= cutoffs[ResolutionHour] {
		return ResolutionHour
	}
	return ResolutionDay
}

func alignSegments(
	segments []Segment,
	rangeValue Range,
	location *time.Location,
) ([]Segment, bool, error) {
	if len(segments) == 0 {
		return nil, false, ErrInvalidSeriesPlan
	}
	aligned := append([]Segment(nil), segments...)
	start, err := floorBoundary(rangeValue.From, aligned[0].ResolutionSec, location)
	if err != nil {
		return nil, false, err
	}
	approximate := start != rangeValue.From
	aligned[0].From = start
	for index := 0; index+1 < len(aligned); index++ {
		resolution := aligned[index].ResolutionSec
		if aligned[index+1].ResolutionSec > resolution {
			resolution = aligned[index+1].ResolutionSec
		}
		boundary, err := ceilBoundary(segments[index].To, resolution, location)
		if err != nil {
			return nil, false, err
		}
		if boundary != segments[index].To {
			approximate = true
		}
		aligned[index].To = boundary
		aligned[index+1].From = boundary
	}
	end, err := ceilBoundary(rangeValue.To, aligned[len(aligned)-1].ResolutionSec, location)
	if err != nil {
		return nil, false, err
	}
	if end != rangeValue.To {
		approximate = true
	}
	aligned[len(aligned)-1].To = end
	filtered := aligned[:0]
	for _, segment := range aligned {
		if segment.From < segment.To {
			if len(filtered) > 0 && filtered[len(filtered)-1].To != segment.From {
				segment.From = filtered[len(filtered)-1].To
			}
			if segment.From < segment.To {
				filtered = append(filtered, segment)
			}
		}
	}
	return mergeSegments(filtered), approximate, nil
}

func floorBoundary(second, resolutionSec int64, location *time.Location) (int64, error) {
	window, err := WindowAt(time.Unix(second, 0), resolutionSec, location)
	if err != nil {
		return 0, ErrInvalidSeriesPlan
	}
	return window.BucketStart, nil
}

func ceilBoundary(second, resolutionSec int64, location *time.Location) (int64, error) {
	window, err := WindowAt(time.Unix(second, 0), resolutionSec, location)
	if err != nil {
		return 0, ErrInvalidSeriesPlan
	}
	if second == window.BucketStart {
		return second, nil
	}
	return window.BucketEnd, nil
}

func estimatedPoints(segments []Segment) int64 {
	var total int64
	for _, segment := range segments {
		duration := segment.To - segment.From
		points := (duration + segment.ResolutionSec - 1) / segment.ResolutionSec
		if total > MaximumSeriesPoints || points > MaximumSeriesPoints-total {
			return MaximumSeriesPoints + 1
		}
		total += points
	}
	return total
}

func promoteLargestSegment(segments []Segment) bool {
	selected := -1
	var selectedPoints int64
	for index, segment := range segments {
		if segment.ResolutionSec == ResolutionDay {
			continue
		}
		points := (segment.To - segment.From + segment.ResolutionSec - 1) / segment.ResolutionSec
		if selected < 0 || points > selectedPoints {
			selected = index
			selectedPoints = points
		}
	}
	if selected < 0 {
		return false
	}
	segments[selected].ResolutionSec = nextResolution(segments[selected].ResolutionSec)
	return true
}

func nextResolution(resolutionSec int64) int64 {
	switch resolutionSec {
	case ResolutionTenSeconds:
		return ResolutionMinute
	case ResolutionMinute:
		return ResolutionHalfHour
	case ResolutionHalfHour:
		return ResolutionHour
	default:
		return ResolutionDay
	}
}

func mergeSegments(segments []Segment) []Segment {
	merged := make([]Segment, 0, len(segments))
	for _, segment := range segments {
		if segment.From >= segment.To {
			continue
		}
		if len(merged) > 0 && merged[len(merged)-1].ResolutionSec == segment.ResolutionSec &&
			merged[len(merged)-1].To == segment.From {
			merged[len(merged)-1].To = segment.To
			continue
		}
		merged = append(merged, segment)
	}
	return merged
}
