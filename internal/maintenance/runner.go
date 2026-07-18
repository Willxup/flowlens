package maintenance

import (
	"context"
	"errors"
	"time"

	"github.com/Willxup/flowlens/internal/backup"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

var (
	ErrInvalidOptions       = errors.New("invalid FlowLens maintenance options")
	ErrRollupMaintenance    = errors.New("FlowLens rollup maintenance failed")
	ErrRetentionMaintenance = errors.New("FlowLens retention maintenance failed")
	ErrScheduledMaintenance = errors.New("FlowLens scheduled maintenance failed")
)

// RollupStore is the exact durable boundary needed by the catch-up runner.
type RollupStore interface {
	NextTrafficRollupWindow(
		context.Context,
		int64,
		int64,
		int64,
		*time.Location,
	) (rollup.Window, bool, error)
	RollupTraffic(context.Context, int64, rollup.Window) (storage.TrafficRollup, error)
}

// MaintenanceStore extends rollup storage with the exact cleanup/audit calls
// needed by one serial maintenance run.
type MaintenanceStore interface {
	RollupStore
	CleanupTraffic(context.Context, storage.RetentionCutoffs, *time.Location) (storage.CleanupResult, error)
	CleanupMaintenance(context.Context, int64) (int64, error)
	RecordMaintenance(context.Context, storage.MaintenanceRun) error
}

// LifecycleStore adds the exact hourly/daily primitives used by RunScheduled.
type LifecycleStore interface {
	MaintenanceStore
	CapacityStatus(context.Context) (storage.CapacityStatus, error)
	LatestMaintenance(context.Context, string) (storage.MaintenanceRun, bool, error)
	Checkpoint(context.Context, bool) error
	Optimize(context.Context) error
	IncrementalVacuum(context.Context, int64) error
}

// BackupCreator is injectable only to keep lifecycle tests deterministic.
type BackupCreator func(context.Context, backup.Options, time.Time) (backup.Artifact, error)

// Options configures serial rollup catch-up.
type Options struct {
	Store        LifecycleStore
	Location     *time.Location
	Retention    config.Retention
	Backup       backup.Options
	BackupTime   config.ClockTime
	CreateBackup BackupCreator
}

// Runner owns the Stage 2 maintenance order.
type Runner struct {
	store        LifecycleStore
	location     *time.Location
	retention    config.Retention
	backup       backup.Options
	backupTime   config.ClockTime
	createBackup BackupCreator
}

type edge struct {
	source int64
	target int64
}

var rollupEdges = [...]edge{
	{source: rollup.ResolutionTenSeconds, target: rollup.ResolutionMinute},
	{source: rollup.ResolutionMinute, target: rollup.ResolutionHalfHour},
	{source: rollup.ResolutionHalfHour, target: rollup.ResolutionHour},
	{source: rollup.ResolutionMinute, target: rollup.ResolutionDay},
}

// New validates the minimal maintenance dependencies.
func New(options Options) (*Runner, error) {
	if options.Store == nil || options.Location == nil {
		return nil, ErrInvalidOptions
	}
	creator := options.CreateBackup
	if creator == nil {
		creator = backup.Create
	}
	return &Runner{
		store: options.Store, location: options.Location,
		retention: options.Retention, backup: options.Backup,
		backupTime: options.BackupTime, createBackup: creator,
	}, nil
}

// CatchUpRollups processes every fully elapsed missing global bucket from the
// oldest gap forward, one transaction at a time.
func (r *Runner) CatchUpRollups(ctx context.Context, completeBefore time.Time) error {
	if completeBefore.Unix() <= 0 {
		return ErrRollupMaintenance
	}
	for _, edge := range rollupEdges {
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			window, found, err := r.store.NextTrafficRollupWindow(
				ctx,
				edge.source,
				edge.target,
				completeBefore.Unix(),
				r.location,
			)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return ErrRollupMaintenance
			}
			if !found {
				break
			}
			if _, err := r.store.RollupTraffic(ctx, edge.source, window); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return ErrRollupMaintenance
			}
		}
	}
	return nil
}

// RunOnce catches up higher buckets before deleting any eligible source rows.
func (r *Runner) RunOnce(ctx context.Context, now time.Time, retention config.Retention) error {
	if now.Unix() <= 0 || retention.TenSecondDays <= 0 || retention.MinuteDays <= 0 ||
		retention.HalfHourDays <= 0 || retention.HourDays <= 0 {
		return ErrInvalidOptions
	}
	localNow := now.In(r.location)
	if _, err := r.store.CleanupMaintenance(ctx, localNow.AddDate(0, 0, -90).Unix()); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrRetentionMaintenance
	}
	run := storage.MaintenanceRun{Operation: "rollup_cleanup", StartedAt: now.Unix()}
	recordFailure := func(code string, result error) error {
		run.Error = &code
		_ = r.store.RecordMaintenance(ctx, run)
		return result
	}
	if err := r.CatchUpRollups(ctx, now); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return recordFailure("rollup_failed", err)
	}
	cutoffs := storage.RetentionCutoffs{
		TenSecondBefore: localNow.AddDate(0, 0, -retention.TenSecondDays).Unix(),
		MinuteBefore:    localNow.AddDate(0, 0, -retention.MinuteDays).Unix(),
		HalfHourBefore:  localNow.AddDate(0, 0, -retention.HalfHourDays).Unix(),
		HourBefore:      localNow.AddDate(0, 0, -retention.HourDays).Unix(),
	}
	cleanup, err := r.store.CleanupTraffic(ctx, cutoffs, r.location)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return recordFailure("cleanup_failed", ErrRetentionMaintenance)
	}
	run.DeletedRows = cleanup.DeletedTenSecond + cleanup.DeletedMinute +
		cleanup.DeletedHalfHour + cleanup.DeletedHour
	endedAt := now.Unix()
	run.EndedAt = &endedAt
	if err := r.store.RecordMaintenance(ctx, run); err != nil {
		return ErrRetentionMaintenance
	}
	return nil
}

// RunScheduled performs serial rollup/cleanup, at-most-hourly checkpoint, and
// at-most-daily current backup work.
func (r *Runner) RunScheduled(ctx context.Context, now time.Time) error {
	if !validSchedule(r.retention, r.backup, r.backupTime) {
		return ErrInvalidOptions
	}
	if err := r.RunOnce(ctx, now, r.retention); err != nil {
		return err
	}
	if err := r.runCheckpoint(ctx, now); err != nil {
		return err
	}
	if err := r.runBackup(ctx, now); err != nil {
		return err
	}
	return nil
}

// NextWake returns the next minute boundary for the single maintenance loop.
func (r *Runner) NextWake(now time.Time) time.Time {
	return now.Truncate(time.Minute).Add(time.Minute)
}

func (r *Runner) runCheckpoint(ctx context.Context, now time.Time) error {
	latest, found, err := r.store.LatestMaintenance(ctx, "checkpoint")
	if err != nil {
		return ErrScheduledMaintenance
	}
	hourStart := now.Truncate(time.Hour).Unix()
	if found && latest.StartedAt >= hourStart && latest.EndedAt != nil && latest.Error == nil {
		return nil
	}
	capacity, err := r.store.CapacityStatus(ctx)
	if err != nil {
		return r.recordScheduledFailure(ctx, now, "checkpoint", "capacity_failed")
	}
	if err := r.store.Checkpoint(ctx, capacity.WALBytes > 64<<20); err != nil {
		return r.recordScheduledFailure(ctx, now, "checkpoint", "checkpoint_failed")
	}
	return r.recordScheduledSuccess(ctx, now, "checkpoint", capacity, 0)
}

func (r *Runner) runBackup(ctx context.Context, now time.Time) error {
	latest, found, err := r.store.LatestMaintenance(ctx, "backup")
	if err != nil {
		return ErrScheduledMaintenance
	}
	if backupDue(now, r.location, r.backupTime, latest, found) {
		if err := r.store.Optimize(ctx); err != nil {
			return r.recordScheduledFailure(ctx, now, "backup", "optimize_failed")
		}
		if err := r.store.IncrementalVacuum(ctx, 1024); err != nil {
			return r.recordScheduledFailure(ctx, now, "backup", "vacuum_failed")
		}
		if _, err := r.createBackup(ctx, r.backup, now); err != nil {
			return r.recordScheduledFailure(ctx, now, "backup", "backup_failed")
		}
		capacity, err := r.store.CapacityStatus(ctx)
		if err != nil {
			return r.recordScheduledFailure(ctx, now, "backup", "capacity_failed")
		}
		if err := r.recordScheduledSuccess(ctx, now, "backup", capacity, 0); err != nil {
			return err
		}
		endedAt := now.Unix()
		latest = storage.MaintenanceRun{Operation: "backup", StartedAt: now.Unix(), EndedAt: &endedAt}
		found = true
	}
	if !found {
		return nil
	}
	return r.runBackupRetention(ctx, now, latest)
}

func (r *Runner) runBackupRetention(ctx context.Context, now time.Time, latestBackup storage.MaintenanceRun) error {
	latest, found, err := r.store.LatestMaintenance(ctx, "backup_retention")
	if err != nil {
		return ErrScheduledMaintenance
	}
	if found && latest.EndedAt != nil && latest.Error == nil && latest.StartedAt >= latestBackup.StartedAt {
		return nil
	}
	if err := backup.Prune(r.backup.Directory, r.backup.DailyKeep, r.backup.MonthlyKeep); err != nil {
		return r.recordScheduledFailure(ctx, now, "backup_retention", "prune_failed")
	}
	capacity, err := r.store.CapacityStatus(ctx)
	if err != nil {
		return r.recordScheduledFailure(ctx, now, "backup_retention", "capacity_failed")
	}
	return r.recordScheduledSuccess(ctx, now, "backup_retention", capacity, 0)
}

func (r *Runner) recordScheduledSuccess(
	ctx context.Context,
	now time.Time,
	operation string,
	capacity storage.CapacityStatus,
	deletedRows int64,
) error {
	endedAt := now.Unix()
	run := storage.MaintenanceRun{
		Operation: operation, StartedAt: now.Unix(), EndedAt: &endedAt,
		DeletedRows: deletedRows, DatabaseBytes: capacity.DatabaseBytes, WALBytes: capacity.WALBytes,
	}
	if err := r.store.RecordMaintenance(ctx, run); err != nil {
		return ErrScheduledMaintenance
	}
	return nil
}

func (r *Runner) recordScheduledFailure(
	ctx context.Context,
	now time.Time,
	operation string,
	code string,
) error {
	run := storage.MaintenanceRun{Operation: operation, StartedAt: now.Unix(), Error: &code}
	_ = r.store.RecordMaintenance(ctx, run)
	return ErrScheduledMaintenance
}

func validSchedule(retention config.Retention, options backup.Options, backupTime config.ClockTime) bool {
	return retention.TenSecondDays > 0 && retention.MinuteDays > 0 &&
		retention.HalfHourDays > 0 && retention.HourDays > 0 &&
		options.Directory != "" && options.DailyKeep > 0 && options.MonthlyKeep > 0 &&
		options.BucketTimezone != "" && options.ApplicationVersion != "" &&
		backupTime.Hour >= 0 && backupTime.Hour <= 23 && backupTime.Minute >= 0 && backupTime.Minute <= 59
}

func backupDue(
	now time.Time,
	location *time.Location,
	backupTime config.ClockTime,
	latest storage.MaintenanceRun,
	found bool,
) bool {
	if !found || latest.EndedAt == nil || latest.Error != nil {
		return true
	}
	localNow := now.In(location)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	last := time.Unix(latest.StartedAt, 0).In(location)
	lastDay := time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, location)
	if !lastDay.Before(today) {
		return false
	}
	scheduledToday := time.Date(
		localNow.Year(), localNow.Month(), localNow.Day(), backupTime.Hour, backupTime.Minute, 0, 0, location,
	)
	return !localNow.Before(scheduledToday) || lastDay.Before(today.AddDate(0, 0, -1))
}
