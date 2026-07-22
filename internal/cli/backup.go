package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Willxup/flowlens/internal/backup"
	"github.com/Willxup/flowlens/internal/buildinfo"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/storage"
)

func runBackup(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	fail := func() int {
		fmt.Fprintln(stderr, "FlowLens backup failed")
		return 1
	}
	loadConfig := dependencies.LoadConfig
	if loadConfig == nil {
		loadConfig = config.Load
	}
	cfg, err := loadConfig()
	if err != nil {
		return fail()
	}
	info, err := os.Lstat(cfg.Storage.DatabasePath)
	if err != nil || !info.Mode().IsRegular() {
		return fail()
	}
	store, err := storage.Open(ctx, storage.Options{
		DatabasePath: cfg.Storage.DatabasePath, SoftLimitBytes: int64(cfg.Storage.SoftLimit),
	})
	if err != nil {
		return fail()
	}
	ok := false
	if err := store.QuickCheck(ctx); err == nil {
		status, inspectErr := store.InspectSchema(ctx)
		ok = inspectErr == nil && status.CurrentVersion > 0 &&
			status.CurrentVersion == status.LatestVersion && !status.NeedsMigration
	}
	if !ok {
		_ = store.Close()
		return fail()
	}
	now := dependencies.Now
	if now == nil {
		now = time.Now
	}
	_, createErr := backup.Create(ctx, backup.Options{
		Store: store, Directory: cfg.Backup.Directory,
		DailyKeep: cfg.Backup.DailyKeep, MonthlyKeep: cfg.Backup.MonthlyKeep,
		BucketTimezone: cfg.Time.Timezone, ApplicationVersion: buildinfo.Version,
	}, now())
	pruneErr := error(nil)
	if createErr == nil {
		pruneErr = backup.Prune(cfg.Backup.Directory, cfg.Backup.DailyKeep, cfg.Backup.MonthlyKeep)
	}
	closeErr := store.Close()
	if createErr != nil || pruneErr != nil || closeErr != nil {
		return fail()
	}
	fmt.Fprintln(stdout, "backup: ok")
	return 0
}
