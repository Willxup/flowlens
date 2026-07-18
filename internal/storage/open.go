package storage

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// Options configures the SQLite storage location.
type Options struct {
	DatabasePath   string
	SoftLimitBytes int64
}

// String prevents database path disclosure through ordinary formatting.
func (Options) String() string {
	return "StorageOptions{redacted}"
}

// GoString prevents database path disclosure through Go-syntax formatting.
func (options Options) GoString() string {
	return options.String()
}

// Store owns one SQLite pool and its exclusive data-directory lock.
type Store struct {
	db             *sql.DB
	lock           *directoryLock
	databasePath   string
	softLimitBytes int64
	capacityMu     sync.Mutex
	protecting     bool

	closeOnce sync.Once
	closeErr  error
}

// String prevents database path disclosure through ordinary formatting.
func (*Store) String() string {
	return "SQLiteStore{redacted}"
}

// GoString prevents database path disclosure through Go-syntax formatting.
func (s *Store) GoString() string {
	return s.String()
}

// Open acquires the data-directory lock and opens a policy-constrained SQLite
// pool. It does not apply application migrations.
func Open(ctx context.Context, options Options) (*Store, error) {
	if options.SoftLimitBytes < 0 {
		return nil, errors.New("SQLite soft limit must not be negative")
	}
	if err := validateDatabasePath(options.DatabasePath); err != nil {
		return nil, err
	}
	lock, err := acquireDirectoryLock(options.DatabasePath)
	if err != nil {
		return nil, err
	}

	database, err := sql.Open("sqlite", sqliteDSN(options.DatabasePath))
	if err != nil {
		_ = lock.close()
		return nil, errors.New("cannot open FlowLens SQLite database")
	}
	database.SetMaxOpenConns(4)
	database.SetMaxIdleConns(4)
	store := &Store{
		db: database, lock: lock, databasePath: options.DatabasePath, softLimitBytes: options.SoftLimitBytes,
	}
	if err := database.PingContext(ctx); err != nil {
		_ = store.Close()
		return nil, errors.New("cannot connect to FlowLens SQLite database")
	}
	if err := store.verifyConnectionPolicy(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

// Close releases the SQLite pool and lock exactly once.
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		databaseErr := s.db.Close()
		lockErr := s.lock.close()
		if databaseErr != nil || lockErr != nil {
			s.closeErr = errors.New("cannot close FlowLens SQLite store")
		}
	})
	return s.closeErr
}

func validateDatabasePath(databasePath string) error {
	if databasePath == "" || !filepath.IsAbs(databasePath) || filepath.Clean(databasePath) != databasePath {
		return errors.New("SQLite database path must be a clean absolute path")
	}
	directory := filepath.Dir(databasePath)
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return errors.New("SQLite database directory is unavailable")
	}
	info, err = os.Lstat(databasePath)
	if err == nil && !info.Mode().IsRegular() {
		return errors.New("SQLite database path must be a regular file")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect SQLite database path")
	}
	return nil
}

func sqliteDSN(databasePath string) string {
	uri := url.URL{Scheme: "file", Path: databasePath}
	query := uri.Query()
	for _, pragma := range []string{
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"foreign_keys(ON)",
		"busy_timeout(5000)",
		"auto_vacuum(INCREMENTAL)",
		"temp_store(MEMORY)",
	} {
		query.Add("_pragma", pragma)
	}
	query.Set("_txlock", "immediate")
	uri.RawQuery = query.Encode()
	return uri.String()
}
