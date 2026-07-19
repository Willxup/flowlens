package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestLabelsValidateCanonicalKeysAndResolvePrecedence(t *testing.T) {
	now := time.Date(2026, time.July, 19, 9, 0, 0, 0, time.UTC)
	store := &recordingQueryStore{}
	service := newServiceWith(t, store, fakeLiveSource{capabilities: clashapi.DimensionCapabilities{DestinationIP: true, DestinationPort: true}}, now, 20, attribution.SourcePrefix)
	host, err := service.CreateLabel(context.Background(), query.CreateLabel{LabelType: "host", MatchValue: "198.51.100.1", DisplayName: " Gateway "})
	if err != nil || host.DisplayName != "Gateway" {
		t.Fatalf("CreateLabel(host) = %#v, %v", host, err)
	}
	endpoint, err := service.CreateLabel(context.Background(), query.CreateLabel{LabelType: "endpoint", MatchValue: "198.51.100.1:443", DisplayName: "API"})
	if err != nil {
		t.Fatalf("CreateLabel(endpoint) error = %v", err)
	}
	if _, err := service.CreateLabel(context.Background(), query.CreateLabel{LabelType: "host", MatchValue: "198.051.100.1", DisplayName: "bad"}); err == nil {
		t.Fatal("noncanonical CreateLabel() error = nil")
	}
	store.trafficResponses = [][]storage.TrafficRollup{{{UploadBytes: 5, DownloadBytes: 7}}}
	store.flowResponses = [][]storage.FlowPoint{{
		{Dimension: queryDimension(1), UploadBytes: 5, DownloadBytes: 7},
		{Dimension: querySpecialDimension(2)}, {Dimension: querySpecialDimension(3)},
	}}
	result, err := service.Breakdown(context.Background(), rollup.Range{From: now.Add(-time.Hour).Unix(), To: now.Unix()}, query.ByEndpoint)
	if err != nil || len(result.Items) != 1 || result.Items[0].DisplayName != "API" {
		t.Fatalf("endpoint Breakdown() = %#v, %v", result, err)
	}
	if _, err := service.DeleteLabel(context.Background(), endpoint.ID); err != nil {
		t.Fatalf("DeleteLabel(endpoint) error = %v", err)
	}
	store.trafficResponses = [][]storage.TrafficRollup{{{UploadBytes: 5, DownloadBytes: 7}}}
	store.flowResponses = [][]storage.FlowPoint{{
		{Dimension: queryDimension(1), UploadBytes: 5, DownloadBytes: 7},
		{Dimension: querySpecialDimension(2)}, {Dimension: querySpecialDimension(3)},
	}}
	result, err = service.Breakdown(context.Background(), rollup.Range{From: now.Add(-time.Hour).Unix(), To: now.Unix()}, query.ByEndpoint)
	if err != nil || result.Items[0].DisplayName != "Gateway:443" {
		t.Fatalf("host fallback Breakdown() = %#v, %v", result, err)
	}
}

func TestLabelCandidatesUseThirtyDayFlowWindowAndBoundedSort(t *testing.T) {
	now := time.Date(2026, time.July, 19, 9, 0, 0, 0, time.UTC)
	store := &recordingQueryStore{
		labels: []storage.ServiceLabel{{ID: 1, LabelType: "endpoint", MatchValue: "198.51.100.2:443", DisplayName: "Second"}},
		flowResponses: [][]storage.FlowPoint{{
			{Dimension: queryDimension(1), UploadBytes: 10, DownloadBytes: 20},
			{Dimension: queryDimension(2), UploadBytes: 40, DownloadBytes: 50},
			{Dimension: querySpecialDimension(3), UploadBytes: 1, DownloadBytes: 1},
		}},
	}
	service := newServiceWith(t, store, fakeLiveSource{}, now, 20, attribution.SourcePrefix)
	candidates, err := service.LabelCandidates(context.Background())
	if err != nil {
		t.Fatalf("LabelCandidates() error = %v", err)
	}
	if len(candidates) != 4 || candidates[0].LabelType != "host" || candidates[0].MatchValue != "198.51.100.2" ||
		candidates[1].LabelType != "endpoint" || candidates[1].MatchValue != "198.51.100.2:443" ||
		candidates[1].DisplayName != "Second" || candidates[1].UploadBytes != 40 || candidates[1].DownloadBytes != 50 {
		t.Fatalf("LabelCandidates() = %#v", candidates)
	}
}

func TestLabelCandidatesExcludeBucketsCrossingThirtyDayBoundary(t *testing.T) {
	now := time.Date(2026, time.July, 19, 9, 17, 23, 0, time.UTC)
	store := &recordingQueryStore{}
	service := newServiceWith(t, store, fakeLiveSource{}, now, 20, attribution.SourcePrefix)
	if _, err := service.LabelCandidates(context.Background()); err != nil {
		t.Fatalf("LabelCandidates() error = %v", err)
	}
	if len(store.flowSegmentCalls) != 1 || len(store.flowSegmentCalls[0]) == 0 {
		t.Fatalf("flow segment calls = %#v", store.flowSegmentCalls)
	}
	segments := store.flowSegmentCalls[0]
	wantFrom := now.Add(-30 * 24 * time.Hour).Unix()
	wantTo := now.Unix()
	if segments[0].From != wantFrom || segments[len(segments)-1].To != wantTo {
		t.Fatalf("candidate segments = %#v, want exact range [%d,%d]", segments, wantFrom, wantTo)
	}
}
