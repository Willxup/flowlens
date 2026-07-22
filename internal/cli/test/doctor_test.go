package cli_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/cli"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestDoctorChecksStorageAndRequiredClashCapabilities(t *testing.T) {
	databasePath := migratedDatabase(t)
	server := doctorFixtureServer(t, false)
	defer server.Close()
	dependencies := cli.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return doctorConfig(databasePath, server.URL), nil
		},
		HTTPClient: server.Client(),
	}

	code, stdout, stderr := runWithDependencies([]string{"doctor"}, dependencies)
	const want = "config: ok\nstorage: ok\nclash_api: ok\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("doctor = (%d, %q, %q)", code, stdout, stderr)
	}
}

func TestDoctorRejectsMissingDatabaseWithoutCreatingIt(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "missing.db")
	server := doctorFixtureServer(t, false)
	defer server.Close()
	dependencies := cli.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return doctorConfig(databasePath, server.URL), nil
		},
		HTTPClient: server.Client(),
	}

	code, stdout, stderr := runWithDependencies([]string{"doctor"}, dependencies)
	if code != 1 || stdout != "config: ok\n" || stderr != "FlowLens doctor storage check failed\n" {
		t.Fatalf("doctor missing database = (%d, %q, %q)", code, stdout, stderr)
	}
	if _, err := os.Stat(databasePath); !os.IsNotExist(err) {
		t.Fatalf("missing database was created: %v", err)
	}
}

func TestDoctorRejectsRequiredClashCapabilityFailure(t *testing.T) {
	databasePath := migratedDatabase(t)
	server := doctorFixtureServer(t, true)
	defer server.Close()
	dependencies := cli.Dependencies{
		LoadConfig: func() (config.Config, error) {
			return doctorConfig(databasePath, server.URL), nil
		},
		HTTPClient: server.Client(),
	}

	code, stdout, stderr := runWithDependencies([]string{"doctor"}, dependencies)
	if code != 1 || stdout != "config: ok\nstorage: ok\n" || stderr != "FlowLens doctor Clash API check failed\n" {
		t.Fatalf("doctor Clash failure = (%d, %q, %q)", code, stdout, stderr)
	}
}

func migratedDatabase(t *testing.T) string {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	store, err := storage.Open(context.Background(), storage.Options{DatabasePath: databasePath})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Migrate(context.Background()); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return databasePath
}

func doctorConfig(databasePath, clashURL string) config.Config {
	return config.Config{
		ClashAPI: config.ClashAPI{
			URL: clashURL, Secret: config.Secret("fixture-clash-secret"),
			RequestTimeout: config.Duration{Duration: time.Second}, MaxResponseSize: config.ByteSize(1 << 20),
		},
		Storage: config.Storage{DatabasePath: databasePath, SoftLimit: config.ByteSize(1 << 28)},
		Time:    config.Time{Timezone: "UTC"},
	}
}

func doctorFixtureServer(t *testing.T, failVersion bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer fixture-clash-secret" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/version":
			if failVersion {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = writer.Write([]byte(`{"version":"sing-box 1.12.0-fixture"}`))
		case "/connections":
			_, _ = writer.Write([]byte(`{"uploadTotal":1,"downloadTotal":2,"connections":[]}`))
		case "/traffic":
			_, _ = writer.Write([]byte("{\"up\":1,\"down\":2}\n"))
		case "/memory":
			writer.WriteHeader(http.StatusNotFound)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
}
