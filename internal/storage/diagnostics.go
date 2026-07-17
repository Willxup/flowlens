package storage

import (
	"context"
	"errors"
	"strings"
)

// Diagnostics reports non-sensitive SQLite policy values.
type Diagnostics struct {
	JournalMode   string
	Synchronous   int64
	ForeignKeys   int64
	BusyTimeoutMS int64
	AutoVacuum    int64
	TempStore     int64
}

// Diagnostics reads all connection-local policy values through one connection.
func (s *Store) Diagnostics(ctx context.Context) (Diagnostics, error) {
	connection, err := s.db.Conn(ctx)
	if err != nil {
		return Diagnostics{}, errors.New("cannot acquire SQLite diagnostics connection")
	}
	defer connection.Close()

	var diagnostics Diagnostics
	if err := connection.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&diagnostics.JournalMode); err != nil {
		return Diagnostics{}, errors.New("cannot read SQLite journal policy")
	}
	queries := []struct {
		statement string
		target    *int64
	}{
		{"PRAGMA synchronous", &diagnostics.Synchronous},
		{"PRAGMA foreign_keys", &diagnostics.ForeignKeys},
		{"PRAGMA busy_timeout", &diagnostics.BusyTimeoutMS},
		{"PRAGMA auto_vacuum", &diagnostics.AutoVacuum},
		{"PRAGMA temp_store", &diagnostics.TempStore},
	}
	for _, query := range queries {
		if err := connection.QueryRowContext(ctx, query.statement).Scan(query.target); err != nil {
			return Diagnostics{}, errors.New("cannot read SQLite connection policy")
		}
	}
	diagnostics.JournalMode = strings.ToLower(diagnostics.JournalMode)
	return diagnostics, nil
}

// QuickCheck accepts only SQLite's single exact "ok" integrity result.
func (s *Store) QuickCheck(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA quick_check")
	if err != nil {
		return errors.New("cannot run SQLite quick check")
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return errors.New("cannot read SQLite quick check result")
		}
		count++
		if result != "ok" {
			return errors.New("SQLite quick check failed")
		}
	}
	if err := rows.Err(); err != nil {
		return errors.New("cannot read SQLite quick check result")
	}
	if count != 1 {
		return errors.New("SQLite quick check returned an unexpected result")
	}
	return nil
}

func (s *Store) verifyConnectionPolicy(ctx context.Context) error {
	diagnostics, err := s.Diagnostics(ctx)
	if err != nil {
		return err
	}
	if diagnostics != (Diagnostics{
		JournalMode:   "wal",
		Synchronous:   1,
		ForeignKeys:   1,
		BusyTimeoutMS: 5000,
		AutoVacuum:    2,
		TempStore:     2,
	}) {
		return errors.New("SQLite connection policy is unavailable")
	}
	return nil
}
