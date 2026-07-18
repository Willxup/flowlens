package storage

import (
	"context"
	"database/sql"
	"errors"
)

var (
	// ErrStateConflict means the durable cumulative cursor differs from the
	// caller's expected old totals.
	ErrStateConflict = errors.New("FlowLens collector state conflict")
	// ErrTimezoneMismatch prevents reinterpretation of permanent daily buckets.
	ErrTimezoneMismatch = errors.New("FlowLens bucket timezone does not match stored state")
)

// CommitBatch atomically writes one complete 10-second storage transition.
func (s *Store) CommitBatch(ctx context.Context, batch Batch) (CommitResult, error) {
	if err := validateBatch(batch); err != nil {
		return CommitResult{}, err
	}
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CommitResult{}, errors.New("cannot begin FlowLens batch transaction")
	}
	defer transaction.Rollback()

	state, hasState, err := loadCollectorStateFromQuery(ctx, transaction)
	if err != nil {
		return CommitResult{}, err
	}
	if hasState && state.BucketTimezone != batch.NewState.BucketTimezone {
		return CommitResult{}, ErrTimezoneMismatch
	}
	if hasState && state.LastBatchID == batch.BatchID {
		return CommitResult{AlreadyCommitted: true}, nil
	}
	if (batch.ExpectedOldTotals == nil) != !hasState {
		return CommitResult{}, ErrStateConflict
	}
	if hasState && state.LastTotals != *batch.ExpectedOldTotals {
		return CommitResult{}, ErrStateConflict
	}
	capacityBefore, capacityAfter, err := s.beginCapacityTransition()
	if err != nil {
		return CommitResult{}, err
	}
	defer s.capacityMu.Unlock()
	flows := batch.Flows
	if capacityAfter {
		flows, err = applyCapacityPolicy(ctx, transaction, batch.Global, flows)
		if err != nil {
			return CommitResult{}, err
		}
	}
	qualityEvents := batch.QualityEvents
	if capacityBefore.Protecting && !capacityAfter {
		qualityEvents = append(append([]QualityEvent(nil), qualityEvents...), QualityEvent{
			Code: "storage_capacity_recovered", StartedAt: batch.Global.BucketStart,
		})
	}

	if batch.EndRuntimeSession != nil {
		result, err := transaction.ExecContext(ctx, `
			UPDATE runtime_session
			SET ended_at = ?, end_reason = ?
			WHERE id = ? AND ended_at IS NULL
		`, batch.EndRuntimeSession.EndedAt, batch.EndRuntimeSession.EndReason, batch.EndRuntimeSession.ID)
		if err != nil || !exactlyOneRow(result) {
			return CommitResult{}, errors.New("cannot end FlowLens runtime session")
		}
	}
	if batch.NewRuntimeSession != nil {
		start := batch.NewRuntimeSession
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO runtime_session (
				id, started_at, ended_at, start_reason, end_reason,
				last_upload_total, last_download_total, last_seen_at,
				sing_box_version, host_boot_id, data_gap_before_seconds
			) VALUES (?, ?, NULL, ?, NULL, ?, ?, ?, ?, ?, ?)
		`,
			start.ID,
			start.StartedAt,
			start.StartReason,
			batch.NewState.LastTotals.Upload,
			batch.NewState.LastTotals.Download,
			batch.NewState.LastSampleAt,
			start.SingBoxVersion,
			start.HostBootID,
			start.DataGapBeforeSeconds,
		); err != nil {
			return CommitResult{}, errors.New("cannot create FlowLens runtime session")
		}
	}
	if err := upsertTrafficRollup(ctx, transaction, batch.Global); err != nil {
		return CommitResult{}, err
	}
	if err := replaceFlowRollups(ctx, transaction, batch.Global, flows); err != nil {
		return CommitResult{}, err
	}
	for _, event := range qualityEvents {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO quality_event(batch_id, code, started_at, ended_at, flags, detail)
			VALUES (?, ?, ?, ?, ?, ?)
		`, batch.BatchID, event.Code, event.StartedAt, event.EndedAt, event.Flags, event.Detail); err != nil {
			return CommitResult{}, errors.New("cannot write FlowLens quality event")
		}
	}

	result, err := transaction.ExecContext(ctx, `
		UPDATE runtime_session
		SET last_upload_total = ?, last_download_total = ?, last_seen_at = ?
		WHERE id = ? AND ended_at IS NULL
	`,
		batch.NewState.LastTotals.Upload,
		batch.NewState.LastTotals.Download,
		batch.NewState.LastSampleAt,
		batch.NewState.RuntimeSessionID,
	)
	if err != nil || !exactlyOneRow(result) {
		return CommitResult{}, errors.New("cannot update FlowLens runtime session")
	}
	if hasState {
		result, err = transaction.ExecContext(ctx, `
			UPDATE collector_state
			SET runtime_session_id = ?, last_upload_total = ?, last_download_total = ?,
				last_sample_at = ?, last_batch_id = ?, bucket_timezone = ?
			WHERE id = 1
		`,
			batch.NewState.RuntimeSessionID,
			batch.NewState.LastTotals.Upload,
			batch.NewState.LastTotals.Download,
			batch.NewState.LastSampleAt,
			batch.BatchID,
			batch.NewState.BucketTimezone,
		)
		if err != nil || !exactlyOneRow(result) {
			return CommitResult{}, errors.New("cannot update FlowLens collector state")
		}
	} else {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO collector_state (
				id, runtime_session_id, last_upload_total, last_download_total,
				last_sample_at, last_batch_id, bucket_timezone
			) VALUES (1, ?, ?, ?, ?, ?, ?)
		`,
			batch.NewState.RuntimeSessionID,
			batch.NewState.LastTotals.Upload,
			batch.NewState.LastTotals.Download,
			batch.NewState.LastSampleAt,
			batch.BatchID,
			batch.NewState.BucketTimezone,
		); err != nil {
			return CommitResult{}, errors.New("cannot create FlowLens collector state")
		}
	}
	if err := transaction.Commit(); err != nil {
		return CommitResult{}, errors.New("cannot commit FlowLens storage batch")
	}
	s.protecting = capacityAfter
	return CommitResult{}, nil
}

func applyCapacityPolicy(
	ctx context.Context,
	transaction *sql.Tx,
	global TrafficRollup,
	flows []FlowRollup,
) ([]FlowRollup, error) {
	transformed := make([]FlowRollup, 0, len(flows))
	var redirected FlowRollup
	redirected.Dimension = FlowDimension{
		SourceNetwork: []byte{}, DestinationIP: []byte{}, DestinationPort: -1, ClassificationCode: 2,
	}
	for _, flow := range flows {
		if flow.Dimension.ClassificationCode != 1 {
			transformed = append(transformed, flow)
			continue
		}
		exists, err := concreteDimensionExists(ctx, transaction, flow.Dimension)
		if err != nil {
			return nil, err
		}
		if exists {
			transformed = append(transformed, flow)
			continue
		}
		if !safeAdd(&redirected.UploadBytes, flow.UploadBytes) ||
			!safeAdd(&redirected.DownloadBytes, flow.DownloadBytes) ||
			!safeAdd(&redirected.FlowObservationCount, flow.FlowObservationCount) {
			return nil, ErrInvalidBatch
		}
	}
	if redirected.UploadBytes != 0 || redirected.DownloadBytes != 0 || redirected.FlowObservationCount != 0 {
		otherIndex := -1
		for index := range transformed {
			if transformed[index].Dimension.ClassificationCode == 2 {
				otherIndex = index
				break
			}
		}
		if otherIndex < 0 {
			transformed = append(transformed, redirected)
		} else if !safeAdd(&transformed[otherIndex].UploadBytes, redirected.UploadBytes) ||
			!safeAdd(&transformed[otherIndex].DownloadBytes, redirected.DownloadBytes) ||
			!safeAdd(&transformed[otherIndex].FlowObservationCount, redirected.FlowObservationCount) {
			return nil, ErrInvalidBatch
		}
	}
	if err := validateFlows(global, transformed); err != nil {
		return nil, err
	}
	return transformed, nil
}

func concreteDimensionExists(
	ctx context.Context,
	transaction *sql.Tx,
	dimension FlowDimension,
) (bool, error) {
	var exists int
	if err := transaction.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM flow_dimension
			WHERE source_family = ? AND source_network = ? AND source_prefix_len = ?
				AND destination_family = ? AND destination_ip = ? AND destination_port = ?
				AND host = ? AND network_code = ? AND classification_code = ?
		)
	`,
		dimension.SourceFamily,
		normalizedBlob(dimension.SourceNetwork),
		dimension.SourcePrefixLen,
		dimension.DestinationFamily,
		normalizedBlob(dimension.DestinationIP),
		dimension.DestinationPort,
		dimension.Host,
		dimension.NetworkCode,
		dimension.ClassificationCode,
	).Scan(&exists); err != nil {
		return false, errors.New("cannot inspect FlowLens dimension capacity")
	}
	return exists != 0, nil
}

func upsertTrafficRollup(ctx context.Context, transaction *sql.Tx, rollup TrafficRollup) error {
	_, err := transaction.ExecContext(ctx, `
		INSERT INTO traffic_rollup (
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
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		)
		ON CONFLICT(resolution_sec, bucket_start) DO UPDATE SET
			bucket_end = excluded.bucket_end,
			upload_bytes = excluded.upload_bytes,
			download_bytes = excluded.download_bytes,
			recovered_upload_bytes = excluded.recovered_upload_bytes,
			recovered_download_bytes = excluded.recovered_download_bytes,
			speed_upload_sample_sum = excluded.speed_upload_sample_sum,
			speed_download_sample_sum = excluded.speed_download_sample_sum,
			speed_sample_count = excluded.speed_sample_count,
			peak_upload_bytes_per_second = excluded.peak_upload_bytes_per_second,
			peak_download_bytes_per_second = excluded.peak_download_bytes_per_second,
			peak_upload_at = excluded.peak_upload_at,
			peak_download_at = excluded.peak_download_at,
			counter_observed_seconds = excluded.counter_observed_seconds,
			attribution_observed_seconds = excluded.attribution_observed_seconds,
			active_connections_sum = excluded.active_connections_sum,
			active_connections_samples = excluded.active_connections_samples,
			active_connections_max = excluded.active_connections_max,
			memory_bytes_sum = excluded.memory_bytes_sum,
			memory_samples = excluded.memory_samples,
			memory_bytes_max = excluded.memory_bytes_max,
			unattributed_upload_bytes = excluded.unattributed_upload_bytes,
			unattributed_download_bytes = excluded.unattributed_download_bytes,
			reset_count = excluded.reset_count,
			quality_flags = excluded.quality_flags
	`,
		rollup.ResolutionSec,
		rollup.BucketStart,
		rollup.BucketEnd,
		rollup.UploadBytes,
		rollup.DownloadBytes,
		rollup.RecoveredUploadBytes,
		rollup.RecoveredDownloadBytes,
		rollup.SpeedUploadSampleSum,
		rollup.SpeedDownloadSampleSum,
		rollup.SpeedSampleCount,
		rollup.PeakUploadBytesPerSecond,
		rollup.PeakDownloadBytesPerSecond,
		nullableInt64(rollup.PeakUploadAt),
		nullableInt64(rollup.PeakDownloadAt),
		rollup.CounterObservedSeconds,
		rollup.AttributionObservedSeconds,
		rollup.ActiveConnectionsSum,
		rollup.ActiveConnectionsSamples,
		rollup.ActiveConnectionsMax,
		rollup.MemoryBytesSum,
		rollup.MemorySamples,
		rollup.MemoryBytesMax,
		rollup.UnattributedUploadBytes,
		rollup.UnattributedDownloadBytes,
		rollup.ResetCount,
		rollup.QualityFlags,
	)
	if err != nil {
		return errors.New("cannot write FlowLens global bucket")
	}
	return nil
}

func replaceFlowRollups(ctx context.Context, transaction *sql.Tx, global TrafficRollup, flows []FlowRollup) error {
	if _, err := transaction.ExecContext(ctx,
		`DELETE FROM flow_rollup WHERE resolution_sec = ? AND bucket_start = ?`,
		global.ResolutionSec,
		global.BucketStart,
	); err != nil {
		return errors.New("cannot replace FlowLens dimensional bucket")
	}
	for _, flow := range flows {
		dimensionID, err := resolveDimension(ctx, transaction, flow.Dimension)
		if err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO flow_rollup (
				resolution_sec, bucket_start, dimension_id,
				upload_bytes, download_bytes, flow_observation_count
			) VALUES (?, ?, ?, ?, ?, ?)
		`,
			global.ResolutionSec,
			global.BucketStart,
			dimensionID,
			flow.UploadBytes,
			flow.DownloadBytes,
			flow.FlowObservationCount,
		); err != nil {
			return errors.New("cannot write FlowLens dimensional row")
		}
	}
	return nil
}

func resolveDimension(ctx context.Context, transaction *sql.Tx, dimension FlowDimension) (int64, error) {
	sourceNetwork := normalizedBlob(dimension.SourceNetwork)
	destinationIP := normalizedBlob(dimension.DestinationIP)
	arguments := []any{
		dimension.SourceFamily,
		sourceNetwork,
		dimension.SourcePrefixLen,
		dimension.DestinationFamily,
		destinationIP,
		dimension.DestinationPort,
		dimension.Host,
		dimension.NetworkCode,
		dimension.ClassificationCode,
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO flow_dimension (
			source_family, source_network, source_prefix_len,
			destination_family, destination_ip, destination_port,
			host, network_code, classification_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (
			source_family, source_network, source_prefix_len,
			destination_family, destination_ip, destination_port,
			host, network_code, classification_code
		) DO NOTHING
	`, arguments...); err != nil {
		return 0, errors.New("cannot resolve FlowLens dimension")
	}
	var dimensionID int64
	if err := transaction.QueryRowContext(ctx, `
		SELECT id
		FROM flow_dimension
		WHERE source_family = ? AND source_network = ? AND source_prefix_len = ?
			AND destination_family = ? AND destination_ip = ? AND destination_port = ?
			AND host = ? AND network_code = ? AND classification_code = ?
	`, arguments...).Scan(&dimensionID); err != nil {
		return 0, errors.New("cannot read FlowLens dimension")
	}
	return dimensionID, nil
}

func exactlyOneRow(result sql.Result) bool {
	if result == nil {
		return false
	}
	affected, err := result.RowsAffected()
	return err == nil && affected == 1
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func normalizedBlob(value []byte) []byte {
	if len(value) == 0 {
		return []byte{}
	}
	return value
}
