package query

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

var ErrQuery = errors.New("FlowLens historical query failed")

// Store is the exact read-only storage boundary used by historical queries.
type Store interface {
	TrafficSeries(context.Context, []rollup.Segment) ([]storage.TrafficRollup, error)
	QualityEvents(context.Context, int64, int64) ([]storage.QualityEventRecord, error)
	CapacityStatus(context.Context) (storage.CapacityStatus, error)
	LatestMaintenance(context.Context, string) (storage.MaintenanceRun, bool, error)
}

// Service owns historical planning and read-model aggregation.
type Service struct {
	store     Store
	now       func() time.Time
	retention config.Retention
	location  *time.Location
}

// NewService validates the minimal query dependencies.
func NewService(
	store Store,
	now func() time.Time,
	retention config.Retention,
	location *time.Location,
) (*Service, error) {
	if store == nil || now == nil || location == nil || retention.TenSecondDays <= 0 ||
		retention.MinuteDays <= 0 || retention.HalfHourDays <= 0 || retention.HourDays <= 0 {
		return nil, ErrQuery
	}
	return &Service{store: store, now: now, retention: retention, location: location}, nil
}

// Series returns the planned non-overlapping exact storage points.
func (s *Service) Series(ctx context.Context, rangeValue rollup.Range) (Series, error) {
	segments, approximate, err := rollup.PlanSeries(rangeValue, s.now(), s.retention, s.location)
	if err != nil {
		return Series{}, ErrQuery
	}
	points, err := s.store.TrafficSeries(ctx, segments)
	if err != nil {
		return Series{}, ErrQuery
	}
	return Series{Points: points, BoundaryApproximate: approximate}, nil
}

// Overview returns the requested and previous equal-duration summaries.
func (s *Service) Overview(ctx context.Context, rangeValue rollup.Range) (Overview, error) {
	now := s.now()
	currentSegments, currentApproximate, err := rollup.PlanSeries(rangeValue, now, s.retention, s.location)
	if err != nil {
		return Overview{}, ErrQuery
	}
	duration := rangeValue.To - rangeValue.From
	previousRange := rollup.Range{From: rangeValue.From - duration, To: rangeValue.From}
	if previousRange.From <= 0 {
		return Overview{}, ErrQuery
	}
	previousSegments, previousApproximate, err := rollup.PlanSeries(previousRange, now, s.retention, s.location)
	if err != nil {
		return Overview{}, ErrQuery
	}
	sharedBoundary := currentSegments[0].From
	previousSegments = trimSegmentsTo(previousSegments, sharedBoundary)
	current, err := s.store.TrafficSeries(ctx, currentSegments)
	if err != nil {
		return Overview{}, ErrQuery
	}
	var previous []storage.TrafficRollup
	if len(previousSegments) > 0 {
		previous, err = s.store.TrafficSeries(ctx, previousSegments)
		if err != nil {
			return Overview{}, ErrQuery
		}
	}
	currentTotals, ok := summarize(current)
	if !ok {
		return Overview{}, ErrQuery
	}
	previousTotals, ok := summarize(previous)
	if !ok {
		return Overview{}, ErrQuery
	}
	return Overview{
		Current: currentTotals, Previous: previousTotals,
		BoundaryApproximate: currentApproximate || previousApproximate || sharedBoundary != rangeValue.From,
	}, nil
}

// Quality returns public-safe quality events in the requested range.
func (s *Service) Quality(ctx context.Context, rangeValue rollup.Range) (Quality, error) {
	if rangeValue.From <= 0 || rangeValue.To <= rangeValue.From || rangeValue.To > s.now().Unix() {
		return Quality{}, ErrQuery
	}
	events, err := s.store.QualityEvents(ctx, rangeValue.From, rangeValue.To)
	if err != nil {
		return Quality{}, ErrQuery
	}
	return Quality{Events: events}, nil
}

// Storage returns capacity plus the newest rollup/cleanup maintenance result.
func (s *Service) Storage(ctx context.Context) (Storage, error) {
	capacity, err := s.store.CapacityStatus(ctx)
	if err != nil {
		return Storage{}, ErrQuery
	}
	run, found, err := s.store.LatestMaintenance(ctx, "rollup_cleanup")
	if err != nil {
		return Storage{}, ErrQuery
	}
	result := Storage{Capacity: capacity}
	if found {
		copy := run
		result.LastRollupCleanup = &copy
	}
	return result, nil
}

func trimSegmentsTo(segments []rollup.Segment, to int64) []rollup.Segment {
	trimmed := make([]rollup.Segment, 0, len(segments))
	for _, segment := range segments {
		if segment.From >= to {
			break
		}
		if segment.To > to {
			segment.To = to
		}
		if segment.From < segment.To {
			trimmed = append(trimmed, segment)
		}
	}
	return trimmed
}

func summarize(points []storage.TrafficRollup) (Totals, bool) {
	var result Totals
	for _, point := range points {
		values := []struct {
			total *int64
			value int64
		}{
			{&result.UploadBytes, point.UploadBytes},
			{&result.DownloadBytes, point.DownloadBytes},
			{&result.ElapsedSeconds, point.BucketEnd - point.BucketStart},
			{&result.ObservedSeconds, point.CounterObservedSeconds},
		}
		for _, value := range values {
			if value.value < 0 || *value.total > math.MaxInt64-value.value {
				return Totals{}, false
			}
			*value.total += value.value
		}
	}
	return result, true
}
