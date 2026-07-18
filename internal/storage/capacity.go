package storage

import (
	"context"
	"errors"
	"os"
)

// CapacityStatus is the non-sensitive storage-size and protection snapshot.
type CapacityStatus struct {
	DatabaseBytes  int64
	WALBytes       int64
	SoftLimitBytes int64
	Protecting     bool
}

// CapacityStatus returns the current file sizes and committed protection mode.
func (s *Store) CapacityStatus(ctx context.Context) (CapacityStatus, error) {
	if err := ctx.Err(); err != nil {
		return CapacityStatus{}, err
	}
	s.capacityMu.Lock()
	defer s.capacityMu.Unlock()
	return s.capacityStatusUnlocked()
}

func (s *Store) beginCapacityTransition() (CapacityStatus, bool, error) {
	s.capacityMu.Lock()
	status, err := s.capacityStatusUnlocked()
	if err != nil {
		s.capacityMu.Unlock()
		return CapacityStatus{}, false, err
	}
	next := status.Protecting
	if status.SoftLimitBytes == 0 {
		next = false
	} else {
		total := status.DatabaseBytes + status.WALBytes
		if status.Protecting {
			next = total >= status.SoftLimitBytes-status.SoftLimitBytes/5
		} else {
			next = total >= status.SoftLimitBytes
		}
	}
	return status, next, nil
}

func (s *Store) capacityStatusUnlocked() (CapacityStatus, error) {
	databaseBytes, err := requiredFileSize(s.databasePath)
	if err != nil {
		return CapacityStatus{}, errors.New("cannot inspect FlowLens database capacity")
	}
	walBytes, err := optionalFileSize(s.databasePath + "-wal")
	if err != nil {
		return CapacityStatus{}, errors.New("cannot inspect FlowLens WAL capacity")
	}
	return CapacityStatus{
		DatabaseBytes: databaseBytes, WALBytes: walBytes,
		SoftLimitBytes: s.softLimitBytes, Protecting: s.protecting,
	}, nil
}

func requiredFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0, errors.New("required file is unavailable")
	}
	return info.Size(), nil
}

func optionalFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return 0, errors.New("optional file is unavailable")
	}
	return info.Size(), nil
}
