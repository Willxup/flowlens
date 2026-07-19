package storage

import (
	"context"
	"errors"

	"github.com/Willxup/flowlens/internal/rollup"
)

// FlowPoint is one dimensional row paired with its durable bucket identity.
type FlowPoint struct {
	ResolutionSec    int64
	BucketStart      int64
	Dimension        FlowDimension
	UploadBytes      int64
	DownloadBytes    int64
	ObservationCount int64
}

// FlowSeries reads exact non-overlapping dimensional segments in chronological
// order without joining display labels.
func (s *Store) FlowSeries(ctx context.Context, segments []rollup.Segment) ([]FlowPoint, error) {
	if !validSeriesSegments(segments) {
		return nil, ErrInvalidQuery
	}
	return flowSeries(ctx, s.db, segments)
}

func flowSeries(ctx context.Context, queryer seriesQuerier, segments []rollup.Segment) ([]FlowPoint, error) {
	var result []FlowPoint
	for _, segment := range segments {
		rows, err := queryer.QueryContext(ctx, `
			SELECT
				f.resolution_sec, f.bucket_start,
				d.source_family, d.source_network, d.source_prefix_len,
				d.destination_family, d.destination_ip, d.destination_port,
				d.host, d.network_code, d.classification_code,
				f.upload_bytes, f.download_bytes, f.flow_observation_count
			FROM flow_rollup AS f
			JOIN traffic_rollup AS t
			  ON t.resolution_sec = f.resolution_sec AND t.bucket_start = f.bucket_start
			JOIN flow_dimension AS d ON d.id = f.dimension_id
			WHERE f.resolution_sec = ? AND t.bucket_start >= ? AND t.bucket_end <= ?
			ORDER BY f.bucket_start, d.classification_code, d.id
		`, segment.ResolutionSec, segment.From, segment.To)
		if err != nil {
			return nil, errors.New("cannot query FlowLens dimensional series")
		}
		for rows.Next() {
			var point FlowPoint
			if err := rows.Scan(
				&point.ResolutionSec, &point.BucketStart,
				&point.Dimension.SourceFamily, &point.Dimension.SourceNetwork, &point.Dimension.SourcePrefixLen,
				&point.Dimension.DestinationFamily, &point.Dimension.DestinationIP, &point.Dimension.DestinationPort,
				&point.Dimension.Host, &point.Dimension.NetworkCode, &point.Dimension.ClassificationCode,
				&point.UploadBytes, &point.DownloadBytes, &point.ObservationCount,
			); err != nil {
				_ = rows.Close()
				return nil, errors.New("cannot read FlowLens dimensional series")
			}
			if point.ResolutionSec != segment.ResolutionSec || point.BucketStart < segment.From || point.BucketStart >= segment.To {
				_ = rows.Close()
				return nil, ErrInvalidQuery
			}
			point.Dimension = cloneStoredDimension(point.Dimension)
			result = append(result, point)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, errors.New("cannot iterate FlowLens dimensional series")
		}
		if err := rows.Close(); err != nil {
			return nil, errors.New("cannot close FlowLens dimensional series")
		}
	}
	return result, nil
}
