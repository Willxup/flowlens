package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/app"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/migrations"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestAppNewCompletesOrderedStartupAndOwnsLock(t *testing.T) {
	server := probeServer(t)
	defer server.Close()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	cfg := appConfig(server.URL, databasePath)

	application, err := app.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("app.New() error = %v", err)
	}
	defer application.Close()
	ready := httptest.NewRecorder()
	application.Handler().ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "http://example.test/readyz", nil))
	if ready.Code != http.StatusNoContent {
		t.Errorf("readyz status = %d", ready.Code)
	}

	second, err := app.New(context.Background(), cfg)
	if !errors.Is(err, storage.ErrLocked) || second != nil {
		t.Errorf("second app.New() = %#v, %v, want ErrLocked", second, err)
	}
	if err := application.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := application.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestAppNewSupportsAnonymousDashboard(t *testing.T) {
	server := probeServer(t)
	defer server.Close()
	cfg := appConfig(server.URL, filepath.Join(t.TempDir(), "flowlens.db"))
	cfg.Auth.Enabled = false
	cfg.Auth.AccessKey = ""
	cfg.Auth.SessionTTL = config.Duration{}

	application, err := app.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("app.New() error = %v", err)
	}
	defer application.Close()

	handler := application.Handler()
	tests := []struct {
		path string
		want int
	}{
		{path: "/", want: http.StatusOK},
		{path: "/api/v1/status", want: http.StatusOK},
		{path: "/api/v1/session", want: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "http://example.test"+test.path, nil)
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("GET %s status = %d, want %d; body = %q", test.path, response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestAppNewRejectsPersistedBucketTimezoneMismatch(t *testing.T) {
	server := probeServer(t)
	defer server.Close()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	cfg := appConfig(server.URL, databasePath)

	application, err := app.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first app.New() error = %v", err)
	}
	if err := application.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO runtime_session(
			id, started_at, start_reason, last_upload_total, last_download_total,
			last_seen_at, sing_box_version, data_gap_before_seconds
		) VALUES ('timezone-session', 1, 'startup', 1, 1, 1, 'fixture', 0)
	`); err != nil {
		_ = database.Close()
		t.Fatalf("insert runtime session: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO collector_state(
			id, runtime_session_id, last_upload_total, last_download_total,
			last_sample_at, last_batch_id, bucket_timezone
		) VALUES (1, 'timezone-session', 1, 1, 1, 'timezone-batch', 'Asia/Shanghai')
	`); err != nil {
		_ = database.Close()
		t.Fatalf("insert collector state: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("database Close() error = %v", err)
	}

	cfg.Time.Timezone = "UTC"
	second, err := app.New(context.Background(), cfg)
	if err == nil || second != nil {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("mismatched app.New() = %#v, %v", second, err)
	}
}

func TestAppNewPublishesProbeDimensionCapabilities(t *testing.T) {
	server := probeServer(t)
	defer server.Close()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	application, err := app.New(context.Background(), appConfig(server.URL, databasePath))
	if err != nil {
		t.Fatalf("app.New() error = %v", err)
	}
	defer application.Close()

	handler := application.Handler()
	login := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(
		http.MethodPost,
		"http://example.test/api/v1/session",
		strings.NewReader(`{"access_key":"fixture-access-key-123456"}`),
	)
	loginRequest.Host = "example.test"
	loginRequest.Header.Set("Origin", "http://example.test")
	loginRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(login, loginRequest)
	if login.Code != http.StatusNoContent || len(login.Result().Cookies()) != 1 {
		t.Fatalf("login = status:%d body:%q", login.Code, login.Body.String())
	}

	statusResponse := httptest.NewRecorder()
	statusRequest := httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/status", nil)
	statusRequest.Host = "example.test"
	statusRequest.AddCookie(login.Result().Cookies()[0])
	handler.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("status = status:%d body:%q", statusResponse.Code, statusResponse.Body.String())
	}
	var payload struct {
		Capabilities map[string]bool `json:"capabilities"`
	}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("status JSON error = %v", err)
	}
	for _, field := range []string{"connection_id", "source", "destination", "port", "network", "domain"} {
		if !payload.Capabilities[field] {
			t.Errorf("status capability %q = false in %#v", field, payload.Capabilities)
		}
	}
}

func TestAppNewBacksUpStage1DatabaseBeforeMigrationTwo(t *testing.T) {
	t.Run("failed migration preserves pre-upgrade snapshot", func(t *testing.T) {
		server := probeServer(t)
		defer server.Close()
		databasePath := filepath.Join(t.TempDir(), "flowlens.db")
		createStage1Database(t, databasePath, true)
		cfg := appConfig(server.URL, databasePath)

		application, err := app.New(context.Background(), cfg)
		if err == nil || application != nil {
			if application != nil {
				_ = application.Close()
			}
			t.Fatalf("app.New() = %#v, %v", application, err)
		}
		paths, err := filepath.Glob(filepath.Join(cfg.Backup.Directory, "flowlens-pre-upgrade-*.db"))
		if err != nil || len(paths) != 1 {
			t.Fatalf("pre-upgrade snapshots = %#v, %v", paths, err)
		}
		backupDatabase, err := sql.Open("sqlite", paths[0])
		if err != nil {
			t.Fatalf("open pre-upgrade snapshot: %v", err)
		}
		defer backupDatabase.Close()
		var version int
		if err := backupDatabase.QueryRow(`SELECT MAX(version) FROM schema_migration`).Scan(&version); err != nil || version != 1 {
			t.Fatalf("pre-upgrade schema version = %d, %v", version, err)
		}
	})

	t.Run("successful migration removes temporary snapshot", func(t *testing.T) {
		server := probeServer(t)
		defer server.Close()
		databasePath := filepath.Join(t.TempDir(), "flowlens.db")
		createStage1Database(t, databasePath, false)
		cfg := appConfig(server.URL, databasePath)

		application, err := app.New(context.Background(), cfg)
		if err != nil {
			t.Fatalf("app.New() error = %v", err)
		}
		defer application.Close()
		database, err := sql.Open("sqlite", databasePath)
		if err != nil {
			t.Fatalf("open migrated database: %v", err)
		}
		defer database.Close()
		var version int
		if err := database.QueryRow(`SELECT MAX(version) FROM schema_migration`).Scan(&version); err != nil || version != 2 {
			t.Fatalf("migrated schema version = %d, %v", version, err)
		}
		paths, err := filepath.Glob(filepath.Join(cfg.Backup.Directory, "flowlens-pre-upgrade-*.db"))
		if err != nil || len(paths) != 0 {
			t.Fatalf("remaining pre-upgrade snapshots = %#v, %v", paths, err)
		}
	})
}

func createStage1Database(t *testing.T, databasePath string, failMigrationTwo bool) {
	t.Helper()
	manifest, err := migrations.List()
	if err != nil || len(manifest) < 2 {
		t.Fatalf("migrations.List() = %#v, %v", manifest, err)
	}
	initializer, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		t.Fatalf("initialize storage policy: %v", err)
	}
	defer initializer.Close()
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(manifest[0].SQL); err != nil {
		_ = database.Close()
		t.Fatalf("apply migration 001: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO schema_migration(version, applied_at, checksum) VALUES (1, 1, ?)`,
		manifest[0].Checksum,
	); err != nil {
		_ = database.Close()
		t.Fatalf("record migration 001: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO traffic_rollup (
			resolution_sec, bucket_start, bucket_end, upload_bytes, download_bytes,
			recovered_upload_bytes, recovered_download_bytes,
			speed_upload_sample_sum, speed_download_sample_sum, speed_sample_count,
			peak_upload_bytes_per_second, peak_download_bytes_per_second,
			peak_upload_at, peak_download_at, counter_observed_seconds,
			attribution_observed_seconds, active_connections_sum,
			active_connections_samples, active_connections_max,
			memory_bytes_sum, memory_samples, memory_bytes_max,
			unattributed_upload_bytes, unattributed_download_bytes,
			reset_count, quality_flags
		) VALUES (60, 120, 180, 30, 70, 0, 0, 0, 0, 0, 0, 0, NULL, NULL, 6, 0, 0, 0, 0, 0, 0, 0, 30, 70, 0, 8)
	`); err != nil {
		_ = database.Close()
		t.Fatalf("insert Stage 1 traffic fixture: %v", err)
	}
	if failMigrationTwo {
		if _, err := database.Exec(`
			CREATE TRIGGER fixture_abort_stage3_backfill BEFORE INSERT ON flow_rollup
			BEGIN SELECT RAISE(ABORT, 'fixture-only abort'); END
		`); err != nil {
			_ = database.Close()
			t.Fatalf("create migration abort trigger: %v", err)
		}
	}
}

func TestAppRunCreatesCurrentStartupBackupAndStops(t *testing.T) {
	server := probeServer(t)
	defer server.Close()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	cfg := appConfig(server.URL, databasePath)
	cfg.Server.Listen = "127.0.0.1:0"
	application, err := app.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("app.New() error = %v", err)
	}
	defer application.Close()
	ctx, cancel := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	go func() { runResult <- application.Run(ctx) }()

	deadline := time.Now().Add(3 * time.Second)
	foundManifest := false
	for time.Now().Before(deadline) {
		entries, readErr := os.ReadDir(cfg.Backup.Directory)
		if readErr == nil {
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".manifest.json") {
					foundManifest = true
					break
				}
			}
		}
		if foundManifest {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !foundManifest {
		cancel()
		t.Fatal("startup backup manifest was not created")
	}
	cancel()
	select {
	case err := <-runResult:
		if err != nil {
			t.Fatalf("App.Run() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("App.Run() did not stop after cancellation")
	}
	if err := application.Close(); err != nil {
		t.Fatalf("Close() immediately after App.Run() error = %v", err)
	}
}

func probeServer(t *testing.T) *httptest.Server {
	t.Helper()
	connectionsCalls := 0
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer fixture-clash-secret" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/version":
			_ = json.NewEncoder(writer).Encode(map[string]any{"version": "sing-box 1.12.0-fixture"})
		case "/connections":
			connectionsCalls++
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"uploadTotal": int64(1000 + connectionsCalls), "downloadTotal": int64(4000 + connectionsCalls),
				"connections": []any{map[string]any{
					"id": "fixture-app-capability-id", "upload": 100, "download": 200,
					"metadata": map[string]any{
						"network": "tcp", "sourceIP": "192.0.2.10", "destinationIP": "198.51.100.1",
						"destinationPort": "443", "host": "api.example.test",
					},
				}},
			})
		case "/traffic":
			_, _ = writer.Write([]byte("{\"up\":1,\"down\":2}\n"))
		case "/memory":
			writer.WriteHeader(http.StatusNotFound)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
}

func appConfig(clashURL, databasePath string) config.Config {
	backupDirectory := filepath.Join(filepath.Dir(databasePath), "backups")
	return config.Config{
		SchemaVersion: 1,
		Server:        config.Server{Listen: "127.0.0.1:18080"},
		ClashAPI: config.ClashAPI{
			URL: clashURL, Secret: config.Secret("fixture-clash-secret"),
			RequestTimeout: config.Duration{Duration: time.Second}, ConnectionsInterval: config.Duration{Duration: time.Second},
			MaxResponseSize: config.ByteSize(1 << 20),
		},
		Auth: config.Auth{
			Enabled: true, AccessKey: config.Secret("fixture-access-key-123456"),
			SessionTTL: config.Duration{Duration: time.Hour},
		},
		Storage: config.Storage{DatabasePath: databasePath, SoftLimit: config.ByteSize(1 << 20)},
		Time:    config.Time{Timezone: "Asia/Shanghai"},
		Retention: config.Retention{
			TenSecondDays: 1, MinuteDays: 7, HalfHourDays: 365, HourDays: 1095, TopK: 20,
		},
		Privacy: config.Privacy{SourceMode: "prefix", SourceIPv4Prefix: 24, SourceIPv6Prefix: 64},
		Backup: config.Backup{
			Directory: backupDirectory, LocalTime: config.ClockTime{Hour: 4}, DailyKeep: 3, MonthlyKeep: 3,
		},
	}
}
