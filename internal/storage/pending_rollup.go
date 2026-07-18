package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Willxup/flowlens/internal/rollup"
)

// NextTrafficRollupWindow derives the oldest fully elapsed source window whose
// planned target bucket is not yet durable.
func (s *Store) NextTrafficRollupWindow(
	ctx context.Context,
	sourceResolutionSec int64,
	targetResolutionSec int64,
	completeBefore int64,
	location *time.Location,
) (rollup.Window, bool, error) {
	if err := ctx.Err(); err != nil {
		return rollup.Window{}, false, err
	}
	if completeBefore <= 0 || !validRollupResolutionEdge(sourceResolutionSec, targetResolutionSec) ||
		(targetResolutionSec == rollup.ResolutionDay && location == nil) {
		return rollup.Window{}, false, ErrInvalidRollup
	}
	state, found, err := loadCollectorStateFromQuery(ctx, s.db)
	if err != nil {
		return rollup.Window{}, false, err
	}
	if !found {
		return rollup.Window{}, false, nil
	}
	stableBefore := completeBefore
	if state.LastSampleAt < stableBefore {
		stableBefore = state.LastSampleAt
	}
	if targetResolutionSec == rollup.ResolutionDay {
		return s.nextDailyRollupWindow(ctx, sourceResolutionSec, stableBefore, location)
	}
	return s.nextFixedRollupWindow(ctx, sourceResolutionSec, targetResolutionSec, stableBefore)
}

func (s *Store) nextFixedRollupWindow(
	ctx context.Context,
	sourceResolutionSec int64,
	targetResolutionSec int64,
	completeBefore int64,
) (rollup.Window, bool, error) {
	var sourceBucketStart int64
	err := s.db.QueryRowContext(ctx, `
		SELECT source.bucket_start
		FROM traffic_rollup AS source
		WHERE source.resolution_sec = ?
			AND source.bucket_end <= ?
			AND NOT EXISTS (
				SELECT 1
				FROM traffic_rollup AS target
				WHERE target.resolution_sec = ?
					AND target.bucket_start = source.bucket_start - (source.bucket_start % ?)
			)
		ORDER BY source.bucket_start
		LIMIT 1
	`, sourceResolutionSec, completeBefore, targetResolutionSec, targetResolutionSec).Scan(&sourceBucketStart)
	if errors.Is(err, sql.ErrNoRows) {
		return rollup.Window{}, false, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return rollup.Window{}, false, ctx.Err()
		}
		return rollup.Window{}, false, errors.New("cannot derive FlowLens pending rollup")
	}
	window, err := rollup.WindowAt(time.Unix(sourceBucketStart, 0), targetResolutionSec, time.UTC)
	if err != nil {
		return rollup.Window{}, false, ErrInvalidRollup
	}
	if window.BucketEnd > completeBefore {
		return rollup.Window{}, false, nil
	}
	return window, true, nil
}

func (s *Store) nextDailyRollupWindow(
	ctx context.Context,
	sourceResolutionSec int64,
	completeBefore int64,
	location *time.Location,
) (rollup.Window, bool, error) {
	cursor := int64(1)
	for {
		if err := ctx.Err(); err != nil {
			return rollup.Window{}, false, err
		}
		var sourceBucketStart int64
		err := s.db.QueryRowContext(ctx, `
			SELECT bucket_start
			FROM traffic_rollup
			WHERE resolution_sec = ? AND bucket_start >= ? AND bucket_end <= ?
			ORDER BY bucket_start
			LIMIT 1
		`, sourceResolutionSec, cursor, completeBefore).Scan(&sourceBucketStart)
		if errors.Is(err, sql.ErrNoRows) {
			return rollup.Window{}, false, nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return rollup.Window{}, false, ctx.Err()
			}
			return rollup.Window{}, false, errors.New("cannot derive FlowLens pending daily rollup")
		}
		window, err := rollup.WindowAt(time.Unix(sourceBucketStart, 0), rollup.ResolutionDay, location)
		if err != nil || !validRollupEdge(sourceResolutionSec, window) {
			return rollup.Window{}, false, ErrInvalidRollup
		}
		if window.BucketEnd > completeBefore {
			return rollup.Window{}, false, nil
		}
		var exists int
		if err := s.db.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM traffic_rollup
				WHERE resolution_sec = ? AND bucket_start = ?
			)
		`, rollup.ResolutionDay, window.BucketStart).Scan(&exists); err != nil {
			if ctx.Err() != nil {
				return rollup.Window{}, false, ctx.Err()
			}
			return rollup.Window{}, false, errors.New("cannot inspect FlowLens daily rollup")
		}
		if exists == 0 {
			return window, true, nil
		}
		cursor = window.BucketEnd
	}
}
