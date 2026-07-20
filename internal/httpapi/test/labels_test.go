package httpapi_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestStage3LabelRoutesUseStrictAuthenticatedContracts(t *testing.T) {
	queries := fixtureStatisticsQueries()
	queries.labels = []query.Label{{ID: 1, LabelType: "host", MatchValue: "198.51.100.1", DisplayName: "Gateway", CreatedAt: 1, UpdatedAt: 1}}
	queries.candidates = []query.LabelCandidate{{LabelType: "endpoint", MatchValue: "198.51.100.1:443", DisplayName: "API", UploadBytes: 10, DownloadBytes: 20}}
	handler := statsHandler(t, queries)
	cookie := loginCookie(t, handler)

	labels := request(t, handler, http.MethodGet, "/api/v1/labels", "", cookie)
	if labels.Code != http.StatusOK || !strings.Contains(labels.Body.String(), `"match_value":"198.51.100.1"`) {
		t.Fatalf("labels = status:%d body:%q", labels.Code, labels.Body.String())
	}
	candidates := request(t, handler, http.MethodGet, "/api/v1/label-candidates", "", cookie)
	if candidates.Code != http.StatusOK || !strings.Contains(candidates.Body.String(), `"upload_bytes":"10"`) {
		t.Fatalf("candidates = status:%d body:%q", candidates.Code, candidates.Body.String())
	}
	created := request(t, handler, http.MethodPost, "/api/v1/labels", `{"label_type":"endpoint","match_value":"198.51.100.1:443","display_name":"API"}`, cookie)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"id":7`) {
		t.Fatalf("created = status:%d body:%q", created.Code, created.Body.String())
	}
	updated := request(t, handler, http.MethodPut, "/api/v1/labels/7", `{"display_name":"Renamed"}`, cookie)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), "Renamed") {
		t.Fatalf("updated = status:%d body:%q", updated.Code, updated.Body.String())
	}
	deleted := request(t, handler, http.MethodDelete, "/api/v1/labels/7", "", cookie)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("deleted = status:%d body:%q", deleted.Code, deleted.Body.String())
	}
}

func TestStage3LabelWritesRejectMalformedBodiesAndIDs(t *testing.T) {
	handler := statsHandler(t, fixtureStatisticsQueries())
	cookie := loginCookie(t, handler)
	for _, test := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/api/v1/labels", `{"label_type":"host","match_value":"198.51.100.1","display_name":"x","extra":true}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/labels", `{"label_type":"host","match_value":"198.51.100.1","display_name":"` + strings.Repeat("x", 2048) + `"}`, http.StatusRequestEntityTooLarge},
		{http.MethodPut, "/api/v1/labels/07", `{"display_name":"x"}`, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/labels?extra=1", "", http.StatusBadRequest},
	} {
		assertResponse(t, handler, test.method, test.path, test.body, cookie, test.want)
	}
}

func TestStage3LabelRoutesEnforceAuthenticationOriginAndJSON(t *testing.T) {
	handler := statsHandler(t, fixtureStatisticsQueries())
	assertResponse(
		t, handler, http.MethodPost, "/api/v1/labels",
		`{"label_type":"host","match_value":"198.51.100.1","display_name":"x"}`, nil,
		http.StatusUnauthorized,
	)
	cookie := loginCookie(t, handler)
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		origin      string
		contentType string
		want        int
	}{
		{
			name: "missing origin", method: http.MethodPost, path: "/api/v1/labels",
			body:        `{"label_type":"host","match_value":"198.51.100.1","display_name":"x"}`,
			contentType: "application/json", want: http.StatusForbidden,
		},
		{
			name: "mismatched origin", method: http.MethodPost, path: "/api/v1/labels",
			body:   `{"label_type":"host","match_value":"198.51.100.1","display_name":"x"}`,
			origin: "http://other.example.test", contentType: "application/json", want: http.StatusForbidden,
		},
		{
			name: "wrong content type", method: http.MethodPost, path: "/api/v1/labels",
			body:   `{"label_type":"host","match_value":"198.51.100.1","display_name":"x"}`,
			origin: "http://example.test", contentType: "text/plain", want: http.StatusUnsupportedMediaType,
		},
		{
			name: "trailing JSON", method: http.MethodPost, path: "/api/v1/labels",
			body:   `{"label_type":"host","match_value":"198.51.100.1","display_name":"x"}{}`,
			origin: "http://example.test", contentType: "application/json", want: http.StatusBadRequest,
		},
		{
			name: "delete missing origin", method: http.MethodDelete, path: "/api/v1/labels/7",
			want: http.StatusForbidden,
		},
		{
			name: "candidate rejects query", method: http.MethodGet, path: "/api/v1/label-candidates?limit=1",
			want: http.StatusBadRequest,
		},
		{
			name: "item rejects method", method: http.MethodGet, path: "/api/v1/labels/7",
			want: http.StatusMethodNotAllowed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, "http://example.test"+test.path, strings.NewReader(test.body))
			request.Host = "example.test"
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			request.AddCookie(cookie)
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d, body=%q", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestStage3LabelRoutesMapPublicErrorsWithoutDetails(t *testing.T) {
	falseValue := false
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		setup  func(*statisticsQueries)
		want   int
	}{
		{
			name: "invalid create", method: http.MethodPost, path: "/api/v1/labels",
			body:  `{"label_type":"host","match_value":"198.51.100.1","display_name":"x"}`,
			setup: func(queries *statisticsQueries) { queries.createLabelErr = storage.ErrInvalidLabel },
			want:  http.StatusBadRequest,
		},
		{
			name: "conflict create", method: http.MethodPost, path: "/api/v1/labels",
			body:  `{"label_type":"host","match_value":"198.51.100.1","display_name":"x"}`,
			setup: func(queries *statisticsQueries) { queries.createLabelErr = storage.ErrLabelConflict },
			want:  http.StatusConflict,
		},
		{
			name: "missing update", method: http.MethodPut, path: "/api/v1/labels/7",
			body:  `{"display_name":"x"}`,
			setup: func(queries *statisticsQueries) { queries.updateLabelErr = storage.ErrLabelNotFound },
			want:  http.StatusNotFound,
		},
		{
			name: "missing delete", method: http.MethodDelete, path: "/api/v1/labels/7",
			setup: func(queries *statisticsQueries) { queries.deleteLabelFound = &falseValue },
			want:  http.StatusNotFound,
		},
		{
			name: "internal failure", method: http.MethodPost, path: "/api/v1/labels",
			body:  `{"label_type":"host","match_value":"198.51.100.1","display_name":"x"}`,
			setup: func(queries *statisticsQueries) { queries.createLabelErr = errors.New("fixture internal failure") },
			want:  http.StatusServiceUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			queries := fixtureStatisticsQueries()
			test.setup(queries)
			handler := statsHandler(t, queries)
			cookie := loginCookie(t, handler)
			response := request(t, handler, test.method, test.path, test.body, cookie)
			if response.Code != test.want || strings.Contains(response.Body.String(), "fixture internal failure") {
				t.Fatalf("response = status:%d body:%q, want %d", response.Code, response.Body.String(), test.want)
			}
		})
	}
}
