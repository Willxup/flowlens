package clashapi_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/clashapi"
)

const (
	fixtureSecret            = "fixture-clash-secret"
	fixtureResponseSentinel  = "fixture-response-body-sentinel"
	fixtureTransportSentinel = "fixture-transport-sentinel"
)

func TestClientSendsBearerAuthorizationAndParsesVersion(t *testing.T) {
	server := fixtureServer(t, "/version", "version.json")
	defer server.Close()

	client := newFixtureClient(t, server.URL+"/", server.Client(), time.Second, 1<<20)
	got, err := client.Version(context.Background())
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if got.Version != "sing-box 1.12.0-fixture" {
		t.Errorf("Version().Version = %q", got.Version)
	}
	if !got.Premium || !got.Meta {
		t.Errorf("Version() flags = premium:%t meta:%t", got.Premium, got.Meta)
	}
}

func TestClientParsesConnectionsSnapshot(t *testing.T) {
	server := fixtureServer(t, "/connections", "connections-2.json")
	defer server.Close()

	client := newFixtureClient(t, server.URL, server.Client(), time.Second, 1<<20)
	got, err := client.Connections(context.Background())
	if err != nil {
		t.Fatalf("Connections() error = %v", err)
	}
	if got.UploadTotal != 1250 || got.DownloadTotal != 4750 || got.Memory != 68157440 {
		t.Errorf("Connections() totals = upload:%d download:%d memory:%d", got.UploadTotal, got.DownloadTotal, got.Memory)
	}
	if len(got.Connections) != 2 {
		t.Fatalf("len(Connections().Connections) = %d", len(got.Connections))
	}
	first := got.Connections[0]
	if first.ID != "00000000-0000-4000-8000-000000000001" || first.Upload != 250 || first.Download != 900 {
		t.Errorf("first connection = %#v", first)
	}
	if first.Metadata.Network != "tcp" ||
		first.Metadata.SourceIP != "192.0.2.10" ||
		first.Metadata.DestinationIP != "198.51.100.20" ||
		first.Metadata.SourcePort != "53001" ||
		first.Metadata.DestinationPort != "443" ||
		first.Metadata.Host != "api.example.test" {
		t.Errorf("first metadata = %#v", first.Metadata)
	}
	second := got.Connections[1]
	if second.Metadata.Network != "udp" || second.Metadata.DestinationIP != "2001:db8:1::20" {
		t.Errorf("second metadata = %#v", second.Metadata)
	}
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	valid := clashapi.Options{
		BaseURL:         "http://127.0.0.1:9090",
		Secret:          fixtureSecret,
		RequestTimeout:  time.Second,
		MaxResponseSize: 1024,
	}
	tests := map[string]func(*clashapi.Options){
		"empty URL":      func(o *clashapi.Options) { o.BaseURL = "" },
		"HTTPS":          func(o *clashapi.Options) { o.BaseURL = "https://example.test:9090" },
		"missing port":   func(o *clashapi.Options) { o.BaseURL = "http://example.test" },
		"path":           func(o *clashapi.Options) { o.BaseURL = "http://example.test:9090/api" },
		"user info":      func(o *clashapi.Options) { o.BaseURL = "http://user@example.test:9090" },
		"query":          func(o *clashapi.Options) { o.BaseURL = "http://example.test:9090?x=1" },
		"fragment":       func(o *clashapi.Options) { o.BaseURL = "http://example.test:9090#x" },
		"empty secret":   func(o *clashapi.Options) { o.Secret = "" },
		"zero timeout":   func(o *clashapi.Options) { o.RequestTimeout = 0 },
		"negative limit": func(o *clashapi.Options) { o.MaxResponseSize = -1 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			options := valid
			mutate(&options)
			_, err := clashapi.New(options)
			if err == nil {
				t.Fatal("New() error = nil")
			}
			assertSanitized(t, err)
		})
	}
}

func TestClientRejectsNegativeGlobalTotals(t *testing.T) {
	for name, body := range map[string]string{
		"upload":   `{"uploadTotal":-1,"downloadTotal":0,"connections":[]}`,
		"download": `{"uploadTotal":0,"downloadTotal":-1,"connections":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, closeServer := clientForBody(t, http.StatusOK, body, 1<<20)
			defer closeServer()
			_, err := client.Connections(context.Background())
			if err == nil {
				t.Fatal("Connections() error = nil")
			}
			assertSanitized(t, err)
		})
	}
}

func TestClientRejectsMissingGlobalTotals(t *testing.T) {
	for name, body := range map[string]string{
		"upload":   `{"downloadTotal":0,"connections":[]}`,
		"download": `{"uploadTotal":0,"connections":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, closeServer := clientForBody(t, http.StatusOK, body, 1<<20)
			defer closeServer()
			_, err := client.Connections(context.Background())
			if err == nil {
				t.Fatal("Connections() error = nil")
			}
			assertSanitized(t, err)
		})
	}
}

func TestClientRejectsNegativeConnectionTotals(t *testing.T) {
	for name, counters := range map[string]string{
		"upload":   `"upload":-1,"download":0`,
		"download": `"upload":0,"download":-1`,
	} {
		t.Run(name, func(t *testing.T) {
			body := `{"uploadTotal":0,"downloadTotal":0,"connections":[{"id":"fixture",` + counters + `,"metadata":{}}]}`
			client, closeServer := clientForBody(t, http.StatusOK, body, 1<<20)
			defer closeServer()
			_, err := client.Connections(context.Background())
			if err == nil {
				t.Fatal("Connections() error = nil")
			}
			assertSanitized(t, err)
		})
	}
}

func TestClientRejectsOversizedFiniteResponse(t *testing.T) {
	body := `{"version":"` + fixtureResponseSentinel + strings.Repeat("x", 128) + `"}`
	client, closeServer := clientForBody(t, http.StatusOK, body, 32)
	defer closeServer()

	_, err := client.Version(context.Background())
	if err == nil {
		t.Fatal("Version() error = nil")
	}
	assertSanitized(t, err)
}

func TestClientRejectsTrailingFiniteJSON(t *testing.T) {
	client, closeServer := clientForBody(t, http.StatusOK, `{"version":"one"}{"version":"two"}`, 1024)
	defer closeServer()

	_, err := client.Version(context.Background())
	if err == nil {
		t.Fatal("Version() error = nil")
	}
	assertSanitized(t, err)
}

func TestClientRejectsNonSuccessWithoutReturningBody(t *testing.T) {
	client, closeServer := clientForBody(t, http.StatusUnauthorized, fixtureResponseSentinel, 1024)
	defer closeServer()

	_, err := client.Version(context.Background())
	if err == nil {
		t.Fatal("Version() error = nil")
	}
	assertSanitized(t, err)
}

func TestClientHonorsFiniteRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newFixtureClient(t, server.URL, server.Client(), 20*time.Millisecond, 1024)

	started := time.Now()
	_, err := client.Version(context.Background())
	if err == nil {
		t.Fatal("Version() error = nil")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Version() elapsed = %v", elapsed)
	}
	assertSanitized(t, err)
}

func TestClientRejectsRedirects(t *testing.T) {
	var redirectTargetReached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectTargetReached.Store(true)
		_, _ = io.WriteString(w, `{"version":"redirected"}`)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()
	client := newFixtureClient(t, source.URL, source.Client(), time.Second, 1024)

	_, err := client.Version(context.Background())
	if err == nil {
		t.Fatal("Version() error = nil")
	}
	if redirectTargetReached.Load() {
		t.Error("redirect target was reached")
	}
	assertSanitized(t, err)
}

func TestClientDoesNotReturnInjectedTransportErrorText(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New(fixtureTransportSentinel + " " + fixtureSecret)
	})}
	client := newFixtureClient(t, "http://127.0.0.1:9090", httpClient, time.Second, 1024)

	_, err := client.Version(context.Background())
	if err == nil {
		t.Fatal("Version() error = nil")
	}
	assertSanitized(t, err)
}

func TestClientFormattingRedactsConfiguration(t *testing.T) {
	options := clashapi.Options{
		BaseURL:         "http://192.0.2.200:9090",
		Secret:          fixtureSecret,
		RequestTimeout:  time.Second,
		MaxResponseSize: 1024,
		HTTPClient:      http.DefaultClient,
	}
	client, err := clashapi.New(options)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for name, value := range map[string]any{"options": options, "client": client} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(format, value)
			for _, forbidden := range []string{fixtureSecret, "192.0.2.200", "9090"} {
				if strings.Contains(formatted, forbidden) {
					t.Errorf("%s fmt.Sprintf(%q) contains %q: %s", name, format, forbidden, formatted)
				}
			}
		}
	}
}

func fixtureServer(t *testing.T, wantPath, fixtureName string) *httptest.Server {
	t.Helper()
	body := fixture(t, fixtureName)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("request path = %q, want %q", r.URL.Path, wantPath)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+fixtureSecret {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "test", "fixtures", "clashapi", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func clientForBody(t *testing.T, status int, body string, limit int64) (*clashapi.Client, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	client := newFixtureClient(t, server.URL, server.Client(), time.Second, limit)
	return client, server.Close
}

func newFixtureClient(t *testing.T, baseURL string, httpClient *http.Client, timeout time.Duration, limit int64) *clashapi.Client {
	t.Helper()
	client, err := clashapi.New(clashapi.Options{
		BaseURL:         baseURL,
		Secret:          fixtureSecret,
		RequestTimeout:  timeout,
		MaxResponseSize: limit,
		HTTPClient:      httpClient,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

func assertSanitized(t *testing.T, err error) {
	t.Helper()
	for _, forbidden := range []string{fixtureSecret, fixtureResponseSentinel, fixtureTransportSentinel} {
		if strings.Contains(err.Error(), forbidden) {
			t.Errorf("error contains sensitive text %q: %v", forbidden, err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
