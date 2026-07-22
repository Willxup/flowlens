package query_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
	_ "modernc.org/sqlite"
)

type benchmarkDatabase struct {
	store     *storage.Store
	directory string
	now       time.Time
}

var (
	benchmarkOnce   sync.Once
	benchmarkDB     *benchmarkDatabase
	benchmarkDBErr  error
	benchmarkResult query.Series
	benchmarkPoints []storage.TrafficRollup
)

func TestMain(main *testing.M) {
	code := main.Run()
	if benchmarkDB != nil {
		_ = benchmarkDB.store.Close()
		_ = os.RemoveAll(benchmarkDB.directory)
	}
	os.Exit(code)
}

func BenchmarkThirtyDayAutomaticResolution(benchmark *testing.B) {
	benchmark.StopTimer()
	database := stage5BenchmarkDatabase(benchmark)
	service := benchmarkService(benchmark, database)
	rangeValue := rollup.Range{From: database.now.AddDate(0, 0, -30).Unix(), To: database.now.Unix()}
	series, err := service.Series(context.Background(), rangeValue)
	if err != nil || len(series.Points) < 1400 {
		benchmark.Fatalf("30-day Series() = %d points, %v", len(series.Points), err)
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	benchmark.StartTimer()
	for index := 0; index < benchmark.N; index++ {
		benchmarkResult, err = service.Series(context.Background(), rangeValue)
		if err != nil {
			benchmark.Fatal(err)
		}
	}
	benchmark.StopTimer()
	reportBenchmarkStorage(benchmark, database.store, len(series.Points))
}

func BenchmarkThreeYearDaily(benchmark *testing.B) {
	benchmark.StopTimer()
	database := stage5BenchmarkDatabase(benchmark)
	rangeValue := rollup.Range{From: database.now.AddDate(-3, 0, 0).Unix(), To: database.now.Unix()}
	segments := []rollup.Segment{{
		ResolutionSec: rollup.ResolutionDay, From: rangeValue.From, To: rangeValue.To,
	}}
	points, err := database.store.TrafficSeries(context.Background(), segments)
	if err != nil || len(points) < 1095 {
		benchmark.Fatalf("three-year TrafficSeries() = %d points, %v", len(points), err)
	}
	for _, point := range points {
		if point.ResolutionSec != rollup.ResolutionDay {
			benchmark.Fatalf("three-year point resolution = %d", point.ResolutionSec)
		}
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	benchmark.StartTimer()
	for index := 0; index < benchmark.N; index++ {
		benchmarkPoints, err = database.store.TrafficSeries(context.Background(), segments)
		if err != nil {
			benchmark.Fatal(err)
		}
	}
	benchmark.StopTimer()
	reportBenchmarkStorage(benchmark, database.store, len(points))
}

func stage5BenchmarkDatabase(benchmark *testing.B) *benchmarkDatabase {
	benchmark.Helper()
	benchmarkOnce.Do(func() {
		benchmarkDB, benchmarkDBErr = createStage5BenchmarkDatabase()
	})
	if benchmarkDBErr != nil {
		benchmark.Fatal(benchmarkDBErr)
	}
	return benchmarkDB
}

func createStage5BenchmarkDatabase() (*benchmarkDatabase, error) {
	directory, err := os.MkdirTemp("", "flowlens-stage5-benchmark-")
	if err != nil {
		return nil, err
	}
	databasePath := filepath.Join(directory, "flowlens.db")
	store, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		_ = os.RemoveAll(directory)
		return nil, err
	}
	if _, err := store.Migrate(context.Background()); err != nil {
		_ = store.Close()
		_ = os.RemoveAll(directory)
		return nil, err
	}
	if err := store.Close(); err != nil {
		_ = os.RemoveAll(directory)
		return nil, err
	}
	now := time.Date(2026, time.July, 22, 0, 0, 0, 0, time.UTC)
	if err := seedBenchmarkRollups(databasePath, now); err != nil {
		_ = os.RemoveAll(directory)
		return nil, err
	}
	store, err = storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		_ = os.RemoveAll(directory)
		return nil, err
	}
	if err := store.Checkpoint(context.Background(), true); err != nil {
		_ = store.Close()
		_ = os.RemoveAll(directory)
		return nil, err
	}
	return &benchmarkDatabase{store: store, directory: directory, now: now}, nil
}

func seedBenchmarkRollups(databasePath string, now time.Time) error {
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return err
	}
	defer database.Close()
	transaction, err := database.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	statement, err := transaction.Prepare(`
		INSERT INTO traffic_rollup (
			resolution_sec, bucket_start, bucket_end,
			upload_bytes, download_bytes,
			recovered_upload_bytes, recovered_download_bytes,
			speed_upload_sample_sum, speed_download_sample_sum, speed_sample_count,
			peak_upload_bytes_per_second, peak_download_bytes_per_second,
			peak_upload_at, peak_download_at,
			counter_observed_seconds, attribution_observed_seconds,
			active_connections_sum, active_connections_samples, active_connections_max,
			memory_bytes_sum, memory_samples, memory_bytes_max,
			unattributed_upload_bytes, unattributed_download_bytes,
			reset_count, quality_flags
		) VALUES (?, ?, ?, 1024, 4096, 0, 0, 1024, 4096, 1, 1024, 4096,
			NULL, NULL, ?, 0, 1, 1, 1, 0, 0, 0, 1024, 4096, 0, 0)
	`)
	if err != nil {
		return err
	}
	defer statement.Close()
	insert := func(resolution int64, start time.Time) error {
		_, err := statement.Exec(resolution, start.Unix(), start.Unix()+resolution, resolution)
		return err
	}
	for at := now.AddDate(0, 0, -30); at.Before(now); at = at.Add(30 * time.Minute) {
		if err := insert(rollup.ResolutionHalfHour, at); err != nil {
			return fmt.Errorf("seed half-hour benchmark row: %w", err)
		}
	}
	for at := now.AddDate(-3, 0, 0); at.Before(now); at = at.AddDate(0, 0, 1) {
		if err := insert(rollup.ResolutionDay, at); err != nil {
			return fmt.Errorf("seed daily benchmark row: %w", err)
		}
	}
	return transaction.Commit()
}

func benchmarkService(benchmark *testing.B, database *benchmarkDatabase) *query.Service {
	benchmark.Helper()
	service, err := query.NewService(query.Options{
		Store: database.store, Live: fakeLiveSource{}, Now: func() time.Time { return database.now },
		Retention: config.Retention{
			TenSecondDays: 1, MinuteDays: 7, HalfHourDays: 365, HourDays: 1095, TopK: 20,
		},
		Location: time.UTC, PrivacyMode: attribution.SourcePrefix,
	})
	if err != nil {
		benchmark.Fatal(err)
	}
	return service
}

func reportBenchmarkStorage(benchmark *testing.B, store *storage.Store, points int) {
	benchmark.Helper()
	capacity, err := store.CapacityStatus(context.Background())
	if err != nil {
		benchmark.Fatal(err)
	}
	benchmark.ReportMetric(float64(capacity.DatabaseBytes), "database_bytes")
	benchmark.ReportMetric(float64(capacity.WALBytes), "wal_bytes")
	benchmark.ReportMetric(float64(points), "points/op")
}
