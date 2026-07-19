package storage

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/Willxup/flowlens/internal/rollup"
)

var (
	// ErrInvalidRollup means the requested source-to-target edge is not part of
	// the Stage 2 rollup graph or the target window is malformed.
	ErrInvalidRollup = errors.New("invalid FlowLens rollup request")
	// ErrNoSourceRollups prevents a data gap from being represented as zero
	// traffic merely because no source buckets exist.
	ErrNoSourceRollups = errors.New("no FlowLens source rollups")
)

// RollupTraffic fully recomputes and atomically replaces one global target
// bucket from the planned source resolution.
func (s *Store) RollupTraffic(
	ctx context.Context,
	sourceResolutionSec int64,
	window rollup.Window,
	topKValues ...int,
) (TrafficRollup, error) {
	topK := 20
	if len(topKValues) == 1 {
		topK = topKValues[0]
	}
	if len(topKValues) > 1 || topK < 1 || topK > 100 || !validRollupEdge(sourceResolutionSec, window) {
		return TrafficRollup{}, ErrInvalidRollup
	}
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TrafficRollup{}, errors.New("cannot begin FlowLens rollup transaction")
	}
	defer transaction.Rollback()

	rows, err := transaction.QueryContext(ctx, `
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
	`, sourceResolutionSec, window.BucketStart, window.BucketEnd)
	if err != nil {
		return TrafficRollup{}, errors.New("cannot read FlowLens source rollups")
	}
	target := TrafficRollup{
		ResolutionSec: window.ResolutionSec,
		BucketStart:   window.BucketStart,
		BucketEnd:     window.BucketEnd,
	}
	found := false
	for rows.Next() {
		source, err := scanTrafficRollup(rows)
		if err != nil {
			_ = rows.Close()
			return TrafficRollup{}, errors.New("cannot read FlowLens source rollup")
		}
		found = true
		if !addTrafficRollup(&target, source) {
			_ = rows.Close()
			return TrafficRollup{}, ErrInvalidRollup
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return TrafficRollup{}, errors.New("cannot iterate FlowLens source rollups")
	}
	if err := rows.Close(); err != nil {
		return TrafficRollup{}, errors.New("cannot close FlowLens source rollups")
	}
	if !found {
		return TrafficRollup{}, ErrNoSourceRollups
	}
	flows, err := recomputeFlowRollup(ctx, transaction, sourceResolutionSec, window, target, topK)
	if err != nil {
		return TrafficRollup{}, err
	}
	if err := upsertTrafficRollup(ctx, transaction, target); err != nil {
		return TrafficRollup{}, err
	}
	if err := replaceFlowRollups(ctx, transaction, target, flows); err != nil {
		return TrafficRollup{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TrafficRollup{}, errors.New("cannot commit FlowLens rollup transaction")
	}
	return target, nil
}

func recomputeFlowRollup(
	ctx context.Context,
	transaction *sql.Tx,
	sourceResolutionSec int64,
	window rollup.Window,
	target TrafficRollup,
	topK int,
) ([]FlowRollup, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT
			d.source_family, d.source_network, d.source_prefix_len,
			d.destination_family, d.destination_ip, d.destination_port,
			d.host, d.network_code, d.classification_code,
			f.upload_bytes, f.download_bytes, f.flow_observation_count
		FROM flow_rollup AS f
		JOIN flow_dimension AS d ON d.id = f.dimension_id
		WHERE f.resolution_sec = ? AND f.bucket_start >= ? AND f.bucket_start < ?
	`, sourceResolutionSec, window.BucketStart, window.BucketEnd)
	if err != nil {
		return nil, errors.New("cannot read FlowLens source dimensional rollups")
	}
	defer rows.Close()
	concrete := make(map[string]FlowRollup)
	other := FlowRollup{Dimension: specialFlowDimension(2)}
	unattributed := FlowRollup{Dimension: specialFlowDimension(3)}
	for rows.Next() {
		var flow FlowRollup
		if err := rows.Scan(
			&flow.Dimension.SourceFamily, &flow.Dimension.SourceNetwork, &flow.Dimension.SourcePrefixLen,
			&flow.Dimension.DestinationFamily, &flow.Dimension.DestinationIP, &flow.Dimension.DestinationPort,
			&flow.Dimension.Host, &flow.Dimension.NetworkCode, &flow.Dimension.ClassificationCode,
			&flow.UploadBytes, &flow.DownloadBytes, &flow.FlowObservationCount,
		); err != nil {
			return nil, errors.New("cannot read FlowLens source dimensional rollup")
		}
		var destination *FlowRollup
		switch flow.Dimension.ClassificationCode {
		case 1:
			key := dimensionKey(flow.Dimension)
			merged := concrete[key]
			if merged.Dimension.ClassificationCode == 0 {
				merged.Dimension = cloneStoredDimension(flow.Dimension)
			}
			destination = &merged
			if !safeAdd(&destination.UploadBytes, flow.UploadBytes) || !safeAdd(&destination.DownloadBytes, flow.DownloadBytes) ||
				!safeAdd(&destination.FlowObservationCount, flow.FlowObservationCount) {
				return nil, ErrInvalidRollup
			}
			concrete[key] = *destination
			continue
		case 2:
			destination = &other
		case 3:
			destination = &unattributed
		default:
			return nil, ErrInvalidRollup
		}
		if !safeAdd(&destination.UploadBytes, flow.UploadBytes) || !safeAdd(&destination.DownloadBytes, flow.DownloadBytes) ||
			!safeAdd(&destination.FlowObservationCount, flow.FlowObservationCount) {
			return nil, ErrInvalidRollup
		}
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("cannot iterate FlowLens source dimensional rollups")
	}
	if target.UploadBytes == 0 && target.DownloadBytes == 0 {
		return nil, nil
	}
	values := make([]FlowRollup, 0, len(concrete))
	for _, flow := range concrete {
		values = append(values, flow)
	}
	sort.Slice(values, func(left, right int) bool {
		leftTotal := uint64(values[left].UploadBytes) + uint64(values[left].DownloadBytes)
		rightTotal := uint64(values[right].UploadBytes) + uint64(values[right].DownloadBytes)
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		return dimensionKey(values[left].Dimension) < dimensionKey(values[right].Dimension)
	})
	keep := len(values)
	if keep > topK {
		keep = topK
	}
	for _, flow := range values[keep:] {
		if !safeAdd(&other.UploadBytes, flow.UploadBytes) || !safeAdd(&other.DownloadBytes, flow.DownloadBytes) ||
			!safeAdd(&other.FlowObservationCount, flow.FlowObservationCount) {
			return nil, ErrInvalidRollup
		}
	}
	result := append([]FlowRollup(nil), values[:keep]...)
	result = append(result, other, unattributed)
	if err := validateFlows(target, result); err != nil {
		return nil, ErrInvalidRollup
	}
	return result, nil
}

func specialFlowDimension(classification int64) FlowDimension {
	return FlowDimension{
		SourceNetwork: []byte{}, DestinationIP: []byte{}, DestinationPort: -1,
		ClassificationCode: classification,
	}
}

func cloneStoredDimension(value FlowDimension) FlowDimension {
	value.SourceNetwork = append([]byte(nil), value.SourceNetwork...)
	value.DestinationIP = append([]byte(nil), value.DestinationIP...)
	return value
}

func validRollupEdge(sourceResolutionSec int64, window rollup.Window) bool {
	if !validRollupResolutionEdge(sourceResolutionSec, window.ResolutionSec) {
		return false
	}
	if window.BucketStart <= 0 || window.BucketEnd <= window.BucketStart {
		return false
	}
	duration := window.BucketEnd - window.BucketStart
	if window.ResolutionSec == rollup.ResolutionDay {
		return duration >= 22*rollup.ResolutionHour && duration <= 26*rollup.ResolutionHour
	}
	return duration == window.ResolutionSec && window.BucketStart%window.ResolutionSec == 0
}

func validRollupResolutionEdge(sourceResolutionSec, targetResolutionSec int64) bool {
	switch targetResolutionSec {
	case rollup.ResolutionMinute:
		return sourceResolutionSec == rollup.ResolutionTenSeconds
	case rollup.ResolutionHalfHour:
		return sourceResolutionSec == rollup.ResolutionMinute
	case rollup.ResolutionHour:
		return sourceResolutionSec == rollup.ResolutionHalfHour
	case rollup.ResolutionDay:
		return sourceResolutionSec == rollup.ResolutionMinute
	default:
		return false
	}
}

func scanTrafficRollup(scanner interface{ Scan(...any) error }) (TrafficRollup, error) {
	var value TrafficRollup
	var peakUploadAt, peakDownloadAt sql.NullInt64
	err := scanner.Scan(
		&value.ResolutionSec,
		&value.BucketStart,
		&value.BucketEnd,
		&value.UploadBytes,
		&value.DownloadBytes,
		&value.RecoveredUploadBytes,
		&value.RecoveredDownloadBytes,
		&value.SpeedUploadSampleSum,
		&value.SpeedDownloadSampleSum,
		&value.SpeedSampleCount,
		&value.PeakUploadBytesPerSecond,
		&value.PeakDownloadBytesPerSecond,
		&peakUploadAt,
		&peakDownloadAt,
		&value.CounterObservedSeconds,
		&value.AttributionObservedSeconds,
		&value.ActiveConnectionsSum,
		&value.ActiveConnectionsSamples,
		&value.ActiveConnectionsMax,
		&value.MemoryBytesSum,
		&value.MemorySamples,
		&value.MemoryBytesMax,
		&value.UnattributedUploadBytes,
		&value.UnattributedDownloadBytes,
		&value.ResetCount,
		&value.QualityFlags,
	)
	if err != nil {
		return TrafficRollup{}, err
	}
	value.PeakUploadAt = nullInt64Pointer(peakUploadAt)
	value.PeakDownloadAt = nullInt64Pointer(peakDownloadAt)
	return value, nil
}

func addTrafficRollup(target *TrafficRollup, source TrafficRollup) bool {
	additions := []struct {
		total *int64
		value int64
	}{
		{&target.UploadBytes, source.UploadBytes},
		{&target.DownloadBytes, source.DownloadBytes},
		{&target.RecoveredUploadBytes, source.RecoveredUploadBytes},
		{&target.RecoveredDownloadBytes, source.RecoveredDownloadBytes},
		{&target.SpeedUploadSampleSum, source.SpeedUploadSampleSum},
		{&target.SpeedDownloadSampleSum, source.SpeedDownloadSampleSum},
		{&target.SpeedSampleCount, source.SpeedSampleCount},
		{&target.CounterObservedSeconds, source.CounterObservedSeconds},
		{&target.AttributionObservedSeconds, source.AttributionObservedSeconds},
		{&target.ActiveConnectionsSum, source.ActiveConnectionsSum},
		{&target.ActiveConnectionsSamples, source.ActiveConnectionsSamples},
		{&target.MemoryBytesSum, source.MemoryBytesSum},
		{&target.MemorySamples, source.MemorySamples},
		{&target.UnattributedUploadBytes, source.UnattributedUploadBytes},
		{&target.UnattributedDownloadBytes, source.UnattributedDownloadBytes},
		{&target.ResetCount, source.ResetCount},
	}
	for _, addition := range additions {
		if !safeAdd(addition.total, addition.value) {
			return false
		}
	}
	if source.ActiveConnectionsMax > target.ActiveConnectionsMax {
		target.ActiveConnectionsMax = source.ActiveConnectionsMax
	}
	if source.MemoryBytesMax > target.MemoryBytesMax {
		target.MemoryBytesMax = source.MemoryBytesMax
	}
	if source.PeakUploadBytesPerSecond > target.PeakUploadBytesPerSecond ||
		(source.PeakUploadBytesPerSecond == target.PeakUploadBytesPerSecond &&
			target.PeakUploadAt == nil && source.PeakUploadAt != nil) {
		target.PeakUploadBytesPerSecond = source.PeakUploadBytesPerSecond
		target.PeakUploadAt = copyInt64Pointer(source.PeakUploadAt)
	}
	if source.PeakDownloadBytesPerSecond > target.PeakDownloadBytesPerSecond ||
		(source.PeakDownloadBytesPerSecond == target.PeakDownloadBytesPerSecond &&
			target.PeakDownloadAt == nil && source.PeakDownloadAt != nil) {
		target.PeakDownloadBytesPerSecond = source.PeakDownloadBytesPerSecond
		target.PeakDownloadAt = copyInt64Pointer(source.PeakDownloadAt)
	}
	target.QualityFlags |= source.QualityFlags
	return true
}

func copyInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
