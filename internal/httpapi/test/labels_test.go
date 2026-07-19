package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Willxup/flowlens/internal/query"
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
