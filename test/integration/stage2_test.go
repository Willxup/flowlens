package integration_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/backup"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/maintenance"
	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestStage2RollupCleanupQueryAndBackupChain(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := storage.Open(ctx, storage.Options{
		DatabasePath: filepath.Join(root, "flowlens.db"), SoftLimitBytes: 256 << 20,
	})
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("storage.Close() error = %v", err)
		}
	})
	if _, err := store.Migrate(ctx); err != nil {
		t.Fatalf("storage.Migrate() error = %v", err)
	}
	now := time.Date(2026, time.July, 18, 5, 0, 0, 0, time.UTC)
	bucketStart := now.AddDate(0, 0, -2).Truncate(time.Minute).Unix()
	seedStage2Buckets(t, store, bucketStart)
	backupDirectory := filepath.Join(root, "backups")
	if err := os.MkdirAll(backupDirectory, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	retention := config.Retention{TenSecondDays: 1, MinuteDays: 7, HalfHourDays: 365, HourDays: 1095, TopK: 20}
	backupOptions := backup.Options{
		Store: store, Directory: backupDirectory, DailyKeep: 3, MonthlyKeep: 3,
		BucketTimezone: "UTC", ApplicationVersion: "0.1.0-test",
	}
	runner, err := maintenance.New(maintenance.Options{
		Store: store, Location: time.UTC, Retention: retention,
		Backup: backupOptions, BackupTime: config.ClockTime{Hour: 4},
	})
	if err != nil {
		t.Fatalf("maintenance.New() error = %v", err)
	}
	if err := runner.RunScheduled(ctx, now); err != nil {
		t.Fatalf("RunScheduled() error = %v", err)
	}
	if _, found, err := store.TrafficRollup(ctx, rollup.ResolutionTenSeconds, bucketStart); err != nil || found {
		t.Fatalf("ten-second source after cleanup = found:%t error:%v", found, err)
	}
	minute, found, err := store.TrafficRollup(ctx, rollup.ResolutionMinute, bucketStart)
	if err != nil || !found || minute.UploadBytes != 60 || minute.DownloadBytes != 120 {
		t.Fatalf("minute rollup = %#v, found:%t error:%v", minute, found, err)
	}
	service, err := query.NewService(store, func() time.Time { return now }, retention, time.UTC)
	if err != nil {
		t.Fatalf("query.NewService() error = %v", err)
	}
	series, err := service.Series(ctx, rollup.Range{From: bucketStart, To: bucketStart + 60})
	if err != nil || len(series.Points) != 1 || series.Points[0].ResolutionSec != rollup.ResolutionMinute {
		t.Fatalf("Series() = %#v, %v", series, err)
	}
	manifestPath := singleManifest(t, backupDirectory)
	if _, err := backup.Validate(ctx, manifestPath, backup.ValidationPolicy{
		ExpectedBucketTimezone: "UTC", MaximumSchemaVersion: 1,
	}); err != nil {
		t.Fatalf("backup.Validate() error = %v", err)
	}
	if err := runner.RunScheduled(ctx, now.Add(time.Minute)); err != nil {
		t.Fatalf("second RunScheduled() error = %v", err)
	}
	if second := singleManifest(t, backupDirectory); second != manifestPath {
		t.Fatalf("second manifest = %q, want %q", second, manifestPath)
	}
}

func seedStage2Buckets(t *testing.T, store *storage.Store, bucketStart int64) {
	t.Helper()
	ctx := context.Background()
	sessionID := "stage2-integration-session"
	previous := storage.ByteTotals{Upload: 1000, Download: 4000}
	for index := int64(0); index < 7; index++ {
		start := bucketStart + index*10
		current := storage.ByteTotals{Upload: previous.Upload + 10, Download: previous.Download + 20}
		batch := storage.Batch{
			BatchID: fmt.Sprintf("stage2-integration-batch-%d", index),
			NewState: storage.CollectorCursor{
				RuntimeSessionID: sessionID, LastTotals: current,
				LastSampleAt: start + 9, BucketTimezone: "UTC",
			},
			Global: storage.TrafficRollup{
				ResolutionSec: 10, BucketStart: start, BucketEnd: start + 10,
				UploadBytes: 10, DownloadBytes: 20,
				CounterObservedSeconds:  10,
				UnattributedUploadBytes: 10, UnattributedDownloadBytes: 20,
			},
			Flows: []storage.FlowRollup{{
				Dimension: storage.FlowDimension{
					SourceNetwork: []byte{}, DestinationIP: []byte{}, DestinationPort: -1, ClassificationCode: 3,
				},
				UploadBytes: 10, DownloadBytes: 20, FlowObservationCount: 1,
			}},
		}
		if index == 0 {
			batch.NewRuntimeSession = &storage.RuntimeSessionStart{
				ID: sessionID, StartedAt: start, StartReason: "startup", SingBoxVersion: "sing-box 1.12.0-fixture",
			}
		} else {
			expected := previous
			batch.ExpectedOldTotals = &expected
		}
		if _, err := store.CommitBatch(ctx, batch); err != nil {
			t.Fatalf("CommitBatch(%d) error = %v", index, err)
		}
		previous = current
	}
}

func singleManifest(t *testing.T, directory string) string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	var manifests []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".manifest.json") {
			manifests = append(manifests, filepath.Join(directory, entry.Name()))
		}
	}
	if len(manifests) != 1 {
		t.Fatalf("manifest paths = %#v", manifests)
	}
	return manifests[0]
}
