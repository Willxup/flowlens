package integration_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/collector"
	"github.com/Willxup/flowlens/internal/httpapi"
	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
	flowstatus "github.com/Willxup/flowlens/internal/status"
)

func TestStage4AuthenticatedEmbeddedWebAPIAndSSEFlow(t *testing.T) {
	status := flowstatus.NewTracker()
	if err := status.Set(flowstatus.LevelOK, "ready", true); err != nil {
		t.Fatalf("status.Set() error = %v", err)
	}
	ring, err := collector.NewRing(4)
	if err != nil {
		t.Fatalf("collector.NewRing() error = %v", err)
	}
	if err := ring.Add(collector.SpeedSample{
		Timestamp: time.Unix(1_700_000_000, 0).UTC(), UploadBytesPerSecond: 12,
		DownloadBytesPerSecond: 34, ActiveConnections: 2, Status: collector.SampleStatusOK,
	}); err != nil {
		t.Fatalf("ring.Add() error = %v", err)
	}
	handler, err := httpapi.New(httpapi.Options{
		AccessKey: "fixture-access-key-123456", SessionTTL: time.Hour, Status: status,
		Queries: stage4Queries{}, CapabilitySource: stage4Capabilities{}, Live: ring,
		Timezone: "Asia/Shanghai", Web: fstest.MapFS{
			"index.html":           &fstest.MapFile{Data: []byte("<!doctype html><title>FlowLens</title>")},
			"theme-init.js":        &fstest.MapFile{Data: []byte("document.documentElement.dataset.theme='light'")},
			"favicon.svg":          &fstest.MapFile{Data: []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>")},
			"assets/app-a1b2c3.js": &fstest.MapFile{Data: []byte("console.log('FlowLens')")},
		},
	})
	if err != nil {
		t.Fatalf("httpapi.New() error = %v", err)
	}

	if response := stage3Request(t, handler, http.MethodGet, "/login", "", nil); response.Code != http.StatusOK {
		t.Fatalf("anonymous login page = %d", response.Code)
	}
	if response := stage3Request(t, handler, http.MethodGet, "/", "", nil); response.Code != http.StatusFound || response.Header().Get("Location") != "/login" {
		t.Fatalf("anonymous root = %d Location=%q", response.Code, response.Header().Get("Location"))
	}
	for _, path := range []string{"/api/v1/status", "/api/v1/storage", "/api/v1/live"} {
		if response := stage3Request(t, handler, http.MethodGet, path, "", nil); response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous GET %s = %d", path, response.Code)
		}
	}

	cookie := stage3Login(t, handler)
	if response := stage3Request(t, handler, http.MethodGet, "/", "", cookie); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "FlowLens") {
		t.Fatalf("authenticated root = %d body=%q", response.Code, response.Body.String())
	}
	if response := stage3Request(t, handler, http.MethodGet, "/api/v1/status", "", cookie); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"timezone":"Asia/Shanghai"`) {
		t.Fatalf("authenticated status = %d body=%q", response.Code, response.Body.String())
	}
	if response := stage3Request(t, handler, http.MethodGet, "/api/v1/storage", "", cookie); response.Code != http.StatusOK {
		t.Fatalf("authenticated storage = %d body=%q", response.Code, response.Body.String())
	}

	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/live", nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", err)
	}
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("GET live error = %v", err)
	}
	reader := bufio.NewReader(response.Body)
	if response.StatusCode != http.StatusOK || stage4ReadSSEEvent(t, reader) != "snapshot" || stage4ReadSSEEvent(t, reader) != "status" {
		t.Fatalf("authenticated SSE status = %d", response.StatusCode)
	}
	cancel()
	_ = response.Body.Close()

	if response := stage3Request(t, handler, http.MethodDelete, "/api/v1/session", "", cookie); response.Code != http.StatusNoContent {
		t.Fatalf("logout = %d body=%q", response.Code, response.Body.String())
	}
	if response := stage3Request(t, handler, http.MethodGet, "/api/v1/status", "", cookie); response.Code != http.StatusUnauthorized {
		t.Fatalf("status after logout = %d", response.Code)
	}
	if response := stage3Request(t, handler, http.MethodGet, "/", "", cookie); response.Code != http.StatusFound || response.Header().Get("Location") != "/login" {
		t.Fatalf("root after logout = %d Location=%q", response.Code, response.Header().Get("Location"))
	}
}

func stage4ReadSSEEvent(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	name := ""
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE event error = %v", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return name
		}
		if strings.HasPrefix(line, "event: ") {
			name = strings.TrimPrefix(line, "event: ")
		}
	}
}

type stage4Queries struct{}

func (stage4Queries) Overview(context.Context, rollup.Range) (query.Overview, error) {
	return query.Overview{}, nil
}
func (stage4Queries) Series(context.Context, rollup.Range) (query.Series, error) {
	return query.Series{}, nil
}
func (stage4Queries) Quality(context.Context, rollup.Range) (query.Quality, error) {
	return query.Quality{}, nil
}
func (stage4Queries) Storage(context.Context) (query.Storage, error) { return query.Storage{}, nil }
func (stage4Queries) Breakdown(context.Context, rollup.Range, query.BreakdownBy) (query.Breakdown, error) {
	return query.Breakdown{}, nil
}
func (stage4Queries) LiveTargets(context.Context) (query.LiveTargets, error) {
	return query.LiveTargets{}, nil
}
func (stage4Queries) RuntimeSessions(context.Context) ([]query.RuntimeSessionRecord, error) {
	return nil, nil
}
func (stage4Queries) Labels(context.Context) ([]query.Label, error) { return nil, nil }
func (stage4Queries) LabelCandidates(context.Context) ([]query.LabelCandidate, error) {
	return nil, nil
}
func (stage4Queries) CreateLabel(context.Context, query.CreateLabel) (query.Label, error) {
	return query.Label{}, nil
}
func (stage4Queries) UpdateLabel(context.Context, int64, string) (query.Label, error) {
	return query.Label{}, nil
}
func (stage4Queries) DeleteLabel(context.Context, int64) (bool, error) { return true, nil }

type stage4Capabilities struct{}

func (stage4Capabilities) Capabilities() clashapi.DimensionCapabilities {
	return clashapi.DimensionCapabilities{}
}
