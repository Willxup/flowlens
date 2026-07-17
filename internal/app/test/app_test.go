package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	}
}
