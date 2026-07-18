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
				"uploadTotal": int64(1000 + connectionsCalls), "downloadTotal": int64(4000 + connectionsCalls), "connections": []any{},
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
		Auth:    config.Auth{AccessKey: config.Secret("fixture-access-key-123456"), SessionTTL: config.Duration{Duration: time.Hour}},
		Storage: config.Storage{DatabasePath: databasePath, SoftLimit: config.ByteSize(1 << 20)},
		Time:    config.Time{Timezone: "Asia/Shanghai"},
		Retention: config.Retention{
			TenSecondDays: 1, MinuteDays: 7, HalfHourDays: 365, HourDays: 1095, TopK: 20,
		},
		Backup: config.Backup{
			Directory: backupDirectory, LocalTime: config.ClockTime{Hour: 4}, DailyKeep: 3, MonthlyKeep: 3,
		},
	}
}
