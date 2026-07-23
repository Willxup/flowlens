package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/app"
	"github.com/Willxup/flowlens/internal/config"
	_ "modernc.org/sqlite"
)

func TestStage1StartupAuthenticationCollectionPersistenceAndShutdown(t *testing.T) {
	clash := newClashFixture(t)
	defer clash.Close()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	cfg := integrationConfig(clash.URL, databasePath)

	application, err := app.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("app.New() error = %v", err)
	}
	defer application.Close()

	unauthorized := perform(application.Handler(), http.MethodGet, "/api/v1/status", "", nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthorized.Code)
	}
	login := perform(application.Handler(), http.MethodPost, "/api/v1/session",
		`{"access_key":"fixture-access-key-123456"}`, nil)
	if login.Code != http.StatusNoContent || len(login.Result().Cookies()) != 1 {
		t.Fatalf("login = status:%d cookies:%#v body:%q", login.Code, login.Result().Cookies(), login.Body.String())
	}
	authorized := perform(application.Handler(), http.MethodGet, "/api/v1/status", "", login.Result().Cookies()[0])
	if authorized.Code != http.StatusOK || !strings.Contains(authorized.Body.String(), `"status":"ok"`) {
		t.Fatalf("authenticated status = %d body=%q", authorized.Code, authorized.Body.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	go func() { runResult <- application.Run(ctx) }()

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		cancel()
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer database.Close()
	deadline := time.Now().Add(12 * time.Second)
	var rollups, nonzeroRollups, sessions, states int64
	for time.Now().Before(deadline) {
		err = database.QueryRow(`
			SELECT
				(SELECT COUNT(*) FROM traffic_rollup),
				(SELECT COUNT(*) FROM traffic_rollup WHERE upload_bytes > 0 OR download_bytes > 0),
				(SELECT COUNT(*) FROM runtime_session),
				(SELECT COUNT(*) FROM collector_state)
		`).Scan(&rollups, &nonzeroRollups, &sessions, &states)
		if err == nil && rollups > 0 && nonzeroRollups > 0 && sessions == 1 && states == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil || rollups == 0 || nonzeroRollups == 0 || sessions != 1 || states != 1 {
		cancel()
		t.Fatalf("durable Stage 1 state = rollups:%d nonzero:%d sessions:%d states:%d err:%v", rollups, nonzeroRollups, sessions, states, err)
	}
	var upload, download, unattributedUpload, unattributedDownload int64
	if err := database.QueryRow(`
		SELECT upload_bytes, download_bytes, unattributed_upload_bytes, unattributed_download_bytes
		FROM traffic_rollup
		WHERE upload_bytes > 0 OR download_bytes > 0
		ORDER BY bucket_start DESC LIMIT 1
	`).Scan(&upload, &download, &unattributedUpload, &unattributedDownload); err != nil {
		cancel()
		t.Fatalf("read traffic rollup: %v", err)
	}
	if upload <= 0 || download <= 0 || upload != unattributedUpload || download != unattributedDownload {
		cancel()
		t.Errorf("global conservation = upload:%d/%d download:%d/%d", upload, unattributedUpload, download, unattributedDownload)
	}

	cancel()
	select {
	case err := <-runResult:
		if err != nil {
			t.Errorf("App.Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("App.Run() did not stop after cancellation")
	}
}

func newClashFixture(t *testing.T) *httptest.Server {
	t.Helper()
	var mutex sync.Mutex
	connectionsCall := int64(0)
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer fixture-clash-secret" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/version":
			_ = json.NewEncoder(writer).Encode(map[string]any{"version": "sing-box 1.12.0-fixture"})
		case "/connections":
			mutex.Lock()
			connectionsCall++
			call := connectionsCall
			mutex.Unlock()
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"uploadTotal": 1000 + call*10, "downloadTotal": 4000 + call*20,
				"connections": []any{},
			})
		case "/traffic":
			flusher, _ := writer.(http.Flusher)
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				_, _ = writer.Write([]byte("{\"up\":128,\"down\":512}\n"))
				if flusher != nil {
					flusher.Flush()
				}
				select {
				case <-request.Context().Done():
					return
				case <-ticker.C:
				}
			}
		case "/memory":
			writer.WriteHeader(http.StatusNotFound)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
}

func integrationConfig(clashURL, databasePath string) config.Config {
	backupDirectory := filepath.Join(filepath.Dir(databasePath), "backups")
	return config.Config{
		SchemaVersion: 1,
		Server:        config.Server{Listen: "127.0.0.1:0"},
		ClashAPI: config.ClashAPI{
			URL: clashURL, Secret: config.Secret("fixture-clash-secret"),
			RequestTimeout:      config.Duration{Duration: time.Second},
			ConnectionsInterval: config.Duration{Duration: 50 * time.Millisecond},
			MaxResponseSize:     config.ByteSize(1 << 20),
		},
		Auth: config.Auth{
			Enabled:    true,
			AccessKey:  config.Secret("fixture-access-key-123456"),
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

func perform(handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "http://example.test"+path, strings.NewReader(body))
	request.Host = "example.test"
	if method == http.MethodPost || method == http.MethodDelete {
		request.Header.Set("Origin", "http://example.test")
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	handler.ServeHTTP(recorder, request)
	return recorder
}
