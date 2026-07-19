package storage

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidLabel  = errors.New("invalid FlowLens service label")
	ErrLabelConflict = errors.New("FlowLens service label already exists")
	ErrLabelNotFound = errors.New("FlowLens service label not found")
)

// ServiceLabel is one durable local display alias.
type ServiceLabel struct {
	ID          int64
	LabelType   string
	MatchValue  string
	DisplayName string
	CreatedAt   int64
	UpdatedAt   int64
}

// Labels returns every label in stable type/key order.
func (s *Store) Labels(ctx context.Context) ([]ServiceLabel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, label_type, match_value, display_name, created_at, updated_at
		FROM service_label
		ORDER BY label_type, match_value
	`)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("cannot read FlowLens service labels")
	}
	defer rows.Close()
	var result []ServiceLabel
	for rows.Next() {
		var label ServiceLabel
		if err := rows.Scan(&label.ID, &label.LabelType, &label.MatchValue, &label.DisplayName, &label.CreatedAt, &label.UpdatedAt); err != nil {
			return nil, errors.New("cannot read FlowLens service label")
		}
		result = append(result, label)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("cannot read FlowLens service labels")
	}
	return result, nil
}

// CreateLabel inserts one already canonical validated label.
func (s *Store) CreateLabel(ctx context.Context, value ServiceLabel) (ServiceLabel, error) {
	if !validServiceLabel(value, false) {
		return ServiceLabel{}, ErrInvalidLabel
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO service_label(label_type, match_value, display_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, value.LabelType, value.MatchValue, value.DisplayName, value.CreatedAt, value.UpdatedAt)
	if err != nil {
		if ctx.Err() != nil {
			return ServiceLabel{}, ctx.Err()
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ServiceLabel{}, ErrLabelConflict
		}
		return ServiceLabel{}, errors.New("cannot create FlowLens service label")
	}
	value.ID, err = result.LastInsertId()
	if err != nil || value.ID <= 0 {
		return ServiceLabel{}, errors.New("cannot identify FlowLens service label")
	}
	return value, nil
}

// UpdateLabel changes only the display name and update timestamp.
func (s *Store) UpdateLabel(ctx context.Context, id int64, displayName string, updatedAt int64) (ServiceLabel, error) {
	if id <= 0 || !validDisplayName(displayName) || updatedAt <= 0 {
		return ServiceLabel{}, ErrInvalidLabel
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE service_label SET display_name = ?, updated_at = ? WHERE id = ?
	`, displayName, updatedAt, id)
	if err != nil {
		if ctx.Err() != nil {
			return ServiceLabel{}, ctx.Err()
		}
		return ServiceLabel{}, errors.New("cannot update FlowLens service label")
	}
	count, err := result.RowsAffected()
	if err != nil {
		return ServiceLabel{}, errors.New("cannot update FlowLens service label")
	}
	if count == 0 {
		return ServiceLabel{}, ErrLabelNotFound
	}
	var value ServiceLabel
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, label_type, match_value, display_name, created_at, updated_at
		FROM service_label WHERE id = ?
	`, id).Scan(&value.ID, &value.LabelType, &value.MatchValue, &value.DisplayName, &value.CreatedAt, &value.UpdatedAt); err != nil {
		return ServiceLabel{}, errors.New("cannot read FlowLens service label")
	}
	return value, nil
}

// DeleteLabel removes one alias and reports whether it existed.
func (s *Store) DeleteLabel(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, ErrInvalidLabel
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM service_label WHERE id = ?`, id)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, errors.New("cannot delete FlowLens service label")
	}
	count, err := result.RowsAffected()
	if err != nil || count < 0 || count > 1 {
		return false, errors.New("cannot delete FlowLens service label")
	}
	return count == 1, nil
}

func validServiceLabel(value ServiceLabel, requireID bool) bool {
	return (!requireID || value.ID > 0) && (value.LabelType == "host" || value.LabelType == "endpoint") &&
		value.MatchValue != "" && strings.TrimSpace(value.MatchValue) == value.MatchValue && len(value.MatchValue) <= 512 &&
		validDisplayName(value.DisplayName) && value.CreatedAt > 0 && value.UpdatedAt >= value.CreatedAt
}

func validDisplayName(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 64
}
