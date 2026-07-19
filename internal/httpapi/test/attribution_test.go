package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestStage3AttributionRoutesAuthenticateAndEncodeExplicitDTOs(t *testing.T) {
	coverage := 0.8
	retention := 0.75
	queries := fixtureStatisticsQueries()
	queries.breakdown = query.Breakdown{
		By: query.ByEndpoint, Available: true, BoundaryApproximate: true,
		ConnectionCoverage: &coverage, DimensionRetention: &retention,
		Global:       storage.ByteTotals{Upload: 1<<54 + 1, Download: 400},
		Other:        storage.ByteTotals{Upload: 10, Download: 20},
		Unattributed: storage.ByteTotals{Upload: 30, Download: 40},
		Items:        []query.BreakdownItem{{RawValue: "198.51.100.1:443", DisplayName: "API", NetworkCode: 1, UploadBytes: 50, DownloadBytes: 60}},
	}
	queries.live = query.LiveTargets{ObservedAt: 100, IntervalMillis: 1000, ActiveConnections: 2, ConnectionCoverage: &coverage,
		Targets: []query.LiveTarget{{RawEndpoint: "198.51.100.1:443", DisplayName: "API", NetworkCode: 1, UploadBytesPerSecond: 50, DownloadBytesPerSecond: 60}}}
	queries.sessions = []query.RuntimeSessionRecord{{StartedAt: 10, LastSeenAt: 20, StartReason: "startup", SingBoxVersion: "fixture"}}
	handler := statsHandler(t, queries)
	assertResponse(t, handler, http.MethodGet, "/api/v1/breakdown?from=100&to=200&by=endpoint", "", nil, http.StatusUnauthorized)
	cookie := loginCookie(t, handler)

	breakdown := request(t, handler, http.MethodGet, "/api/v1/breakdown?from=100&to=200&by=endpoint", "", cookie)
	if breakdown.Code != http.StatusOK || !strings.Contains(breakdown.Body.String(), `"upload_bytes":"18014398509481985"`) ||
		!strings.Contains(breakdown.Body.String(), `"approximate":true`) || strings.Contains(breakdown.Body.String(), "UUID") {
		t.Fatalf("breakdown = status:%d body:%q", breakdown.Code, breakdown.Body.String())
	}
	live := request(t, handler, http.MethodGet, "/api/v1/connections/live", "", cookie)
	if live.Code != http.StatusOK || !strings.Contains(live.Body.String(), `"upload_bytes_per_second":50`) ||
		strings.Contains(live.Body.String(), `"upload_bytes":`) || strings.Contains(live.Body.String(), "192.0.2.") {
		t.Fatalf("live = status:%d body:%q", live.Code, live.Body.String())
	}
	sessions := request(t, handler, http.MethodGet, "/api/v1/runtime-sessions", "", cookie)
	if sessions.Code != http.StatusOK || strings.Contains(sessions.Body.String(), `"id"`) {
		t.Fatalf("sessions = status:%d body:%q", sessions.Code, sessions.Body.String())
	}
	var sessionPayload struct {
		Sessions []map[string]json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal(sessions.Body.Bytes(), &sessionPayload); err != nil || len(sessionPayload.Sessions) != 1 {
		t.Fatalf("sessions JSON = %#v, %v", sessionPayload, err)
	}
	wantSessionFields := map[string]bool{
		"started_at": true, "ended_at": true, "start_reason": true, "end_reason": true,
		"last_seen_at": true, "sing_box_version": true, "data_gap_before_seconds": true,
	}
	if len(sessionPayload.Sessions[0]) != len(wantSessionFields) {
		t.Fatalf("session fields = %#v", sessionPayload.Sessions[0])
	}
	for field := range sessionPayload.Sessions[0] {
		if !wantSessionFields[field] {
			t.Fatalf("unexpected session field %q in %#v", field, sessionPayload.Sessions[0])
		}
	}
	var value map[string]any
	if err := json.Unmarshal(breakdown.Body.Bytes(), &value); err != nil {
		t.Fatalf("breakdown JSON error = %v", err)
	}
}

func TestStage3ReadRoutesRejectUnknownQueriesAndMethods(t *testing.T) {
	handler := statsHandler(t, fixtureStatisticsQueries())
	cookie := loginCookie(t, handler)
	for _, test := range []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/api/v1/breakdown?from=100&to=200&by=target", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/breakdown?from=100&to=200&by=bad", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/breakdown?from=100&to=200&by=target&extra=1", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/connections/live?extra=1", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/runtime-sessions?limit=1", http.StatusBadRequest},
	} {
		assertResponse(t, handler, test.method, test.path, "", cookie, test.want)
	}
}
