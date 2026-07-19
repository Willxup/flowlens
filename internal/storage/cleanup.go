package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Willxup/flowlens/internal/rollup"
)

var ErrInvalidRetention = errors.New("invalid FlowLens retention request")

// RetentionCutoffs contains exclusive source-bucket end cutoffs. Zero disables
// cleanup for that resolution in the current run.
type RetentionCutoffs struct {
	TenSecondBefore int64
	MinuteBefore    int64
	HalfHourBefore  int64
	HourBefore      int64
}

// CleanupResult reports deleted global source rows by resolution.
type CleanupResult struct {
	DeletedTenSecond int64
	DeletedMinute    int64
	DeletedHalfHour  int64
	DeletedHour      int64
}

// CleanupTraffic deletes eligible source rows only after every required target
// is durable. Daily rows and dimension dictionary rows are never deleted.
func (s *Store) CleanupTraffic(
	ctx context.Context,
	cutoffs RetentionCutoffs,
	location *time.Location,
) (CleanupResult, error) {
	if location == nil || cutoffs.TenSecondBefore < 0 || cutoffs.MinuteBefore < 0 ||
		cutoffs.HalfHourBefore < 0 || cutoffs.HourBefore < 0 {
		return CleanupResult{}, ErrInvalidRetention
	}
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		if ctx.Err() != nil {
			return CleanupResult{}, ctx.Err()
		}
		return CleanupResult{}, errors.New("cannot begin FlowLens cleanup transaction")
	}
	defer transaction.Rollback()

	var result CleanupResult
	result.DeletedTenSecond, err = deleteFixedRollupSources(
		ctx, transaction, rollup.ResolutionTenSeconds, rollup.ResolutionMinute, cutoffs.TenSecondBefore,
	)
	if err != nil {
		return CleanupResult{}, err
	}
	result.DeletedMinute, err = deleteCalendarGuardedSources(
		ctx, transaction, rollup.ResolutionMinute, rollup.ResolutionHalfHour, cutoffs.MinuteBefore, location,
	)
	if err != nil {
		return CleanupResult{}, err
	}
	result.DeletedHalfHour, err = deleteFixedRollupSources(
		ctx, transaction, rollup.ResolutionHalfHour, rollup.ResolutionHour, cutoffs.HalfHourBefore,
	)
	if err != nil {
		return CleanupResult{}, err
	}
	result.DeletedHour, err = deleteCalendarGuardedSources(
		ctx, transaction, rollup.ResolutionHour, 0, cutoffs.HourBefore, location,
	)
	if err != nil {
		return CleanupResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return CleanupResult{}, errors.New("cannot commit FlowLens cleanup transaction")
	}
	return result, nil
}

func deleteFixedRollupSources(
	ctx context.Context,
	transaction *sql.Tx,
	sourceResolutionSec int64,
	targetResolutionSec int64,
	cutoff int64,
) (int64, error) {
	if cutoff == 0 {
		return 0, nil
	}
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM flow_rollup
		WHERE resolution_sec = ? AND bucket_start IN (
			SELECT source.bucket_start
			FROM traffic_rollup AS source
			WHERE source.resolution_sec = ? AND source.bucket_end <= ?
				AND EXISTS (
					SELECT 1
					FROM traffic_rollup AS target
					WHERE target.resolution_sec = ?
						AND target.bucket_start = source.bucket_start - (source.bucket_start % ?)
						AND (SELECT COALESCE(SUM(f.upload_bytes), 0) FROM flow_rollup AS f
							WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.upload_bytes
						AND (SELECT COALESCE(SUM(f.download_bytes), 0) FROM flow_rollup AS f
							WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.download_bytes
						AND (SELECT COALESCE(SUM(CASE WHEN d.classification_code = 3 THEN f.upload_bytes ELSE 0 END), 0)
							FROM flow_rollup AS f JOIN flow_dimension AS d ON d.id = f.dimension_id
							WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.unattributed_upload_bytes
						AND (SELECT COALESCE(SUM(CASE WHEN d.classification_code = 3 THEN f.download_bytes ELSE 0 END), 0)
							FROM flow_rollup AS f JOIN flow_dimension AS d ON d.id = f.dimension_id
							WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.unattributed_download_bytes
				)
		)
	`,
		sourceResolutionSec,
		sourceResolutionSec,
		cutoff,
		targetResolutionSec,
		targetResolutionSec,
	); err != nil {
		return 0, errors.New("cannot delete FlowLens dimensional source rollups")
	}
	result, err := transaction.ExecContext(ctx, `
		DELETE FROM traffic_rollup
		WHERE resolution_sec = ? AND bucket_end <= ?
			AND EXISTS (
				SELECT 1
				FROM traffic_rollup AS target
				WHERE target.resolution_sec = ?
					AND target.bucket_start = traffic_rollup.bucket_start - (traffic_rollup.bucket_start % ?)
					AND (SELECT COALESCE(SUM(f.upload_bytes), 0) FROM flow_rollup AS f
						WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.upload_bytes
					AND (SELECT COALESCE(SUM(f.download_bytes), 0) FROM flow_rollup AS f
						WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.download_bytes
					AND (SELECT COALESCE(SUM(CASE WHEN d.classification_code = 3 THEN f.upload_bytes ELSE 0 END), 0)
						FROM flow_rollup AS f JOIN flow_dimension AS d ON d.id = f.dimension_id
						WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.unattributed_upload_bytes
					AND (SELECT COALESCE(SUM(CASE WHEN d.classification_code = 3 THEN f.download_bytes ELSE 0 END), 0)
						FROM flow_rollup AS f JOIN flow_dimension AS d ON d.id = f.dimension_id
						WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.unattributed_download_bytes
			)
	`, sourceResolutionSec, cutoff, targetResolutionSec, targetResolutionSec)
	if err != nil {
		return 0, errors.New("cannot delete FlowLens global source rollups")
	}
	return affectedRows(result)
}

func deleteCalendarGuardedSources(
	ctx context.Context,
	transaction *sql.Tx,
	sourceResolutionSec int64,
	fixedTargetResolutionSec int64,
	cutoff int64,
	location *time.Location,
) (int64, error) {
	if cutoff == 0 {
		return 0, nil
	}
	rows, err := transaction.QueryContext(ctx, `
		SELECT bucket_start
		FROM traffic_rollup
		WHERE resolution_sec = ? AND bucket_end <= ?
		ORDER BY bucket_start
	`, sourceResolutionSec, cutoff)
	if err != nil {
		return 0, errors.New("cannot read FlowLens cleanup candidates")
	}
	var bucketStarts []int64
	for rows.Next() {
		var bucketStart int64
		if err := rows.Scan(&bucketStart); err != nil {
			_ = rows.Close()
			return 0, errors.New("cannot read FlowLens cleanup candidate")
		}
		bucketStarts = append(bucketStarts, bucketStart)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, errors.New("cannot iterate FlowLens cleanup candidates")
	}
	if err := rows.Close(); err != nil {
		return 0, errors.New("cannot close FlowLens cleanup candidates")
	}

	var deleted int64
	for _, bucketStart := range bucketStarts {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if fixedTargetResolutionSec != 0 {
			fixedStart := bucketStart - bucketStart%fixedTargetResolutionSec
			exists, err := trafficRollupConserves(ctx, transaction, fixedTargetResolutionSec, fixedStart)
			if err != nil {
				return 0, err
			}
			if !exists {
				continue
			}
		}
		day, err := rollup.WindowAt(time.Unix(bucketStart, 0), rollup.ResolutionDay, location)
		if err != nil {
			return 0, ErrInvalidRetention
		}
		exists, err := trafficRollupConserves(ctx, transaction, rollup.ResolutionDay, day.BucketStart)
		if err != nil {
			return 0, err
		}
		if !exists {
			continue
		}
		if _, err := transaction.ExecContext(ctx,
			`DELETE FROM flow_rollup WHERE resolution_sec = ? AND bucket_start = ?`,
			sourceResolutionSec, bucketStart,
		); err != nil {
			return 0, errors.New("cannot delete FlowLens dimensional source rollup")
		}
		result, err := transaction.ExecContext(ctx,
			`DELETE FROM traffic_rollup WHERE resolution_sec = ? AND bucket_start = ?`,
			sourceResolutionSec, bucketStart,
		)
		if err != nil {
			return 0, errors.New("cannot delete FlowLens global source rollup")
		}
		count, err := affectedRows(result)
		if err != nil {
			return 0, err
		}
		deleted += count
	}
	return deleted, nil
}

func trafficRollupExists(
	ctx context.Context,
	queryer rowQuerier,
	resolutionSec int64,
	bucketStart int64,
) (bool, error) {
	var exists int
	if err := queryer.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM traffic_rollup
			WHERE resolution_sec = ? AND bucket_start = ?
		)
	`, resolutionSec, bucketStart).Scan(&exists); err != nil {
		return false, errors.New("cannot inspect FlowLens cleanup target")
	}
	return exists != 0, nil
}

func trafficRollupConserves(
	ctx context.Context,
	queryer rowQuerier,
	resolutionSec int64,
	bucketStart int64,
) (bool, error) {
	var conserving int
	if err := queryer.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM traffic_rollup AS target
			WHERE target.resolution_sec = ? AND target.bucket_start = ?
				AND (SELECT COALESCE(SUM(f.upload_bytes), 0) FROM flow_rollup AS f
					WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.upload_bytes
				AND (SELECT COALESCE(SUM(f.download_bytes), 0) FROM flow_rollup AS f
					WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.download_bytes
				AND (SELECT COALESCE(SUM(CASE WHEN d.classification_code = 3 THEN f.upload_bytes ELSE 0 END), 0)
					FROM flow_rollup AS f JOIN flow_dimension AS d ON d.id = f.dimension_id
					WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.unattributed_upload_bytes
				AND (SELECT COALESCE(SUM(CASE WHEN d.classification_code = 3 THEN f.download_bytes ELSE 0 END), 0)
					FROM flow_rollup AS f JOIN flow_dimension AS d ON d.id = f.dimension_id
					WHERE f.resolution_sec = target.resolution_sec AND f.bucket_start = target.bucket_start) = target.unattributed_download_bytes
		)
	`, resolutionSec, bucketStart).Scan(&conserving); err != nil {
		return false, errors.New("cannot inspect FlowLens cleanup target conservation")
	}
	return conserving != 0, nil
}

func affectedRows(result sql.Result) (int64, error) {
	if result == nil {
		return 0, errors.New("cannot count FlowLens cleanup rows")
	}
	count, err := result.RowsAffected()
	if err != nil || count < 0 {
		return 0, errors.New("cannot count FlowLens cleanup rows")
	}
	return count, nil
}
