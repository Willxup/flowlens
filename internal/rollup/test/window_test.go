package rollup_test

import (
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/rollup"
)

func TestWindowAtAlignsFixedResolutionBucketsInUTC(t *testing.T) {
	at := time.Date(2026, time.July, 18, 12, 34, 56, 0, time.UTC)
	tests := []struct {
		name       string
		resolution int64
		start      time.Time
		end        time.Time
	}{
		{
			name:       "ten seconds",
			resolution: rollup.ResolutionTenSeconds,
			start:      time.Date(2026, time.July, 18, 12, 34, 50, 0, time.UTC),
			end:        time.Date(2026, time.July, 18, 12, 35, 0, 0, time.UTC),
		},
		{
			name:       "minute",
			resolution: rollup.ResolutionMinute,
			start:      time.Date(2026, time.July, 18, 12, 34, 0, 0, time.UTC),
			end:        time.Date(2026, time.July, 18, 12, 35, 0, 0, time.UTC),
		},
		{
			name:       "half hour",
			resolution: rollup.ResolutionHalfHour,
			start:      time.Date(2026, time.July, 18, 12, 30, 0, 0, time.UTC),
			end:        time.Date(2026, time.July, 18, 13, 0, 0, 0, time.UTC),
		},
		{
			name:       "hour",
			resolution: rollup.ResolutionHour,
			start:      time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC),
			end:        time.Date(2026, time.July, 18, 13, 0, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			window, err := rollup.WindowAt(at, test.resolution, time.UTC)
			if err != nil {
				t.Fatalf("WindowAt() error = %v", err)
			}
			if window.ResolutionSec != test.resolution ||
				window.BucketStart != test.start.Unix() || window.BucketEnd != test.end.Unix() {
				t.Fatalf("WindowAt() = %#v", window)
			}
		})
	}
}

func TestWindowAtUsesConfiguredNaturalDayInNonHourTimezone(t *testing.T) {
	location := mustLocation(t, "Asia/Kathmandu")
	at := time.Date(2026, time.July, 18, 12, 34, 56, 0, location)

	window, err := rollup.WindowAt(at, rollup.ResolutionDay, location)
	if err != nil {
		t.Fatalf("WindowAt() error = %v", err)
	}
	wantStart := time.Date(2026, time.July, 18, 0, 0, 0, 0, location).Unix()
	wantEnd := time.Date(2026, time.July, 19, 0, 0, 0, 0, location).Unix()
	if window.BucketStart != wantStart || window.BucketEnd != wantEnd {
		t.Fatalf("WindowAt() = %#v, want [%d,%d)", window, wantStart, wantEnd)
	}
}

func TestWindowAtPreservesDSTNaturalDayLength(t *testing.T) {
	location := mustLocation(t, "America/New_York")
	tests := []struct {
		name string
		at   time.Time
		want time.Duration
	}{
		{
			name: "spring forward",
			at:   time.Date(2026, time.March, 8, 12, 0, 0, 0, location),
			want: 23 * time.Hour,
		},
		{
			name: "fall back",
			at:   time.Date(2026, time.November, 1, 12, 0, 0, 0, location),
			want: 25 * time.Hour,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			window, err := rollup.WindowAt(test.at, rollup.ResolutionDay, location)
			if err != nil {
				t.Fatalf("WindowAt() error = %v", err)
			}
			if duration := time.Duration(window.BucketEnd-window.BucketStart) * time.Second; duration != test.want {
				t.Fatalf("natural day length = %s, want %s", duration, test.want)
			}
		})
	}
}

func TestWindowAtRejectsUnsupportedResolutionAndMissingTimezone(t *testing.T) {
	at := time.Date(2026, time.July, 18, 12, 34, 56, 0, time.UTC)
	if _, err := rollup.WindowAt(at, 120, time.UTC); err == nil {
		t.Fatal("WindowAt() accepted unsupported resolution")
	}
	if _, err := rollup.WindowAt(at, rollup.ResolutionDay, nil); err == nil {
		t.Fatal("WindowAt() accepted nil timezone")
	}
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q) error = %v", name, err)
	}
	return location
}
