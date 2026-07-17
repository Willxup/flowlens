package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

// ErrLocked means another FlowLens process owns the data-directory lock.
var ErrLocked = errors.New("FlowLens data directory is already locked")

type directoryLock struct {
	file *os.File

	closeOnce sync.Once
	closeErr  error
}

func acquireDirectoryLock(databasePath string) (*directoryLock, error) {
	lockPath := filepath.Join(filepath.Dir(databasePath), ".flowlens.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errors.New("cannot open FlowLens data-directory lock")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, errors.New("cannot secure FlowLens data-directory lock")
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrLocked
		}
		return nil, errors.New("cannot acquire FlowLens data-directory lock")
	}
	return &directoryLock{file: file}, nil
}

func (l *directoryLock) close() error {
	l.closeOnce.Do(func() {
		unlockErr := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
		fileErr := l.file.Close()
		if unlockErr != nil || fileErr != nil {
			l.closeErr = errors.New("cannot release FlowLens data-directory lock")
		}
	})
	return l.closeErr
}
