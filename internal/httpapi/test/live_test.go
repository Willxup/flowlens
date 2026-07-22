package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Willxup/flowlens/internal/collector"
	"github.com/Willxup/flowlens/internal/httpapi"
	flowstatus "github.com/Willxup/flowlens/internal/status"
)

func TestLiveSSERequiresSessionAndStrictRequest(t *testing.T) {
	ring, _ := collector.NewRing(4)
	handler := liveHandler(t, ring)
	assertResponse(t, handler, http.MethodGet, "/api/v1/live", "", nil, http.StatusUnauthorized)
	cookie := loginCookie(t, handler)
	assertResponse(t, handler, http.MethodPost, "/api/v1/live", "", cookie, http.StatusMethodNotAllowed)
	assertResponse(t, handler, http.MethodGet, "/api/v1/live?extra=1", "", cookie, http.StatusBadRequest)
}

func TestLiveSSEStreamsSnapshotStatusAndNewSamples(t *testing.T) {
	ring, _ := collector.NewRing(4)
	firstAt := time.Unix(1_700_000_000, 0).UTC()
	if err := ring.Add(collector.SpeedSample{
		Timestamp: firstAt, UploadBytesPerSecond: 12, DownloadBytesPerSecond: 34,
		ActiveConnections: 5, Status: collector.SampleStatusOK,
	}); err != nil {
		t.Fatalf("Ring.Add() error = %v", err)
	}
	handler := liveHandler(t, ring)
	cookie := loginCookie(t, handler)
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/live", nil)
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("GET live: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" ||
		response.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("live response = %d headers=%v", response.StatusCode, response.Header)
	}
	reader := bufio.NewReader(response.Body)
	snapshot := readSSEEvent(t, reader)
	status := readSSEEvent(t, reader)
	if snapshot.name != "snapshot" || snapshot.id != "1" || status.name != "status" || status.id != "2" {
		t.Fatalf("initial events = %#v %#v", snapshot, status)
	}
	var snapshotBody struct {
		Sequence int64 `json:"sequence"`
		Samples  []struct {
			Timestamp              int64  `json:"timestamp"`
			UploadBytesPerSecond   int64  `json:"upload_bytes_per_second"`
			DownloadBytesPerSecond int64  `json:"download_bytes_per_second"`
			ActiveConnections      int64  `json:"active_connections"`
			Status                 string `json:"status"`
		} `json:"samples"`
	}
	if err := json.Unmarshal([]byte(snapshot.data), &snapshotBody); err != nil {
		t.Fatalf("snapshot JSON: %v", err)
	}
	if snapshotBody.Sequence != 1 || len(snapshotBody.Samples) != 1 ||
		snapshotBody.Samples[0].Timestamp != firstAt.Unix() || snapshotBody.Samples[0].ActiveConnections != 5 {
		t.Fatalf("snapshot body = %#v", snapshotBody)
	}
	if strings.Contains(snapshot.data, "uuid") || strings.Contains(snapshot.data, "source") {
		t.Fatalf("snapshot leaked connection detail: %s", snapshot.data)
	}

	if err := ring.Add(collector.SpeedSample{
		Timestamp: firstAt.Add(time.Second), UploadBytesPerSecond: 56, DownloadBytesPerSecond: 78,
		ActiveConnections: 6, Status: collector.SampleStatusDegraded,
	}); err != nil {
		t.Fatalf("second Ring.Add() error = %v", err)
	}
	sample := readSSEEvent(t, reader)
	if sample.name != "sample" || sample.id != "3" || !strings.Contains(sample.data, `"active_connections":6`) {
		t.Fatalf("sample event = %#v", sample)
	}
	cancel()
}

type sseEvent struct {
	id   string
	name string
	data string
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) sseEvent {
	t.Helper()
	result := sseEvent{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				t.Fatalf("SSE ended before event: %#v", result)
			}
			t.Fatalf("read SSE: %v", err)
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			return result
		}
		switch {
		case strings.HasPrefix(line, "id: "):
			result.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			result.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			result.data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func liveHandler(t *testing.T, ring *collector.Ring) http.Handler {
	t.Helper()
	tracker := flowstatus.NewTracker()
	if err := tracker.Set(flowstatus.LevelOK, "ready", true); err != nil {
		t.Fatalf("status Set() error = %v", err)
	}
	handler, err := httpapi.New(httpapi.Options{
		AccessKey: fixtureAccessKey, SessionTTL: time.Hour, Status: tracker,
		Queries: fixtureStatisticsQueries(), CapabilitySource: fixtureCapabilitySource{}, Live: ring,
		Web: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("FlowLens")}}, Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("httpapi.New() error = %v", err)
	}
	return handler
}
