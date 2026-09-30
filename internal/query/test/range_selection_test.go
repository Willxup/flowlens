package query_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
)

func TestServiceResolveRangeUsesServerClockForPresets(t *testing.T) {
	now := time.Date(2026, time.July, 29, 0, 20, 0, 0, time.UTC)
	service := newService(t, &recordingQueryStore{}, now)
	tests := []struct {
		name      string
		selection query.RangeSelection
		want      rollup.Range
	}{
		{
			name:      "today",
			selection: query.RangeSelection{Kind: query.RangeToday},
			want:      rollup.Range{From: time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC).Unix(), To: now.Unix()},
		},
		{
			name:      "yesterday",
			selection: query.RangeSelection{Kind: query.RangeYesterday},
			want: rollup.Range{
				From: time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC).Unix(),
				To:   time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC).Unix(),
			},
		},
		{name: "7d", selection: query.RangeSelection{Kind: query.RangeSevenDays}, want: rollup.Range{From: now.Add(-7 * 24 * time.Hour).Unix(), To: now.Unix()}},
		{name: "30d", selection: query.RangeSelection{Kind: query.RangeThirtyDays}, want: rollup.Range{From: now.Add(-30 * 24 * time.Hour).Unix(), To: now.Unix()}},
		{name: "90d", selection: query.RangeSelection{Kind: query.RangeNinetyDays}, want: rollup.Range{From: now.Add(-90 * 24 * time.Hour).Unix(), To: now.Unix()}},
		{
			name:      "year",
			selection: query.RangeSelection{Kind: query.RangeYear},
			want:      rollup.Range{From: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).Unix(), To: now.Unix()},
		},
		{name: "lifetime", selection: query.RangeSelection{Kind: query.RangeLifetime}, want: rollup.Range{From: 86_400, To: now.Unix()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := service.ResolveRange(test.selection)
			if err != nil || got != test.want {
				t.Fatalf("ResolveRange(%#v) = %#v, %v, want %#v", test.selection, got, err, test.want)
			}
		})
	}
}

func TestServiceResolveRangeUsesConfiguredTimezoneForCustomDates(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	now := time.Date(2026, time.July, 29, 0, 20, 0, 0, location)
	service := newServiceAtLocation(t, &recordingQueryStore{}, now, location)

	historical, err := service.ResolveRange(query.RangeSelection{
		Kind: query.RangeCustom, FromDate: "2026-07-01", ToDate: "2026-07-14",
	})
	if err != nil || historical != (rollup.Range{
		From: time.Date(2026, time.July, 1, 0, 0, 0, 0, location).Unix(),
		To:   time.Date(2026, time.July, 15, 0, 0, 0, 0, location).Unix(),
	}) {
		t.Fatalf("historical custom range = %#v, %v", historical, err)
	}

	current, err := service.ResolveRange(query.RangeSelection{
		Kind: query.RangeCustom, FromDate: "2026-07-29", ToDate: "2026-07-29",
	})
	if err != nil || current != (rollup.Range{
		From: time.Date(2026, time.July, 29, 0, 0, 0, 0, location).Unix(), To: now.Unix(),
	}) {
		t.Fatalf("current custom range = %#v, %v", current, err)
	}
}

func TestServiceResolveRangeUsesConfiguredTimezoneForCalendarPresets(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	now := time.Date(2026, time.July, 29, 15, 20, 0, 0, location)
	service := newServiceAtLocation(t, &recordingQueryStore{}, now, location)
	tests := []struct {
		name      string
		selection query.RangeSelection
		want      rollup.Range
	}{
		{
			name:      "today",
			selection: query.RangeSelection{Kind: query.RangeToday},
			want: rollup.Range{
				From: time.Date(2026, time.July, 29, 0, 0, 0, 0, location).Unix(),
				To:   now.Unix(),
			},
		},
		{
			name:      "yesterday",
			selection: query.RangeSelection{Kind: query.RangeYesterday},
			want: rollup.Range{
				From: time.Date(2026, time.July, 28, 0, 0, 0, 0, location).Unix(),
				To:   time.Date(2026, time.July, 29, 0, 0, 0, 0, location).Unix(),
			},
		},
		{
			name:      "year",
			selection: query.RangeSelection{Kind: query.RangeYear},
			want: rollup.Range{
				From: time.Date(2026, time.January, 1, 0, 0, 0, 0, location).Unix(),
				To:   now.Unix(),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := service.ResolveRange(test.selection)
			if err != nil || got != test.want {
				t.Fatalf("ResolveRange(%#v) = %#v, %v, want %#v", test.selection, got, err, test.want)
			}
		})
	}
}

func TestServiceResolveRangeRejectsInvalidSelections(t *testing.T) {
	now := time.Date(2026, time.July, 29, 0, 20, 0, 0, time.UTC)
	service := newService(t, &recordingQueryStore{}, now)
	for _, selection := range []query.RangeSelection{
		{Kind: "tomorrow"},
		{Kind: query.RangeToday, FromDate: "2026-07-01"},
		{Kind: query.RangeCustom, FromDate: "2026-02-30", ToDate: "2026-03-01"},
		{Kind: query.RangeCustom, FromDate: "2026-07-20", ToDate: "2026-07-19"},
		{Kind: query.RangeCustom, FromDate: "2026-07-29", ToDate: "2026-07-30"},
	} {
		if _, err := service.ResolveRange(selection); !errors.Is(err, query.ErrRangeSelection) {
			t.Errorf("ResolveRange(%#v) error = %v, want ErrRangeSelection", selection, err)
		}
	}
}
