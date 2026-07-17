package storage

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Willxup/flowlens/internal/migrations"
)

var (
	// ErrSchemaChecksum means an applied migration no longer matches the binary.
	ErrSchemaChecksum = errors.New("SQLite schema migration checksum mismatch")
	// ErrSchemaTooNew means the database was created by a newer FlowLens binary.
	ErrSchemaTooNew = errors.New("SQLite schema version is newer than this FlowLens binary")
	// ErrUnversionedDatabase means application tables exist without migration history.
	ErrUnversionedDatabase = errors.New("SQLite database contains an unversioned schema")
)

var createTablePattern = regexp.MustCompile(`(?im)CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-z_][a-z0-9_]*)`)

// SchemaStatus describes the recorded and embedded schema boundary.
type SchemaStatus struct {
	CurrentVersion int
	LatestVersion  int
	NeedsMigration bool
}

// InspectSchema performs a read-only migration history and table-set check.
func (s *Store) InspectSchema(ctx context.Context) (SchemaStatus, error) {
	migrationList, err := migrations.List()
	if err != nil {
		return SchemaStatus{}, errors.New("cannot inspect embedded SQLite schema")
	}
	latest := 0
	if len(migrationList) > 0 {
		latest = migrationList[len(migrationList)-1].Version
	}
	actualTables, err := s.applicationTables(ctx)
	if err != nil {
		return SchemaStatus{}, err
	}
	hasHistory := containsString(actualTables, "schema_migration")
	if !hasHistory {
		if len(actualTables) != 0 {
			return SchemaStatus{}, ErrUnversionedDatabase
		}
		return SchemaStatus{LatestVersion: latest, NeedsMigration: latest > 0}, nil
	}

	rows, err := s.db.QueryContext(ctx, `SELECT version, checksum FROM schema_migration ORDER BY version`)
	if err != nil {
		return SchemaStatus{}, errors.New("cannot read SQLite migration history")
	}
	defer rows.Close()
	current := 0
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return SchemaStatus{}, errors.New("cannot read SQLite migration record")
		}
		if version <= 0 || version != current+1 {
			return SchemaStatus{}, errors.New("SQLite migration history is not contiguous")
		}
		if version > latest {
			return SchemaStatus{}, ErrSchemaTooNew
		}
		if checksum != migrationList[version-1].Checksum {
			return SchemaStatus{}, ErrSchemaChecksum
		}
		current = version
	}
	if err := rows.Err(); err != nil {
		return SchemaStatus{}, errors.New("cannot read SQLite migration history")
	}
	if current == 0 {
		return SchemaStatus{}, errors.New("SQLite migration history is empty")
	}
	expectedTables := tablesFromMigrations(migrationList[:current])
	if !equalStrings(actualTables, expectedTables) {
		return SchemaStatus{}, errors.New("SQLite application schema does not match migration history")
	}
	return SchemaStatus{
		CurrentVersion: current,
		LatestVersion:  latest,
		NeedsMigration: current < latest,
	}, nil
}

// Migrate applies all outstanding embedded migrations in one transaction.
func (s *Store) Migrate(ctx context.Context) (SchemaStatus, error) {
	status, err := s.InspectSchema(ctx)
	if err != nil || !status.NeedsMigration {
		return status, err
	}
	migrationList, err := migrations.List()
	if err != nil {
		return SchemaStatus{}, errors.New("cannot read embedded SQLite migrations")
	}
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SchemaStatus{}, errors.New("cannot begin SQLite migration transaction")
	}
	defer transaction.Rollback()
	for _, migration := range migrationList[status.CurrentVersion:] {
		if _, err := transaction.ExecContext(ctx, migration.SQL); err != nil {
			return SchemaStatus{}, errors.New("cannot apply SQLite migration")
		}
		if _, err := transaction.ExecContext(ctx,
			`INSERT INTO schema_migration(version, applied_at, checksum) VALUES (?, ?, ?)`,
			migration.Version,
			time.Now().UTC().Unix(),
			migration.Checksum,
		); err != nil {
			return SchemaStatus{}, errors.New("cannot record SQLite migration")
		}
	}
	if err := transaction.Commit(); err != nil {
		return SchemaStatus{}, errors.New("cannot commit SQLite migration")
	}
	if err := s.QuickCheck(ctx); err != nil {
		return SchemaStatus{}, err
	}
	return s.InspectSchema(ctx)
}

func (s *Store) applicationTables(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name
		FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return nil, errors.New("cannot inspect SQLite application tables")
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, errors.New("cannot inspect SQLite application table")
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("cannot inspect SQLite application tables")
	}
	return tables, nil
}

func tablesFromMigrations(migrationList []migrations.Migration) []string {
	set := make(map[string]struct{})
	for _, migration := range migrationList {
		for _, match := range createTablePattern.FindAllStringSubmatch(migration.SQL, -1) {
			set[strings.ToLower(match[1])] = struct{}{}
		}
	}
	tables := make([]string, 0, len(set))
	for table := range set {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}

func containsString(values []string, target string) bool {
	index := sort.SearchStrings(values, target)
	return index < len(values) && values[index] == target
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
