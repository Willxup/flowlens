package rollup_test

import (
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/rollup"
)

func TestPlanSeriesKeepsRecentShortRangeAtTenSeconds(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	rangeValue := rollup.Range{From: now.Add(-time.Hour).Unix(), To: now.Unix()}

	segments, approximate, err := rollup.PlanSeries(rangeValue, now, defaultRetention(), time.UTC)
	if err != nil {
		t.Fatalf("PlanSeries() error = %v", err)
	}
	if approximate || len(segments) != 1 || segments[0] != (rollup.Segment{
		ResolutionSec: rollup.ResolutionTenSeconds,
		From:          rangeValue.From,
		To:            rangeValue.To,
	}) {
		t.Fatalf("PlanSeries() = %#v, approximate=%t", segments, approximate)
	}
}

func TestPlanSeriesCoarsensOneDayToPointLimit(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	rangeValue := rollup.Range{From: now.Add(-24 * time.Hour).Unix(), To: now.Unix()}

	segments, _, err := rollup.PlanSeries(rangeValue, now, defaultRetention(), time.UTC)
	if err != nil {
		t.Fatalf("PlanSeries() error = %v", err)
	}
	if len(segments) != 2 || segments[0].ResolutionSec != rollup.ResolutionMinute ||
		segments[1].ResolutionSec != rollup.ResolutionTenSeconds || segments[0].To != segments[1].From {
		t.Fatalf("PlanSeries() = %#v", segments)
	}
	if points := plannedPoints(segments); points > rollup.MaximumSeriesPoints {
		t.Fatalf("planned points = %d", points)
	}
}

func TestPlanSeriesPreservesRecentTenSecondTailWhenMinuteRollupIsNotStable(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 37, 0, time.UTC)
	rangeValue := rollup.Range{From: now.Add(-24 * time.Hour).Unix(), To: now.Unix()}

	segments, approximate, err := rollup.PlanSeries(rangeValue, now, defaultRetention(), time.UTC)
	if err != nil {
		t.Fatalf("PlanSeries() error = %v", err)
	}
	if !approximate || len(segments) < 2 {
		t.Fatalf("PlanSeries() = %#v, approximate=%t", segments, approximate)
	}
	last := segments[len(segments)-1]
	wantTailStart := now.Truncate(time.Minute).Add(-time.Minute).Unix()
	wantTailEnd := now.Add(3 * time.Second).Unix()
	if last.ResolutionSec != rollup.ResolutionTenSeconds || last.From != wantTailStart || last.To != wantTailEnd {
		t.Fatalf("recent tail = %#v, want ten-second [%d,%d)", last, wantTailStart, wantTailEnd)
	}
	if segments[len(segments)-2].To != last.From {
		t.Fatalf("segments overlap or gap: %#v", segments)
	}
}

func TestPlanSeriesBuildsNonOverlappingMixedResolutionTail(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 34, 56, 0, time.UTC)
	rangeValue := rollup.Range{From: now.AddDate(0, 0, -8).Unix(), To: now.Unix()}

	segments, approximate, err := rollup.PlanSeries(rangeValue, now, defaultRetention(), time.UTC)
	if err != nil {
		t.Fatalf("PlanSeries() error = %v", err)
	}
	if !approximate || len(segments) < 2 {
		t.Fatalf("PlanSeries() = %#v, approximate=%t", segments, approximate)
	}
	for index, segment := range segments {
		if segment.From >= segment.To {
			t.Fatalf("segment %d = %#v", index, segment)
		}
		if index > 0 && segments[index-1].To != segment.From {
			t.Fatalf("segments overlap or gap: %#v", segments)
		}
	}
	if points := plannedPoints(segments); points > rollup.MaximumSeriesPoints {
		t.Fatalf("planned points = %d, segments=%#v", points, segments)
	}
	if segments[0].ResolutionSec <= segments[len(segments)-1].ResolutionSec {
		t.Fatalf("oldest resolution is not coarser: %#v", segments)
	}
}

func TestPlanSeriesUsesNaturalDayBoundsForOldHistory(t *testing.T) {
	location := mustLocation(t, "America/New_York")
	now := time.Date(2026, time.November, 2, 12, 0, 0, 0, location)
	rangeValue := rollup.Range{
		From: now.AddDate(-4, 0, 0).Add(3 * time.Hour).Unix(),
		To:   now.AddDate(-3, -11, 0).Add(5 * time.Hour).Unix(),
	}

	segments, approximate, err := rollup.PlanSeries(rangeValue, now, defaultRetention(), location)
	if err != nil {
		t.Fatalf("PlanSeries() error = %v", err)
	}
	if !approximate || len(segments) == 0 || segments[0].ResolutionSec != rollup.ResolutionDay {
		t.Fatalf("PlanSeries() = %#v, approximate=%t", segments, approximate)
	}
	start := time.Unix(segments[0].From, 0).In(location)
	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Fatalf("daily start = %s", start)
	}
}

func TestPlanSeriesRejectsInvalidInput(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		rangeValue rollup.Range
		retention  config.Retention
		location   *time.Location
	}{
		{name: "empty range", rangeValue: rollup.Range{From: now.Unix(), To: now.Unix()}, retention: defaultRetention(), location: time.UTC},
		{name: "nonpositive start", rangeValue: rollup.Range{To: now.Unix()}, retention: defaultRetention(), location: time.UTC},
		{name: "invalid retention", rangeValue: rollup.Range{From: now.Add(-time.Hour).Unix(), To: now.Unix()}, retention: config.Retention{}, location: time.UTC},
		{name: "nil location", rangeValue: rollup.Range{From: now.Add(-time.Hour).Unix(), To: now.Unix()}, retention: defaultRetention()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := rollup.PlanSeries(test.rangeValue, now, test.retention, test.location); err == nil {
				t.Fatal("PlanSeries() error = nil")
			}
		})
	}
}

func defaultRetention() config.Retention {
	return config.Retention{TenSecondDays: 1, MinuteDays: 7, HalfHourDays: 365, HourDays: 1095, TopK: 20}
}

func plannedPoints(segments []rollup.Segment) int64 {
	var total int64
	for _, segment := range segments {
		total += (segment.To - segment.From + segment.ResolutionSec - 1) / segment.ResolutionSec
	}
	return total
}
