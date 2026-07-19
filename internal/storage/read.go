package storage

import (
	"context"
	"database/sql"
	"errors"
)

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// LoadCollectorState returns the single durable cursor when present.
func (s *Store) LoadCollectorState(ctx context.Context) (CollectorState, bool, error) {
	return loadCollectorStateFromQuery(ctx, s.db)
}

func loadCollectorStateFromQuery(ctx context.Context, queryer rowQuerier) (CollectorState, bool, error) {
	var state CollectorState
	err := queryer.QueryRowContext(ctx, `
		SELECT runtime_session_id, last_upload_total, last_download_total,
			last_sample_at, last_batch_id, bucket_timezone
		FROM collector_state
		WHERE id = 1
	`).Scan(
		&state.RuntimeSessionID,
		&state.LastTotals.Upload,
		&state.LastTotals.Download,
		&state.LastSampleAt,
		&state.LastBatchID,
		&state.BucketTimezone,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CollectorState{}, false, nil
	}
	if err != nil {
		return CollectorState{}, false, errors.New("cannot read FlowLens collector state")
	}
	return state, true, nil
}

// TrafficRollup returns one exact global bucket.
func (s *Store) TrafficRollup(ctx context.Context, resolutionSec, bucketStart int64) (TrafficRollup, bool, error) {
	var rollup TrafficRollup
	var peakUploadAt, peakDownloadAt sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
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
		WHERE resolution_sec = ? AND bucket_start = ?
	`, resolutionSec, bucketStart).Scan(
		&rollup.ResolutionSec,
		&rollup.BucketStart,
		&rollup.BucketEnd,
		&rollup.UploadBytes,
		&rollup.DownloadBytes,
		&rollup.RecoveredUploadBytes,
		&rollup.RecoveredDownloadBytes,
		&rollup.SpeedUploadSampleSum,
		&rollup.SpeedDownloadSampleSum,
		&rollup.SpeedSampleCount,
		&rollup.PeakUploadBytesPerSecond,
		&rollup.PeakDownloadBytesPerSecond,
		&peakUploadAt,
		&peakDownloadAt,
		&rollup.CounterObservedSeconds,
		&rollup.AttributionObservedSeconds,
		&rollup.ActiveConnectionsSum,
		&rollup.ActiveConnectionsSamples,
		&rollup.ActiveConnectionsMax,
		&rollup.MemoryBytesSum,
		&rollup.MemorySamples,
		&rollup.MemoryBytesMax,
		&rollup.UnattributedUploadBytes,
		&rollup.UnattributedDownloadBytes,
		&rollup.ResetCount,
		&rollup.QualityFlags,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return TrafficRollup{}, false, nil
	}
	if err != nil {
		return TrafficRollup{}, false, errors.New("cannot read FlowLens global bucket")
	}
	rollup.PeakUploadAt = nullInt64Pointer(peakUploadAt)
	rollup.PeakDownloadAt = nullInt64Pointer(peakDownloadAt)
	return rollup, true, nil
}

// FlowRollups returns chronological-query-safe copies of one dimensional bucket.
func (s *Store) FlowRollups(ctx context.Context, resolutionSec, bucketStart int64) ([]FlowRollup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			d.source_family, d.source_network, d.source_prefix_len,
			d.destination_family, d.destination_ip, d.destination_port,
			d.host, d.network_code, d.classification_code,
			f.upload_bytes, f.download_bytes, f.flow_observation_count
		FROM flow_rollup AS f
		JOIN flow_dimension AS d ON d.id = f.dimension_id
		WHERE f.resolution_sec = ? AND f.bucket_start = ?
		ORDER BY d.classification_code, d.id
	`, resolutionSec, bucketStart)
	if err != nil {
		return nil, errors.New("cannot read FlowLens dimensional bucket")
	}
	defer rows.Close()
	var flows []FlowRollup
	for rows.Next() {
		var flow FlowRollup
		if err := rows.Scan(
			&flow.Dimension.SourceFamily,
			&flow.Dimension.SourceNetwork,
			&flow.Dimension.SourcePrefixLen,
			&flow.Dimension.DestinationFamily,
			&flow.Dimension.DestinationIP,
			&flow.Dimension.DestinationPort,
			&flow.Dimension.Host,
			&flow.Dimension.NetworkCode,
			&flow.Dimension.ClassificationCode,
			&flow.UploadBytes,
			&flow.DownloadBytes,
			&flow.FlowObservationCount,
		); err != nil {
			return nil, errors.New("cannot read FlowLens dimensional row")
		}
		flow.Dimension.SourceNetwork = append([]byte(nil), flow.Dimension.SourceNetwork...)
		flow.Dimension.DestinationIP = append([]byte(nil), flow.Dimension.DestinationIP...)
		flows = append(flows, flow)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("cannot read FlowLens dimensional bucket")
	}
	return flows, nil
}

// RuntimeSession returns one persisted session.
func (s *Store) RuntimeSession(ctx context.Context, id string) (RuntimeSession, bool, error) {
	var session RuntimeSession
	var endedAt sql.NullInt64
	var endReason, hostBootID sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, started_at, ended_at, start_reason, end_reason,
			last_upload_total, last_download_total, last_seen_at,
			sing_box_version, host_boot_id, data_gap_before_seconds
		FROM runtime_session
		WHERE id = ?
	`, id).Scan(
		&session.ID,
		&session.StartedAt,
		&endedAt,
		&session.StartReason,
		&endReason,
		&session.LastTotals.Upload,
		&session.LastTotals.Download,
		&session.LastSeenAt,
		&session.SingBoxVersion,
		&hostBootID,
		&session.DataGapBeforeSeconds,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeSession{}, false, nil
	}
	if err != nil {
		return RuntimeSession{}, false, errors.New("cannot read FlowLens runtime session")
	}
	session.EndedAt = nullInt64Pointer(endedAt)
	session.EndReason = nullStringPointer(endReason)
	session.HostBootID = nullStringPointer(hostBootID)
	return session, true, nil
}

// RuntimeSessions returns a bounded newest-first session list.
func (s *Store) RuntimeSessions(ctx context.Context, limit int) ([]RuntimeSession, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrInvalidQuery
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, started_at, ended_at, start_reason, end_reason,
			last_upload_total, last_download_total, last_seen_at,
			sing_box_version, host_boot_id, data_gap_before_seconds
		FROM runtime_session
		ORDER BY started_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, errors.New("cannot read FlowLens runtime sessions")
	}
	defer rows.Close()
	result := make([]RuntimeSession, 0, limit)
	for rows.Next() {
		var session RuntimeSession
		var endedAt sql.NullInt64
		var endReason, hostBootID sql.NullString
		if err := rows.Scan(
			&session.ID, &session.StartedAt, &endedAt, &session.StartReason, &endReason,
			&session.LastTotals.Upload, &session.LastTotals.Download, &session.LastSeenAt,
			&session.SingBoxVersion, &hostBootID, &session.DataGapBeforeSeconds,
		); err != nil {
			return nil, errors.New("cannot read FlowLens runtime session")
		}
		session.EndedAt = nullInt64Pointer(endedAt)
		session.EndReason = nullStringPointer(endReason)
		session.HostBootID = nullStringPointer(hostBootID)
		result = append(result, session)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("cannot read FlowLens runtime sessions")
	}
	return result, nil
}

// QualityEventCount returns the number of events committed by one stable batch.
func (s *Store) QualityEventCount(ctx context.Context, batchID string) (int64, error) {
	var count int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM quality_event WHERE batch_id = ?`,
		batchID,
	).Scan(&count); err != nil {
		return 0, errors.New("cannot count FlowLens quality events")
	}
	return count, nil
}

func nullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}
