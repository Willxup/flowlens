package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"modernc.org/sqlite"
)

type onlineBackuper interface {
	NewBackup(string) (*sqlite.Backup, error)
}

// OnlineBackup creates one independent SQLite snapshot without stopping writes.
func (s *Store) OnlineBackup(ctx context.Context, destination string) error {
	if err := validateBackupDestination(s.databasePath, destination); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cleanup := func() {
		_ = os.Remove(destination)
		_ = os.Remove(destination + "-wal")
		_ = os.Remove(destination + "-shm")
	}
	destinationFile, err := os.OpenFile(destination, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("cannot create FlowLens backup destination")
	}
	if err := destinationFile.Close(); err != nil {
		cleanup()
		return errors.New("cannot close FlowLens backup destination")
	}
	connection, err := s.db.Conn(ctx)
	if err != nil {
		cleanup()
		return errors.New("cannot acquire FlowLens backup connection")
	}
	defer connection.Close()
	if err := connection.Raw(func(driverConnection any) error {
		backuper, ok := driverConnection.(onlineBackuper)
		if !ok {
			return errors.New("SQLite online backup is unavailable")
		}
		backup, err := backuper.NewBackup(destination)
		if err != nil {
			return errors.New("cannot initialize SQLite online backup")
		}
		for {
			more, stepErr := backup.Step(-1)
			if stepErr != nil {
				_ = backup.Finish()
				return errors.New("cannot copy SQLite online backup")
			}
			if !more {
				break
			}
		}
		if err := backup.Finish(); err != nil {
			return errors.New("cannot finish SQLite online backup")
		}
		return nil
	}); err != nil {
		cleanup()
		return errors.New("cannot create FlowLens online backup")
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		cleanup()
		return errors.New("cannot secure FlowLens online backup")
	}
	return nil
}

func validateBackupDestination(source, destination string) error {
	if destination == "" || !filepath.IsAbs(destination) || filepath.Clean(destination) != destination ||
		destination == source {
		return errors.New("invalid FlowLens backup destination")
	}
	info, err := os.Stat(filepath.Dir(destination))
	if err != nil || !info.IsDir() {
		return errors.New("FlowLens backup directory is unavailable")
	}
	if _, err := os.Lstat(destination); err == nil || !errors.Is(err, os.ErrNotExist) {
		return errors.New("FlowLens backup destination already exists")
	}
	return nil
}
