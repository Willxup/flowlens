package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Willxup/flowlens/internal/storage"
)

// CreatePreUpgrade creates and verifies one temporary uncompressed snapshot.
func CreatePreUpgrade(
	ctx context.Context,
	store *storage.Store,
	directory string,
	createdAt time.Time,
) (string, error) {
	if store == nil || createdAt.Unix() <= 0 || directory == "" || !filepath.IsAbs(directory) ||
		filepath.Clean(directory) != directory {
		return "", errors.New("invalid FlowLens pre-upgrade backup options")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return "", errors.New("FlowLens pre-upgrade backup directory is unavailable")
	}
	path := filepath.Join(directory, "flowlens-pre-upgrade-"+createdAt.UTC().Format("20060102T150405Z")+".db")
	if err := store.OnlineBackup(ctx, path); err != nil {
		return "", errors.New("cannot create FlowLens pre-upgrade backup")
	}
	valid := false
	defer func() {
		if !valid {
			_ = os.Remove(path)
			_ = os.Remove(path + "-wal")
			_ = os.Remove(path + "-shm")
		}
	}()
	if _, err := inspectDatabase(path); err != nil {
		return "", errors.New("cannot validate FlowLens pre-upgrade backup")
	}
	if err := syncFile(path); err != nil {
		return "", err
	}
	if err := syncDirectory(directory); err != nil {
		return "", err
	}
	valid = true
	return path, nil
}

// RemovePreUpgrade removes one successfully superseded temporary snapshot.
func RemovePreUpgrade(path string) error {
	base := filepath.Base(path)
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path ||
		!strings.HasPrefix(base, "flowlens-pre-upgrade-") || !strings.HasSuffix(base, ".db") {
		return errors.New("invalid FlowLens pre-upgrade backup path")
	}
	if err := os.Remove(path); err != nil {
		return errors.New("cannot remove FlowLens pre-upgrade backup")
	}
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	return syncDirectory(filepath.Dir(path))
}
