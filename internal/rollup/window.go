package rollup

import (
	"errors"
	"time"
)

const (
	ResolutionTenSeconds int64 = 10
	ResolutionMinute     int64 = 60
	ResolutionHalfHour   int64 = 30 * 60
	ResolutionHour       int64 = 60 * 60
	ResolutionDay        int64 = 24 * 60 * 60
)

var ErrInvalidWindow = errors.New("invalid FlowLens rollup window")

// Window is one exact half-open rollup bucket.
type Window struct {
	ResolutionSec int64
	BucketStart   int64
	BucketEnd     int64
}

// WindowAt returns the exact bucket containing at. Fixed-duration buckets are
// UTC-aligned; daily buckets follow calendar midnights in location.
func WindowAt(at time.Time, resolutionSec int64, location *time.Location) (Window, error) {
	if resolutionSec == ResolutionDay {
		if location == nil {
			return Window{}, ErrInvalidWindow
		}
		local := at.In(location)
		year, month, day := local.Date()
		start := time.Date(year, month, day, 0, 0, 0, 0, location)
		end := start.AddDate(0, 0, 1)
		return Window{ResolutionSec: resolutionSec, BucketStart: start.Unix(), BucketEnd: end.Unix()}, nil
	}

	switch resolutionSec {
	case ResolutionTenSeconds, ResolutionMinute, ResolutionHalfHour, ResolutionHour:
	default:
		return Window{}, ErrInvalidWindow
	}
	seconds := at.Unix()
	start := floorTo(seconds, resolutionSec)
	return Window{ResolutionSec: resolutionSec, BucketStart: start, BucketEnd: start + resolutionSec}, nil
}

func floorTo(value, multiple int64) int64 {
	remainder := value % multiple
	if remainder < 0 {
		remainder += multiple
	}
	return value - remainder
}
