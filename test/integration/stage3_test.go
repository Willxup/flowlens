package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/collector"
	"github.com/Willxup/flowlens/internal/config"
	"github.com/Willxup/flowlens/internal/httpapi"
	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
	flowstatus "github.com/Willxup/flowlens/internal/status"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestStage3AttributionRollupAliasAndHTTPChain(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "flowlens.db")
	store, err := storage.Open(ctx, storage.Options{DatabasePath: databasePath, SoftLimitBytes: 256 << 20})
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	retention := config.Retention{TenSecondDays: 1, MinuteDays: 7, HalfHourDays: 365, HourDays: 1095, TopK: 2}
	tracker, err := attribution.NewTracker(attribution.Options{
		TopK: retention.TopK, SourceMode: attribution.SourcePrefix, IPv4Prefix: 24, IPv6Prefix: 64,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}
	counter, _ := collector.NewCounterTracker(nil)
	bucketStart := int64(1767225600)
	bucket, err := collector.NewGlobalBucket(bucketStart, retention.TopK)
	if err != nil {
		t.Fatalf("NewGlobalBucket() error = %v", err)
	}
	snapshots := []clashapi.ConnectionsSnapshot{
		readStage3Snapshot(t, "connections-stage3-1.json"),
		readStage3Snapshot(t, "connections-stage3-2.json"),
		readStage3Snapshot(t, "connections-stage3-3.json"),
	}
	for index, snapshot := range snapshots {
		at := time.Unix(bucketStart+int64(index+1), 0)
		observation, err := counter.Preview(collector.ByteTotals{Upload: snapshot.UploadTotal, Download: snapshot.DownloadTotal}, false)
		if err != nil {
			t.Fatalf("counter Preview(%d) error = %v", index, err)
		}
		prepared, err := tracker.Prepare(at, snapshot.Connections, storage.ByteTotals{
			Upload: observation.Delta.Upload, Download: observation.Delta.Download,
		}, observation.Baseline)
		if err != nil {
			t.Fatalf("attribution Prepare(%d) error = %v", index, err)
		}
		if err := bucket.ObserveConnections(at.Unix(), observation, int64(len(snapshot.Connections)), prepared.Contribution()); err != nil {
			t.Fatalf("ObserveConnections(%d) error = %v", index, err)
		}
		counter.Commit(observation)
		tracker.Commit(prepared)
	}
	last, _ := counter.Last()
	batch := storage.Batch{
		BatchID: "stage3-integration-batch",
		NewState: storage.CollectorCursor{
			RuntimeSessionID: "stage3-integration-session", LastTotals: storage.ByteTotals{Upload: last.Upload, Download: last.Download},
			LastSampleAt: bucketStart + 3, BucketTimezone: "UTC",
		},
		Global: bucket.Rollup(), Flows: bucket.Flows(),
		NewRuntimeSession: &storage.RuntimeSessionStart{
			ID: "stage3-integration-session", StartedAt: bucketStart + 1, StartReason: "startup", SingBoxVersion: "sing-box fixture",
		},
	}
	if _, err := store.CommitBatch(ctx, batch); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	minute, err := rollup.WindowAt(time.Unix(bucketStart, 0), rollup.ResolutionMinute, time.UTC)
	if err != nil {
		t.Fatalf("WindowAt() error = %v", err)
	}
	if _, err := store.RollupTraffic(ctx, rollup.ResolutionTenSeconds, minute, retention.TopK); err != nil {
		t.Fatalf("RollupTraffic() error = %v", err)
	}
	minuteFlows, err := store.FlowRollups(ctx, rollup.ResolutionMinute, minute.BucketStart)
	if err != nil || len(minuteFlows) > retention.TopK+2 {
		t.Fatalf("minute FlowRollups() = %#v, %v", minuteFlows, err)
	}

	now := time.Unix(bucketStart+120, 0)
	service, err := query.NewService(query.Options{
		Store: store, Live: tracker, Now: func() time.Time { return now }, Retention: retention,
		Location: time.UTC, PrivacyMode: attribution.SourcePrefix,
	})
	if err != nil {
		t.Fatalf("query.NewService() error = %v", err)
	}
	rangeValue := rollup.Range{From: bucketStart, To: bucketStart + 10}
	before, err := service.Breakdown(ctx, rangeValue, query.ByEndpoint)
	if err != nil || len(before.Items) != 2 {
		t.Fatalf("initial Breakdown() = %#v, %v", before, err)
	}
	for _, by := range []query.BreakdownBy{query.BySource, query.ByDomain} {
		breakdown, err := service.Breakdown(ctx, rangeValue, by)
		if err != nil || !breakdown.Available || len(breakdown.Items) != 2 {
			t.Fatalf("Breakdown(%s) = %#v, %v", by, breakdown, err)
		}
		var represented storage.ByteTotals
		for _, item := range breakdown.Items {
			represented.Upload += item.UploadBytes
			represented.Download += item.DownloadBytes
		}
		represented.Upload += breakdown.Other.Upload + breakdown.Unattributed.Upload
		represented.Download += breakdown.Other.Download + breakdown.Unattributed.Download
		if represented != breakdown.Global {
			t.Fatalf("Breakdown(%s) represented = %#v, global = %#v", by, represented, breakdown.Global)
		}
	}
	label, err := service.CreateLabel(ctx, query.CreateLabel{
		LabelType: "endpoint", MatchValue: "198.51.100.10:443", DisplayName: "Fixture API",
	})
	if err != nil || label.ID <= 0 {
		t.Fatalf("CreateLabel() = %#v, %v", label, err)
	}
	after, err := service.Breakdown(ctx, rangeValue, query.ByEndpoint)
	if err != nil || len(after.Items) != 2 || after.Items[0].DisplayName != "Fixture API" {
		t.Fatalf("aliased Breakdown() = %#v, %v", after, err)
	}

	statusTracker := flowstatus.NewTracker()
	_ = statusTracker.Set(flowstatus.LevelOK, "ready", true)
	handler, err := httpapi.New(httpapi.Options{
		AccessKey: "fixture-access-key-123456", SessionTTL: time.Hour,
		Status: statusTracker, Queries: service, CapabilitySource: tracker, Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("httpapi.New() error = %v", err)
	}
	cookie := stage3Login(t, handler)
	path := "/api/v1/breakdown?from=" + strconv.FormatInt(rangeValue.From, 10) + "&to=" + strconv.FormatInt(rangeValue.To, 10) + "&by=endpoint"
	response := stage3Request(t, handler, http.MethodGet, path, "", cookie)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"upload_bytes":"`) ||
		!strings.Contains(response.Body.String(), "Fixture API") {
		t.Fatalf("HTTP breakdown = status:%d body:%q", response.Code, response.Body.String())
	}
	liveResponse := stage3Request(t, handler, http.MethodGet, "/api/v1/connections/live", "", cookie)
	if liveResponse.Code != http.StatusOK || strings.Contains(liveResponse.Body.String(), snapshots[0].Connections[0].ID) {
		t.Fatalf("HTTP live = status:%d body:%q", liveResponse.Code, liveResponse.Body.String())
	}

	gapSnapshot := snapshots[len(snapshots)-1]
	gapSnapshot.UploadTotal += 10
	gapSnapshot.DownloadTotal += 20
	gapSnapshot.Connections = append([]clashapi.Connection(nil), gapSnapshot.Connections...)
	for index := range gapSnapshot.Connections {
		gapSnapshot.Connections[index].Upload += 5
		gapSnapshot.Connections[index].Download += 10
	}
	gapAt := time.Unix(bucketStart+11, 0)
	gapObservation, err := counter.Preview(collector.ByteTotals{
		Upload: gapSnapshot.UploadTotal, Download: gapSnapshot.DownloadTotal,
	}, true)
	if err != nil {
		t.Fatalf("gap counter Preview() error = %v", err)
	}
	gapPrepared, err := tracker.Prepare(gapAt, gapSnapshot.Connections, storage.ByteTotals{
		Upload: gapObservation.Delta.Upload, Download: gapObservation.Delta.Download,
	}, true)
	if err != nil {
		t.Fatalf("gap attribution Prepare() error = %v", err)
	}
	gapBucket, err := collector.NewGlobalBucket(bucketStart+10, retention.TopK)
	if err != nil {
		t.Fatalf("gap NewGlobalBucket() error = %v", err)
	}
	if err := gapBucket.ObserveConnections(
		gapAt.Unix(), gapObservation, int64(len(gapSnapshot.Connections)), gapPrepared.Contribution(),
	); err != nil {
		t.Fatalf("gap ObserveConnections() error = %v", err)
	}
	counter.Commit(gapObservation)
	tracker.Commit(gapPrepared)
	previousTotals := batch.NewState.LastTotals
	gapLast, _ := counter.Last()
	gapBatch := storage.Batch{
		BatchID:           "stage3-integration-gap-batch",
		ExpectedOldTotals: &previousTotals,
		NewState: storage.CollectorCursor{
			RuntimeSessionID: "stage3-integration-session",
			LastTotals:       storage.ByteTotals{Upload: gapLast.Upload, Download: gapLast.Download},
			LastSampleAt:     gapAt.Unix(), BucketTimezone: "UTC",
		},
		Global: gapBucket.Rollup(), Flows: gapBucket.Flows(),
	}
	if _, err := store.CommitBatch(ctx, gapBatch); err != nil {
		t.Fatalf("gap CommitBatch() error = %v", err)
	}
	gapRollup, found, err := store.TrafficRollup(ctx, rollup.ResolutionTenSeconds, bucketStart+10)
	if err != nil || !found || gapRollup.UploadBytes != 10 || gapRollup.DownloadBytes != 20 ||
		gapRollup.UnattributedUploadBytes != 10 || gapRollup.UnattributedDownloadBytes != 20 ||
		gapRollup.QualityFlags&collector.QualityFlagGap == 0 {
		t.Fatalf("gap TrafficRollup() = %#v, %t, %v", gapRollup, found, err)
	}
	gapFlows, err := store.FlowRollups(ctx, rollup.ResolutionTenSeconds, bucketStart+10)
	if err != nil || len(gapFlows) != 2 || gapFlows[0].Dimension.ClassificationCode != 2 ||
		gapFlows[1].Dimension.ClassificationCode != 3 || gapFlows[1].UploadBytes != 10 || gapFlows[1].DownloadBytes != 20 {
		t.Fatalf("gap FlowRollups() = %#v, %v", gapFlows, err)
	}
	for _, path := range []string{databasePath, databasePath + "-wal"} {
		contents, err := os.ReadFile(path)
		if err != nil && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", filepath.Base(path), err)
		}
		for _, snapshot := range snapshots {
			for _, connection := range snapshot.Connections {
				if bytes.Contains(contents, []byte(connection.ID)) {
					t.Fatalf("database artifact contains connection UUID")
				}
			}
		}
	}
}

func readStage3Snapshot(t *testing.T, name string) clashapi.ConnectionsSnapshot {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "fixtures", "clashapi", name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	var snapshot clashapi.ConnectionsSnapshot
	if err := json.Unmarshal(contents, &snapshot); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", name, err)
	}
	return snapshot
}

func stage3Login(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	response := stage3Request(t, handler, http.MethodPost, "/api/v1/session", `{"access_key":"fixture-access-key-123456"}`, nil)
	if response.Code != http.StatusNoContent || len(response.Result().Cookies()) != 1 {
		t.Fatalf("login = status:%d body:%q", response.Code, response.Body.String())
	}
	return response.Result().Cookies()[0]
}

func stage3Request(t *testing.T, handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "http://example.test"+path, strings.NewReader(body))
	request.Host = "example.test"
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete {
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
