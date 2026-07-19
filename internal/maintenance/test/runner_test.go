package maintenance_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/backup"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/maintenance"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestCatchUpRollupsProcessesOldestWindowsInPlannedEdgeOrder(t *testing.T) {
	store := &recordingRollupStore{pending: map[string][]rollup.Window{
		edgeKey(rollup.ResolutionTenSeconds, rollup.ResolutionMinute): {
			window(60, rollup.ResolutionMinute),
			window(120, rollup.ResolutionMinute),
		},
		edgeKey(rollup.ResolutionMinute, rollup.ResolutionHalfHour): {
			window(1800, rollup.ResolutionHalfHour),
		},
		edgeKey(rollup.ResolutionHalfHour, rollup.ResolutionHour): {
			window(3600, rollup.ResolutionHour),
		},
		edgeKey(rollup.ResolutionMinute, rollup.ResolutionDay): {
			window(86400, rollup.ResolutionDay),
		},
	}}
	runner := newRunner(t, store)

	if err := runner.CatchUpRollups(context.Background(), time.Unix(200000, 0)); err != nil {
		t.Fatalf("CatchUpRollups() error = %v", err)
	}
	want := []string{
		"10:60:60", "10:60:120", "60:1800:1800", "1800:3600:3600", "60:86400:86400",
	}
	if !reflect.DeepEqual(store.calls, want) {
		t.Fatalf("rollup calls = %#v, want %#v", store.calls, want)
	}
	if err := runner.CatchUpRollups(context.Background(), time.Unix(200000, 0)); err != nil {
		t.Fatalf("second CatchUpRollups() error = %v", err)
	}
	if !reflect.DeepEqual(store.calls, want) {
		t.Fatalf("second rollup calls = %#v, want unchanged %#v", store.calls, want)
	}
}

func TestCatchUpRollupsStopsAtFirstStorageFailure(t *testing.T) {
	failingEdge := edgeKey(rollup.ResolutionMinute, rollup.ResolutionHalfHour)
	store := &recordingRollupStore{
		pending: map[string][]rollup.Window{
			edgeKey(rollup.ResolutionTenSeconds, rollup.ResolutionMinute): {
				window(60, rollup.ResolutionMinute),
			},
			failingEdge: {
				window(1800, rollup.ResolutionHalfHour),
			},
			edgeKey(rollup.ResolutionHalfHour, rollup.ResolutionHour): {
				window(3600, rollup.ResolutionHour),
			},
		},
		failEdge: failingEdge,
	}
	runner := newRunner(t, store)

	err := runner.CatchUpRollups(context.Background(), time.Unix(200000, 0))
	if !errors.Is(err, maintenance.ErrRollupMaintenance) {
		t.Fatalf("CatchUpRollups() error = %v, want ErrRollupMaintenance", err)
	}
	want := []string{"10:60:60", "60:1800:1800"}
	if !reflect.DeepEqual(store.calls, want) {
		t.Fatalf("rollup calls = %#v, want %#v", store.calls, want)
	}
}

func TestCatchUpRollupsHonorsCancellationAndRejectsInvalidOptions(t *testing.T) {
	if _, err := maintenance.New(maintenance.Options{Location: time.UTC}); !errors.Is(err, maintenance.ErrInvalidOptions) {
		t.Fatalf("New(nil store) error = %v", err)
	}
	store := &recordingRollupStore{pending: map[string][]rollup.Window{}}
	if _, err := maintenance.New(maintenance.Options{Store: store}); !errors.Is(err, maintenance.ErrInvalidOptions) {
		t.Fatalf("New(nil location) error = %v", err)
	}
	runner := newRunner(t, store)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.CatchUpRollups(ctx, time.Unix(200000, 0)); !errors.Is(err, context.Canceled) {
		t.Fatalf("CatchUpRollups() error = %v, want context.Canceled", err)
	}
	if len(store.calls) != 0 {
		t.Fatalf("canceled rollup calls = %#v", store.calls)
	}
}

func TestRunOnceRollsUpBeforeCleanupAndRecordsResult(t *testing.T) {
	store := &recordingRollupStore{
		pending: map[string][]rollup.Window{
			edgeKey(rollup.ResolutionTenSeconds, rollup.ResolutionMinute): {
				window(60, rollup.ResolutionMinute),
			},
		},
		cleanupResult: storage.CleanupResult{
			DeletedTenSecond: 1,
			DeletedMinute:    2,
			DeletedHalfHour:  3,
			DeletedHour:      4,
		},
	}
	runner := newRunner(t, store)
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	retention := config.Retention{TenSecondDays: 1, MinuteDays: 2, HalfHourDays: 3, HourDays: 4}

	if err := runner.RunOnce(context.Background(), now, retention); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	wantCalls := []string{"maintenance_cleanup", "quality_cleanup", "10:60:60", "cleanup", "record"}
	if !reflect.DeepEqual(store.calls, wantCalls) {
		t.Fatalf("RunOnce() calls = %#v, want %#v", store.calls, wantCalls)
	}
	wantCutoffs := storage.RetentionCutoffs{
		TenSecondBefore: now.AddDate(0, 0, -1).Unix(),
		MinuteBefore:    now.AddDate(0, 0, -2).Unix(),
		HalfHourBefore:  now.AddDate(0, 0, -3).Unix(),
		HourBefore:      now.AddDate(0, 0, -4).Unix(),
	}
	if store.cleanupCutoffs != wantCutoffs {
		t.Fatalf("RunOnce() cutoffs = %#v, want %#v", store.cleanupCutoffs, wantCutoffs)
	}
	if store.qualityBefore != now.AddDate(-3, 0, 0).Unix() {
		t.Fatalf("RunOnce() quality cutoff = %d, want %d", store.qualityBefore, now.AddDate(-3, 0, 0).Unix())
	}
	if len(store.runs) != 1 || store.runs[0].Operation != "rollup_cleanup" ||
		store.runs[0].DeletedRows != 10 || store.runs[0].EndedAt == nil ||
		*store.runs[0].EndedAt != now.Unix() {
		t.Fatalf("maintenance runs = %#v", store.runs)
	}
}

func TestRunScheduledPerformsHourlyCheckpointAndOneCurrentBackup(t *testing.T) {
	store := &recordingRollupStore{
		pending:     map[string][]rollup.Window{},
		capacity:    storage.CapacityStatus{WALBytes: 65 << 20},
		maintenance: make(map[string]storage.MaintenanceRun),
	}
	var backupTimes []time.Time
	options := maintenance.Options{
		Store: store, Location: time.UTC,
		Retention: config.Retention{TenSecondDays: 1, MinuteDays: 7, HalfHourDays: 365, HourDays: 1095},
		Backup: backup.Options{
			Directory: t.TempDir(), DailyKeep: 3, MonthlyKeep: 3,
			BucketTimezone: "UTC", ApplicationVersion: "0.1.0-test",
		},
		BackupTime: config.ClockTime{Hour: 4},
		CreateBackup: func(ctx context.Context, options backup.Options, at time.Time) (backup.Artifact, error) {
			backupTimes = append(backupTimes, at)
			return backup.Artifact{}, nil
		},
	}
	runner, err := maintenance.New(options)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	now := time.Date(2026, time.July, 18, 5, 0, 0, 0, time.UTC)

	if err := runner.RunScheduled(context.Background(), now); err != nil {
		t.Fatalf("RunScheduled() error = %v", err)
	}
	if !reflect.DeepEqual(store.checkpoints, []bool{true}) || store.optimizeCalls != 1 ||
		!reflect.DeepEqual(store.vacuumPages, []int64{1024}) || len(backupTimes) != 1 || !backupTimes[0].Equal(now) {
		t.Fatalf("scheduled state = checkpoints:%#v optimize:%d vacuum:%#v backups:%#v",
			store.checkpoints, store.optimizeCalls, store.vacuumPages, backupTimes)
	}
	if err := runner.RunScheduled(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatalf("second RunScheduled() error = %v", err)
	}
	if len(store.checkpoints) != 1 || store.optimizeCalls != 1 || len(store.vacuumPages) != 1 || len(backupTimes) != 1 {
		t.Fatalf("duplicate scheduled work = checkpoints:%#v optimize:%d vacuum:%#v backups:%#v",
			store.checkpoints, store.optimizeCalls, store.vacuumPages, backupTimes)
	}
	if next := runner.NextWake(now.Add(10 * time.Second)); !next.Equal(now.Add(time.Minute)) {
		t.Fatalf("NextWake() = %s, want %s", next, now.Add(time.Minute))
	}
}

func TestRunScheduledRetriesPruneWithoutCreatingAnotherSnapshot(t *testing.T) {
	store := &recordingRollupStore{
		pending: map[string][]rollup.Window{}, maintenance: make(map[string]storage.MaintenanceRun),
	}
	badDirectory := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badDirectory, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	backupCalls := 0
	runner, err := maintenance.New(maintenance.Options{
		Store: store, Location: time.UTC,
		Retention: config.Retention{TenSecondDays: 1, MinuteDays: 7, HalfHourDays: 365, HourDays: 1095},
		Backup: backup.Options{
			Directory: badDirectory, DailyKeep: 3, MonthlyKeep: 3,
			BucketTimezone: "UTC", ApplicationVersion: "0.1.0-test",
		},
		BackupTime: config.ClockTime{Hour: 4},
		FindBackup: func(context.Context, string, time.Time, string) (bool, error) {
			return false, nil
		},
		CreateBackup: func(context.Context, backup.Options, time.Time) (backup.Artifact, error) {
			backupCalls++
			return backup.Artifact{}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	now := time.Date(2026, time.July, 18, 5, 0, 0, 0, time.UTC)
	if err := runner.RunScheduled(context.Background(), now); !errors.Is(err, maintenance.ErrScheduledMaintenance) {
		t.Fatalf("first RunScheduled() error = %v", err)
	}
	if err := runner.RunScheduled(context.Background(), now.Add(time.Minute)); !errors.Is(err, maintenance.ErrScheduledMaintenance) {
		t.Fatalf("second RunScheduled() error = %v", err)
	}
	if backupCalls != 1 {
		t.Fatalf("backup calls = %d, want 1", backupCalls)
	}
	if run, found := store.maintenance["backup"]; !found || run.EndedAt == nil || run.Error != nil {
		t.Fatalf("backup maintenance = %#v, found=%t", run, found)
	}
}

func TestRunScheduledRecognizesCommittedBackupWhenBookkeepingRetryIsNeeded(t *testing.T) {
	store := &recordingRollupStore{
		pending:        map[string][]rollup.Window{},
		maintenance:    make(map[string]storage.MaintenanceRun),
		failRecordOnce: "backup",
	}
	findCalls := 0
	backupCalls := 0
	runner, err := maintenance.New(maintenance.Options{
		Store: store, Location: time.UTC,
		Retention: config.Retention{TenSecondDays: 1, MinuteDays: 7, HalfHourDays: 365, HourDays: 1095},
		Backup: backup.Options{
			Directory: t.TempDir(), DailyKeep: 3, MonthlyKeep: 3,
			BucketTimezone: "UTC", ApplicationVersion: "0.1.0-test",
		},
		BackupTime: config.ClockTime{Hour: 4},
		FindBackup: func(context.Context, string, time.Time, string) (bool, error) {
			findCalls++
			return findCalls > 1, nil
		},
		CreateBackup: func(context.Context, backup.Options, time.Time) (backup.Artifact, error) {
			backupCalls++
			return backup.Artifact{}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	now := time.Date(2026, time.July, 18, 5, 0, 0, 0, time.UTC)
	if err := runner.RunScheduled(context.Background(), now); !errors.Is(err, maintenance.ErrScheduledMaintenance) {
		t.Fatalf("first RunScheduled() error = %v", err)
	}
	if err := runner.RunScheduled(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatalf("second RunScheduled() error = %v", err)
	}
	if findCalls != 2 || backupCalls != 1 || store.optimizeCalls != 1 ||
		!reflect.DeepEqual(store.vacuumPages, []int64{1024}) {
		t.Fatalf("backup retry state = finds:%d backups:%d optimize:%d vacuum:%#v",
			findCalls, backupCalls, store.optimizeCalls, store.vacuumPages)
	}
	if run, found := store.maintenance["backup"]; !found || run.EndedAt == nil || run.Error != nil {
		t.Fatalf("backup maintenance = %#v, found=%t", run, found)
	}
	if run, found := store.maintenance["backup_retention"]; !found || run.EndedAt == nil || run.Error != nil {
		t.Fatalf("backup retention = %#v, found=%t", run, found)
	}
}

type recordingRollupStore struct {
	pending        map[string][]rollup.Window
	calls          []string
	failEdge       string
	cleanupResult  storage.CleanupResult
	cleanupCutoffs storage.RetentionCutoffs
	qualityBefore  int64
	runs           []storage.MaintenanceRun
	capacity       storage.CapacityStatus
	maintenance    map[string]storage.MaintenanceRun
	checkpoints    []bool
	optimizeCalls  int
	vacuumPages    []int64
	failRecordOnce string
}

func (s *recordingRollupStore) CleanupTraffic(
	ctx context.Context,
	cutoffs storage.RetentionCutoffs,
	location *time.Location,
) (storage.CleanupResult, error) {
	s.calls = append(s.calls, "cleanup")
	s.cleanupCutoffs = cutoffs
	return s.cleanupResult, nil
}

func (s *recordingRollupStore) CleanupMaintenance(context.Context, int64) (int64, error) {
	s.calls = append(s.calls, "maintenance_cleanup")
	return 0, nil
}

func (s *recordingRollupStore) CleanupQualityEvents(_ context.Context, before int64) (int64, error) {
	s.calls = append(s.calls, "quality_cleanup")
	s.qualityBefore = before
	return 0, nil
}

func (s *recordingRollupStore) RecordMaintenance(
	ctx context.Context,
	run storage.MaintenanceRun,
) error {
	s.calls = append(s.calls, "record")
	s.runs = append(s.runs, run)
	if s.failRecordOnce == run.Operation {
		s.failRecordOnce = ""
		return errors.New("fixture maintenance record failure")
	}
	if s.maintenance != nil {
		s.maintenance[run.Operation] = run
	}
	return nil
}

func (s *recordingRollupStore) CapacityStatus(context.Context) (storage.CapacityStatus, error) {
	return s.capacity, nil
}

func (s *recordingRollupStore) LatestMaintenance(
	ctx context.Context,
	operation string,
) (storage.MaintenanceRun, bool, error) {
	run, found := s.maintenance[operation]
	return run, found, nil
}

func (s *recordingRollupStore) Checkpoint(ctx context.Context, truncate bool) error {
	s.checkpoints = append(s.checkpoints, truncate)
	return nil
}

func (s *recordingRollupStore) Optimize(context.Context) error {
	s.optimizeCalls++
	return nil
}

func (s *recordingRollupStore) IncrementalVacuum(ctx context.Context, pages int64) error {
	s.vacuumPages = append(s.vacuumPages, pages)
	return nil
}

func (s *recordingRollupStore) NextTrafficRollupWindow(
	ctx context.Context,
	sourceResolutionSec int64,
	targetResolutionSec int64,
	completeBefore int64,
	location *time.Location,
) (rollup.Window, bool, error) {
	if err := ctx.Err(); err != nil {
		return rollup.Window{}, false, err
	}
	windows := s.pending[edgeKey(sourceResolutionSec, targetResolutionSec)]
	if len(windows) == 0 {
		return rollup.Window{}, false, nil
	}
	return windows[0], true, nil
}

func (s *recordingRollupStore) RollupTraffic(
	ctx context.Context,
	sourceResolutionSec int64,
	window rollup.Window,
	topK ...int,
) (storage.TrafficRollup, error) {
	key := edgeKey(sourceResolutionSec, window.ResolutionSec)
	s.calls = append(s.calls, fmt.Sprintf("%s:%d", key, window.BucketStart))
	if key == s.failEdge {
		return storage.TrafficRollup{}, errors.New("fixture storage failure")
	}
	s.pending[key] = s.pending[key][1:]
	return storage.TrafficRollup{}, nil
}

func newRunner(t *testing.T, store maintenance.LifecycleStore) *maintenance.Runner {
	t.Helper()
	runner, err := maintenance.New(maintenance.Options{Store: store, Location: time.UTC})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return runner
}

func edgeKey(source, target int64) string {
	return fmt.Sprintf("%d:%d", source, target)
}

func window(start, resolution int64) rollup.Window {
	end := start + resolution
	if resolution == rollup.ResolutionDay {
		end = start + 24*60*60
	}
	return rollup.Window{ResolutionSec: resolution, BucketStart: start, BucketEnd: end}
}
