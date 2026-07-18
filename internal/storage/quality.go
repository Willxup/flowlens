package storage

import (
	"context"
	"database/sql"
	"errors"
)

// QualityEventRecord is the public-safe quality event projection.
type QualityEventRecord struct {
	Code      string
	StartedAt int64
	EndedAt   *int64
	Flags     int64
}

// QualityEvents returns events whose start lies in the requested half-open range.
func (s *Store) QualityEvents(
	ctx context.Context,
	from int64,
	to int64,
) ([]QualityEventRecord, error) {
	if from <= 0 || to <= from {
		return nil, ErrInvalidQuery
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT code, started_at, ended_at, flags
		FROM quality_event
		WHERE started_at >= ? AND started_at < ?
		ORDER BY started_at, id
	`, from, to)
	if err != nil {
		return nil, errors.New("cannot query FlowLens quality events")
	}
	defer rows.Close()
	var result []QualityEventRecord
	for rows.Next() {
		var event QualityEventRecord
		var endedAt sql.NullInt64
		if err := rows.Scan(&event.Code, &event.StartedAt, &endedAt, &event.Flags); err != nil {
			return nil, errors.New("cannot read FlowLens quality event")
		}
		event.EndedAt = nullInt64Pointer(endedAt)
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("cannot iterate FlowLens quality events")
	}
	return result, nil
}
