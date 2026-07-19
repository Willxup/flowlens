package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Willxup/flowlens/internal/rollup"
)

var ErrInvalidQuery = errors.New("invalid FlowLens storage query")

type seriesQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// TrafficSeries reads exact non-overlapping segments in chronological order.
func (s *Store) TrafficSeries(
	ctx context.Context,
	segments []rollup.Segment,
) ([]TrafficRollup, error) {
	if !validSeriesSegments(segments) {
		return nil, ErrInvalidQuery
	}
	return trafficSeries(ctx, s.db, segments)
}

// BreakdownSeries reads global and dimensional points from one SQLite
// snapshot so a concurrent collector or maintenance commit cannot split them.
func (s *Store) BreakdownSeries(
	ctx context.Context,
	segments []rollup.Segment,
) ([]TrafficRollup, []FlowPoint, error) {
	if !validSeriesSegments(segments) {
		return nil, nil, ErrInvalidQuery
	}
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, errors.New("cannot begin FlowLens breakdown transaction")
	}
	defer transaction.Rollback()
	traffic, err := trafficSeries(ctx, transaction, segments)
	if err != nil {
		return nil, nil, err
	}
	flows, err := flowSeries(ctx, transaction, segments)
	if err != nil {
		return nil, nil, err
	}
	if err := transaction.Commit(); err != nil {
		return nil, nil, errors.New("cannot commit FlowLens breakdown transaction")
	}
	return traffic, flows, nil
}

func trafficSeries(
	ctx context.Context,
	queryer seriesQuerier,
	segments []rollup.Segment,
) ([]TrafficRollup, error) {
	var result []TrafficRollup
	for _, segment := range segments {
		rows, err := queryer.QueryContext(ctx, `
			SELECT
				resolution_sec, bucket_start, bucket_end,
				upload_bytes, download_bytes,
				recovered_upload_bytes, recovered_download_bytes,
				speed_upload_sample_sum, speed_download_sample_sum, speed_sample_count,
				peak_upload_bytes_per_second, peak_download_bytes_per_second,
				peak_upload_at, peak_download_at,
				counter_observed_seconds, attribution_observed_seconds,
				active_connections_sum, active_connections_samples, active_connections_max,
				memory_bytes_sum, memory_samples, memory_bytes_max,
				unattributed_upload_bytes, unattributed_download_bytes,
				reset_count, quality_flags
			FROM traffic_rollup
			WHERE resolution_sec = ? AND bucket_start >= ? AND bucket_end <= ?
			ORDER BY bucket_start
		`, segment.ResolutionSec, segment.From, segment.To)
		if err != nil {
			return nil, errors.New("cannot query FlowLens traffic series")
		}
		for rows.Next() {
			value, err := scanTrafficRollup(rows)
			if err != nil {
				_ = rows.Close()
				return nil, errors.New("cannot read FlowLens traffic series")
			}
			if value.ResolutionSec != segment.ResolutionSec || value.BucketStart < segment.From || value.BucketEnd > segment.To {
				_ = rows.Close()
				return nil, ErrInvalidQuery
			}
			if len(result) > 0 && result[len(result)-1].BucketEnd > value.BucketStart {
				_ = rows.Close()
				return nil, ErrInvalidQuery
			}
			result = append(result, value)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, errors.New("cannot iterate FlowLens traffic series")
		}
		if err := rows.Close(); err != nil {
			return nil, errors.New("cannot close FlowLens traffic series")
		}
	}
	return result, nil
}

func validSeriesSegments(segments []rollup.Segment) bool {
	if len(segments) == 0 || len(segments) > 5 {
		return false
	}
	for index, segment := range segments {
		if segment.From <= 0 || segment.To <= segment.From || !validStoredResolution(segment.ResolutionSec) {
			return false
		}
		if index > 0 && segments[index-1].To > segment.From {
			return false
		}
	}
	return true
}

func validStoredResolution(resolutionSec int64) bool {
	switch resolutionSec {
	case rollup.ResolutionTenSeconds, rollup.ResolutionMinute, rollup.ResolutionHalfHour,
		rollup.ResolutionHour, rollup.ResolutionDay:
		return true
	default:
		return false
	}
}
