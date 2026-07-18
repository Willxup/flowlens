package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrInvalidMaintenance = errors.New("invalid FlowLens maintenance request")

// MaintenanceRun is one redacted durable maintenance outcome.
type MaintenanceRun struct {
	Operation     string
	StartedAt     int64
	EndedAt       *int64
	DeletedRows   int64
	DatabaseBytes int64
	WALBytes      int64
	Error         *string
}

func (MaintenanceRun) String() string       { return "MaintenanceRun{redacted}" }
func (run MaintenanceRun) GoString() string { return run.String() }

// Checkpoint runs the planned passive or truncate WAL checkpoint.
func (s *Store) Checkpoint(ctx context.Context, truncate bool) error {
	mode := "PASSIVE"
	if truncate {
		mode = "TRUNCATE"
	}
	var busy, logPages, checkpointedPages int64
	if err := s.db.QueryRowContext(ctx, "PRAGMA wal_checkpoint("+mode+")").Scan(
		&busy, &logPages, &checkpointedPages,
	); err != nil || busy != 0 || logPages < 0 || checkpointedPages < 0 {
		return errors.New("cannot checkpoint FlowLens SQLite database")
	}
	return nil
}

// Optimize refreshes SQLite planner statistics after daily maintenance.
func (s *Store) Optimize(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA optimize"); err != nil {
		return errors.New("cannot optimize FlowLens SQLite database")
	}
	return nil
}

// IncrementalVacuum releases one bounded batch of free pages.
func (s *Store) IncrementalVacuum(ctx context.Context, pages int64) error {
	if pages < 1 || pages > 1024 {
		return ErrInvalidMaintenance
	}
	statement := fmt.Sprintf("PRAGMA incremental_vacuum(%d)", pages)
	if _, err := s.db.ExecContext(ctx, statement); err != nil {
		return errors.New("cannot vacuum FlowLens SQLite database")
	}
	return nil
}

// RecordMaintenance appends one already-redacted maintenance outcome.
func (s *Store) RecordMaintenance(ctx context.Context, run MaintenanceRun) error {
	if !validMaintenanceRun(run) {
		return ErrInvalidMaintenance
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO maintenance_run (
			operation, started_at, ended_at, deleted_rows,
			database_bytes, wal_bytes, error
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		run.Operation,
		run.StartedAt,
		nullableInt64(run.EndedAt),
		run.DeletedRows,
		run.DatabaseBytes,
		run.WALBytes,
		nullableString(run.Error),
	); err != nil {
		return errors.New("cannot record FlowLens maintenance run")
	}
	return nil
}

// LatestMaintenance returns the newest run for one stable operation code.
func (s *Store) LatestMaintenance(
	ctx context.Context,
	operation string,
) (MaintenanceRun, bool, error) {
	if !validBoundedText(operation, 64) {
		return MaintenanceRun{}, false, ErrInvalidMaintenance
	}
	var run MaintenanceRun
	var endedAt sql.NullInt64
	var errorText sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT operation, started_at, ended_at, deleted_rows,
			database_bytes, wal_bytes, error
		FROM maintenance_run
		WHERE operation = ?
		ORDER BY id DESC
		LIMIT 1
	`, operation).Scan(
		&run.Operation,
		&run.StartedAt,
		&endedAt,
		&run.DeletedRows,
		&run.DatabaseBytes,
		&run.WALBytes,
		&errorText,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MaintenanceRun{}, false, nil
	}
	if err != nil {
		return MaintenanceRun{}, false, errors.New("cannot read FlowLens maintenance run")
	}
	run.EndedAt = nullInt64Pointer(endedAt)
	run.Error = nullStringPointer(errorText)
	return run, true, nil
}

// CleanupMaintenance removes diagnostic rows older than the exclusive cutoff.
func (s *Store) CleanupMaintenance(ctx context.Context, before int64) (int64, error) {
	if before <= 0 {
		return 0, ErrInvalidMaintenance
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM maintenance_run WHERE started_at < ?`, before)
	if err != nil {
		return 0, errors.New("cannot clean FlowLens maintenance runs")
	}
	return affectedRows(result)
}

func validMaintenanceRun(run MaintenanceRun) bool {
	if !validBoundedText(run.Operation, 64) || run.StartedAt <= 0 ||
		run.DeletedRows < 0 || run.DatabaseBytes < 0 || run.WALBytes < 0 {
		return false
	}
	if run.EndedAt != nil && *run.EndedAt < run.StartedAt {
		return false
	}
	return run.Error == nil || len(*run.Error) <= 4096
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
