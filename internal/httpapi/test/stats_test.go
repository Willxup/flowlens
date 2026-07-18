package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/httpapi"
	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
	flowstatus "github.com/Willxup/flowlens/internal/status"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestStage2StatisticsRoutesRequireSessionAndEncodeExactBytes(t *testing.T) {
	queries := fixtureStatisticsQueries()
	handler := statsHandler(t, queries)
	assertResponse(t, handler, http.MethodGet, "/api/v1/overview?from=100&to=200", "", nil, http.StatusUnauthorized)
	cookie := loginCookie(t, handler)

	overview := request(t, handler, http.MethodGet, "/api/v1/overview?from=100&to=200", "", cookie)
	if overview.Code != http.StatusOK || overview.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("overview response = status:%d content-type:%q body:%q", overview.Code, overview.Header().Get("Content-Type"), overview.Body.String())
	}
	var overviewBody struct {
		Current             map[string]any `json:"current"`
		BoundaryApproximate bool           `json:"boundary_approximate"`
	}
	if err := json.Unmarshal(overview.Body.Bytes(), &overviewBody); err != nil {
		t.Fatalf("overview JSON error = %v", err)
	}
	if overviewBody.Current["upload_bytes"] != "18014398509481985" ||
		overviewBody.Current["download_bytes"] != "400" ||
		overviewBody.Current["total_bytes"] != "18014398509482385" || !overviewBody.BoundaryApproximate {
		t.Fatalf("overview JSON = %#v", overviewBody)
	}

	series := request(t, handler, http.MethodGet, "/api/v1/series?from=100&to=200&resolution=auto", "", cookie)
	if series.Code != http.StatusOK || !strings.Contains(series.Body.String(), `"upload_bytes":"18014398509481985"`) ||
		!strings.Contains(series.Body.String(), `"source_resolution_sec":60`) ||
		!strings.Contains(series.Body.String(), `"boundary_approximate":true`) {
		t.Fatalf("series response = status:%d body:%q", series.Code, series.Body.String())
	}

	quality := request(t, handler, http.MethodGet, "/api/v1/quality?from=100&to=200", "", cookie)
	if quality.Code != http.StatusOK || !strings.Contains(quality.Body.String(), `"code":"fixture_gap"`) ||
		strings.Contains(quality.Body.String(), "fixture detail") {
		t.Fatalf("quality response = status:%d body:%q", quality.Code, quality.Body.String())
	}

	storageResponse := request(t, handler, http.MethodGet, "/api/v1/storage", "", cookie)
	if storageResponse.Code != http.StatusOK || !strings.Contains(storageResponse.Body.String(), `"database_bytes":"4096"`) ||
		!strings.Contains(storageResponse.Body.String(), `"soft_limit_bytes":"1048576"`) ||
		strings.Contains(storageResponse.Body.String(), "/Users/") {
		t.Fatalf("storage response = status:%d body:%q", storageResponse.Code, storageResponse.Body.String())
	}
}

func TestStage2StatisticsRoutesRejectInvalidQueriesAndMethods(t *testing.T) {
	handler := statsHandler(t, fixtureStatisticsQueries())
	cookie := loginCookie(t, handler)
	tests := []struct {
		method string
		path   string
		want   int
	}{
		{method: http.MethodPost, path: "/api/v1/overview?from=100&to=200", want: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/api/v1/overview", want: http.StatusBadRequest},
		{method: http.MethodGet, path: "/api/v1/overview?from=x&to=200", want: http.StatusBadRequest},
		{method: http.MethodGet, path: "/api/v1/overview?from=200&to=100", want: http.StatusBadRequest},
		{method: http.MethodGet, path: "/api/v1/overview?from=100&from=101&to=200", want: http.StatusBadRequest},
		{method: http.MethodGet, path: "/api/v1/overview?from=100&to=200&extra=1", want: http.StatusBadRequest},
		{method: http.MethodGet, path: "/api/v1/series?from=100&to=200&resolution=minute", want: http.StatusBadRequest},
		{method: http.MethodGet, path: "/api/v1/storage?extra=1", want: http.StatusBadRequest},
	}
	for _, test := range tests {
		response := request(t, handler, test.method, test.path, "", cookie)
		if response.Code != test.want {
			t.Errorf("%s %s status = %d, want %d, body=%q", test.method, test.path, response.Code, test.want, response.Body.String())
		}
	}
}

func TestStage2StatisticsRoutesRedactInternalErrors(t *testing.T) {
	queries := fixtureStatisticsQueries()
	queries.fail = true
	handler := statsHandler(t, queries)
	cookie := loginCookie(t, handler)
	for _, path := range []string{
		"/api/v1/overview?from=100&to=200",
		"/api/v1/series?from=100&to=200",
		"/api/v1/quality?from=100&to=200",
		"/api/v1/storage",
	} {
		response := request(t, handler, http.MethodGet, path, "", cookie)
		if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "fixture internal failure") {
			t.Errorf("GET %s response = status:%d body:%q", path, response.Code, response.Body.String())
		}
	}
}

type statisticsQueries struct {
	overview query.Overview
	series   query.Series
	quality  query.Quality
	storage  query.Storage
	fail     bool
}

func (q *statisticsQueries) Overview(context.Context, rollup.Range) (query.Overview, error) {
	if q.fail {
		return query.Overview{}, errors.New("fixture internal failure")
	}
	return q.overview, nil
}

func (q *statisticsQueries) Series(context.Context, rollup.Range) (query.Series, error) {
	if q.fail {
		return query.Series{}, errors.New("fixture internal failure")
	}
	return q.series, nil
}

func (q *statisticsQueries) Quality(context.Context, rollup.Range) (query.Quality, error) {
	if q.fail {
		return query.Quality{}, errors.New("fixture internal failure")
	}
	return q.quality, nil
}

func (q *statisticsQueries) Storage(context.Context) (query.Storage, error) {
	if q.fail {
		return query.Storage{}, errors.New("fixture internal failure")
	}
	return q.storage, nil
}

func fixtureStatisticsQueries() *statisticsQueries {
	endedAt := int64(190)
	return &statisticsQueries{
		overview: query.Overview{
			Current:             query.Totals{UploadBytes: 1<<54 + 1, DownloadBytes: 400, ElapsedSeconds: 100, ObservedSeconds: 90},
			Previous:            query.Totals{UploadBytes: 100, DownloadBytes: 300, ElapsedSeconds: 100, ObservedSeconds: 100},
			BoundaryApproximate: true,
		},
		series: query.Series{
			BoundaryApproximate: true,
			Points: []storage.TrafficRollup{{
				ResolutionSec: rollup.ResolutionMinute,
				BucketStart:   100, BucketEnd: 160,
				UploadBytes: 1<<54 + 1, DownloadBytes: 400,
				PeakUploadBytesPerSecond: 10, PeakDownloadBytesPerSecond: 20,
				SpeedUploadSampleSum: 60, SpeedDownloadSampleSum: 120, SpeedSampleCount: 6,
			}},
		},
		quality: query.Quality{Events: []storage.QualityEventRecord{{
			Code: "fixture_gap", StartedAt: 110, EndedAt: &endedAt, Flags: 1,
		}}},
		storage: query.Storage{
			Capacity: storage.CapacityStatus{
				DatabaseBytes: 4096, WALBytes: 1024, SoftLimitBytes: 1 << 20, Protecting: false,
			},
			LastRollupCleanup: &storage.MaintenanceRun{
				Operation: "rollup_cleanup", StartedAt: 100, EndedAt: &endedAt, DeletedRows: 6,
			},
		},
	}
}

func statsHandler(t *testing.T, queries httpapi.StatisticsQueries) http.Handler {
	t.Helper()
	tracker := flowstatus.NewTracker()
	handler, err := httpapi.New(httpapi.Options{
		AccessKey: fixtureAccessKey, SessionTTL: time.Hour, Status: tracker, Queries: queries,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return handler
}
