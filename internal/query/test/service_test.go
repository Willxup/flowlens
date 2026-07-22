package query_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestServiceSeriesPlansAndReturnsStoredPoints(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	point := storage.TrafficRollup{
		ResolutionSec: rollup.ResolutionMinute,
		BucketStart:   now.Add(-time.Hour).Unix(),
		BucketEnd:     now.Add(-time.Hour + time.Minute).Unix(),
		UploadBytes:   100,
		DownloadBytes: 400,
	}
	store := &recordingQueryStore{trafficResponses: [][]storage.TrafficRollup{{point}}}
	service := newService(t, store, now)
	rangeValue := rollup.Range{From: now.Add(-24 * time.Hour).Unix(), To: now.Unix()}

	series, err := service.Series(context.Background(), rangeValue)
	if err != nil {
		t.Fatalf("Series() error = %v", err)
	}
	if len(store.segmentCalls) != 1 || len(store.segmentCalls[0]) == 0 || !reflect.DeepEqual(series.Points, []storage.TrafficRollup{point}) {
		t.Fatalf("Series() = %#v, calls=%#v", series, store.segmentCalls)
	}
}

func TestServiceOverviewSumsCurrentAndPreviousEqualRanges(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	store := &recordingQueryStore{trafficResponses: [][]storage.TrafficRollup{
		{
			{BucketStart: 100, BucketEnd: 160, UploadBytes: 100, DownloadBytes: 400, CounterObservedSeconds: 50},
			{BucketStart: 160, BucketEnd: 220, UploadBytes: 200, DownloadBytes: 600, CounterObservedSeconds: 60},
		},
		{
			{BucketStart: 40, BucketEnd: 100, UploadBytes: 80, DownloadBytes: 320, CounterObservedSeconds: 55},
		},
	}}
	service := newService(t, store, now)
	rangeValue := rollup.Range{From: now.Add(-time.Hour).Unix(), To: now.Unix()}

	overview, err := service.Overview(context.Background(), rangeValue)
	if err != nil {
		t.Fatalf("Overview() error = %v", err)
	}
	if overview.Current != (query.Totals{
		UploadBytes: 300, DownloadBytes: 1000, ElapsedSeconds: 120, ObservedSeconds: 110,
	}) || overview.Previous != (query.Totals{
		UploadBytes: 80, DownloadBytes: 320, ElapsedSeconds: 60, ObservedSeconds: 55,
	}) {
		t.Fatalf("Overview() = %#v", overview)
	}
	if len(store.segmentCalls) != 2 {
		t.Fatalf("overview segment calls = %#v", store.segmentCalls)
	}
}

func TestServiceOverviewAllowsAllDataWithoutPreviousRange(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	store := &recordingQueryStore{trafficResponses: [][]storage.TrafficRollup{{{
		BucketStart: now.Add(-24 * time.Hour).Unix(), BucketEnd: now.Unix(),
		UploadBytes: 100, DownloadBytes: 400, CounterObservedSeconds: 86_400,
	}}}}
	service := newService(t, store, now)

	overview, err := service.Overview(context.Background(), rollup.Range{From: 86_400, To: now.Unix()})
	if err != nil {
		t.Fatalf("Overview() error = %v", err)
	}
	if overview.Current.UploadBytes != 100 || overview.Current.DownloadBytes != 400 || overview.Previous != (query.Totals{}) {
		t.Fatalf("Overview() = %#v", overview)
	}
	if len(store.segmentCalls) != 1 {
		t.Fatalf("segment calls = %#v, want current range only", store.segmentCalls)
	}
}

func TestServiceOverviewUsesOneSharedApproximateBoundary(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	store := &recordingQueryStore{trafficResponses: [][]storage.TrafficRollup{{}, {}}}
	service := newService(t, store, now)
	rangeValue := rollup.Range{From: now.Add(-time.Hour).Add(3 * time.Second).Unix(), To: now.Unix()}

	overview, err := service.Overview(context.Background(), rangeValue)
	if err != nil {
		t.Fatalf("Overview() error = %v", err)
	}
	if len(store.segmentCalls) != 2 || len(store.segmentCalls[0]) == 0 || len(store.segmentCalls[1]) == 0 {
		t.Fatalf("overview segment calls = %#v", store.segmentCalls)
	}
	currentStart := store.segmentCalls[0][0].From
	previousEnd := store.segmentCalls[1][len(store.segmentCalls[1])-1].To
	if previousEnd > currentStart {
		t.Fatalf("overview ranges overlap: current=%#v previous=%#v", store.segmentCalls[0], store.segmentCalls[1])
	}
	field := reflect.ValueOf(overview).FieldByName("BoundaryApproximate")
	if !field.IsValid() || field.Kind() != reflect.Bool || !field.Bool() {
		t.Fatalf("Overview().BoundaryApproximate is not true: %#v", overview)
	}
}

func TestServiceQualityAndStorageUsePublicReadModels(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	endedAt := now.Add(-time.Minute).Unix()
	run := storage.MaintenanceRun{Operation: "rollup_cleanup", StartedAt: now.Add(-2 * time.Minute).Unix(), EndedAt: &endedAt}
	store := &recordingQueryStore{
		qualityEvents:  []storage.QualityEventRecord{{Code: "fixture_gap", StartedAt: now.Add(-time.Hour).Unix(), Flags: 1}},
		capacity:       storage.CapacityStatus{DatabaseBytes: 4096, WALBytes: 1024, SoftLimitBytes: 1 << 20},
		maintenance:    run,
		hasMaintenance: true,
	}
	service := newService(t, store, now)
	rangeValue := rollup.Range{From: now.Add(-2 * time.Hour).Unix(), To: now.Unix()}

	quality, err := service.Quality(context.Background(), rangeValue)
	if err != nil || !reflect.DeepEqual(quality.Events, store.qualityEvents) {
		t.Fatalf("Quality() = %#v, %v", quality, err)
	}
	status, err := service.Storage(context.Background())
	if err != nil || status.Capacity != store.capacity || status.LastRollupCleanup == nil ||
		!reflect.DeepEqual(*status.LastRollupCleanup, run) {
		t.Fatalf("Storage() = %#v, %v", status, err)
	}
}

func TestServiceRejectsIntegerOverflow(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	store := &recordingQueryStore{trafficResponses: [][]storage.TrafficRollup{{
		{BucketStart: 100, BucketEnd: 110, UploadBytes: math.MaxInt64},
		{BucketStart: 110, BucketEnd: 120, UploadBytes: 1},
	}}}
	service := newService(t, store, now)
	_, err := service.Overview(context.Background(), rollup.Range{From: now.Add(-time.Hour).Unix(), To: now.Unix()})
	if !errors.Is(err, query.ErrQuery) {
		t.Fatalf("Overview() error = %v, want ErrQuery", err)
	}
}

type recordingQueryStore struct {
	trafficResponses [][]storage.TrafficRollup
	segmentCalls     [][]rollup.Segment
	atomicTraffic    []storage.TrafficRollup
	atomicFlows      []storage.FlowPoint
	atomicCalls      int
	flowSegmentCalls [][]rollup.Segment
	qualityEvents    []storage.QualityEventRecord
	capacity         storage.CapacityStatus
	maintenance      storage.MaintenanceRun
	hasMaintenance   bool
	flowResponses    [][]storage.FlowPoint
	sessions         []storage.RuntimeSession
	labels           []storage.ServiceLabel
	nextLabelID      int64
}

func (s *recordingQueryStore) BreakdownSeries(
	ctx context.Context,
	segments []rollup.Segment,
) ([]storage.TrafficRollup, []storage.FlowPoint, error) {
	s.atomicCalls++
	s.segmentCalls = append(s.segmentCalls, append([]rollup.Segment(nil), segments...))
	if s.atomicTraffic != nil || s.atomicFlows != nil {
		return append([]storage.TrafficRollup(nil), s.atomicTraffic...),
			append([]storage.FlowPoint(nil), s.atomicFlows...), nil
	}
	var traffic []storage.TrafficRollup
	if len(s.trafficResponses) > 0 {
		traffic = s.trafficResponses[0]
		s.trafficResponses = s.trafficResponses[1:]
	}
	var flows []storage.FlowPoint
	if len(s.flowResponses) > 0 {
		flows = s.flowResponses[0]
		s.flowResponses = s.flowResponses[1:]
	}
	return append([]storage.TrafficRollup(nil), traffic...), append([]storage.FlowPoint(nil), flows...), nil
}

func (s *recordingQueryStore) Labels(ctx context.Context) ([]storage.ServiceLabel, error) {
	return append([]storage.ServiceLabel(nil), s.labels...), nil
}

func (s *recordingQueryStore) CreateLabel(ctx context.Context, value storage.ServiceLabel) (storage.ServiceLabel, error) {
	s.nextLabelID++
	value.ID = s.nextLabelID
	s.labels = append(s.labels, value)
	return value, nil
}

func (s *recordingQueryStore) UpdateLabel(ctx context.Context, id int64, displayName string, updatedAt int64) (storage.ServiceLabel, error) {
	for index := range s.labels {
		if s.labels[index].ID == id {
			s.labels[index].DisplayName = displayName
			s.labels[index].UpdatedAt = updatedAt
			return s.labels[index], nil
		}
	}
	return storage.ServiceLabel{}, storage.ErrLabelNotFound
}

func (s *recordingQueryStore) DeleteLabel(ctx context.Context, id int64) (bool, error) {
	for index := range s.labels {
		if s.labels[index].ID == id {
			s.labels = append(s.labels[:index], s.labels[index+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (s *recordingQueryStore) FlowSeries(ctx context.Context, segments []rollup.Segment) ([]storage.FlowPoint, error) {
	s.flowSegmentCalls = append(s.flowSegmentCalls, append([]rollup.Segment(nil), segments...))
	if len(s.flowResponses) == 0 {
		return nil, nil
	}
	response := s.flowResponses[0]
	s.flowResponses = s.flowResponses[1:]
	return append([]storage.FlowPoint(nil), response...), nil
}

func (s *recordingQueryStore) RuntimeSessions(ctx context.Context, limit int) ([]storage.RuntimeSession, error) {
	return append([]storage.RuntimeSession(nil), s.sessions...), nil
}

func (s *recordingQueryStore) TrafficSeries(
	ctx context.Context,
	segments []rollup.Segment,
) ([]storage.TrafficRollup, error) {
	s.segmentCalls = append(s.segmentCalls, append([]rollup.Segment(nil), segments...))
	if len(s.trafficResponses) == 0 {
		return nil, nil
	}
	response := s.trafficResponses[0]
	s.trafficResponses = s.trafficResponses[1:]
	return append([]storage.TrafficRollup(nil), response...), nil
}

func (s *recordingQueryStore) QualityEvents(
	ctx context.Context,
	from int64,
	to int64,
) ([]storage.QualityEventRecord, error) {
	return append([]storage.QualityEventRecord(nil), s.qualityEvents...), nil
}

func (s *recordingQueryStore) CapacityStatus(ctx context.Context) (storage.CapacityStatus, error) {
	return s.capacity, nil
}

func (s *recordingQueryStore) LatestMaintenance(
	ctx context.Context,
	operation string,
) (storage.MaintenanceRun, bool, error) {
	return s.maintenance, s.hasMaintenance, nil
}

func newService(t *testing.T, store query.Store, now time.Time) *query.Service {
	return newServiceWith(t, store, fakeLiveSource{}, now, 20, attribution.SourcePrefix)
}

func newServiceWith(t *testing.T, store query.Store, live query.LiveSource, now time.Time, topK int, privacy attribution.SourceMode) *query.Service {
	t.Helper()
	service, err := query.NewService(query.Options{
		Store: store, Live: live, Now: func() time.Time { return now },
		Retention: config.Retention{TenSecondDays: 1, MinuteDays: 7, HalfHourDays: 365, HourDays: 1095, TopK: topK},
		Location:  time.UTC, PrivacyMode: privacy,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}
