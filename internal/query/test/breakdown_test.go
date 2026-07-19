package query_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/query"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestBreakdownConservesAndFoldsQueryTopK(t *testing.T) {
	now := time.Date(2026, time.July, 19, 8, 0, 0, 0, time.UTC)
	store := &recordingQueryStore{
		trafficResponses: [][]storage.TrafficRollup{{{
			UploadBytes: 100, DownloadBytes: 200,
			UnattributedUploadBytes: 10, UnattributedDownloadBytes: 30,
		}}},
		flowResponses: [][]storage.FlowPoint{{
			{Dimension: queryDimension(1), UploadBytes: 60, DownloadBytes: 100},
			{Dimension: queryDimension(2), UploadBytes: 20, DownloadBytes: 50},
			{Dimension: querySpecialDimension(2), UploadBytes: 10, DownloadBytes: 20},
			{Dimension: querySpecialDimension(3), UploadBytes: 10, DownloadBytes: 30},
		}},
	}
	service := newServiceWith(t, store, fakeLiveSource{capabilities: clashapi.DimensionCapabilities{DestinationIP: true}}, now, 1, attribution.SourcePrefix)
	result, err := service.Breakdown(context.Background(), rollup.Range{From: now.Add(-time.Hour).Add(3 * time.Second).Unix(), To: now.Unix()}, query.ByTarget)
	if err != nil {
		t.Fatalf("Breakdown() error = %v", err)
	}
	if !result.Available || !result.BoundaryApproximate || result.NoTraffic || len(result.Items) != 1 ||
		result.Items[0].RawValue != "198.51.100.1" || result.Items[0].UploadBytes != 60 || result.Items[0].DownloadBytes != 100 ||
		result.Other != (storage.ByteTotals{Upload: 30, Download: 70}) ||
		result.Unattributed != (storage.ByteTotals{Upload: 10, Download: 30}) ||
		result.Global != (storage.ByteTotals{Upload: 100, Download: 200}) {
		t.Fatalf("Breakdown() = %#v", result)
	}
	if result.ConnectionCoverage == nil || math.Abs(*result.ConnectionCoverage-260.0/300.0) > 1e-12 ||
		result.DimensionRetention == nil || math.Abs(*result.DimensionRetention-160.0/260.0) > 1e-12 {
		t.Errorf("coverage = %#v retention = %#v", result.ConnectionCoverage, result.DimensionRetention)
	}
}

func TestBreakdownUsesOneConsistentStorageSnapshot(t *testing.T) {
	now := time.Date(2026, time.July, 19, 8, 0, 0, 0, time.UTC)
	store := &recordingQueryStore{
		trafficResponses: [][]storage.TrafficRollup{{{UploadBytes: 5, DownloadBytes: 7}}},
		flowResponses: [][]storage.FlowPoint{{
			{Dimension: queryDimension(1), UploadBytes: 6, DownloadBytes: 8},
			{Dimension: querySpecialDimension(2)}, {Dimension: querySpecialDimension(3)},
		}},
		atomicTraffic: []storage.TrafficRollup{{UploadBytes: 5, DownloadBytes: 7}},
		atomicFlows: []storage.FlowPoint{
			{Dimension: queryDimension(1), UploadBytes: 5, DownloadBytes: 7},
			{Dimension: querySpecialDimension(2)}, {Dimension: querySpecialDimension(3)},
		},
	}
	service := newServiceWith(t, store, fakeLiveSource{}, now, 20, attribution.SourcePrefix)
	result, err := service.Breakdown(
		context.Background(),
		rollup.Range{From: now.Add(-time.Hour).Unix(), To: now.Unix()},
		query.ByTarget,
	)
	if err != nil || store.atomicCalls != 1 || result.Global != (storage.ByteTotals{Upload: 5, Download: 7}) {
		t.Fatalf("Breakdown() = %#v, %v; atomic calls = %d", result, err, store.atomicCalls)
	}
}

func TestBreakdownUnavailableFoldsConcreteAndSourcePrivacyWins(t *testing.T) {
	now := time.Date(2026, time.July, 19, 8, 0, 0, 0, time.UTC)
	store := &recordingQueryStore{
		trafficResponses: [][]storage.TrafficRollup{{{UploadBytes: 5, DownloadBytes: 7}}},
		flowResponses: [][]storage.FlowPoint{{
			{Dimension: queryDimension(1), UploadBytes: 5, DownloadBytes: 7},
			{Dimension: querySpecialDimension(2)}, {Dimension: querySpecialDimension(3)},
		}},
	}
	service := newServiceWith(t, store, fakeLiveSource{}, now, 20, attribution.SourceDisabled)
	result, err := service.Breakdown(context.Background(), rollup.Range{From: now.Add(-time.Hour).Unix(), To: now.Unix()}, query.BySource)
	if err != nil {
		t.Fatalf("Breakdown() error = %v", err)
	}
	if result.Available || len(result.Items) != 0 || result.Other != (storage.ByteTotals{Upload: 5, Download: 7}) {
		t.Fatalf("Breakdown() = %#v", result)
	}
}

func TestRuntimeSessionsOmitsInternalIdentifiers(t *testing.T) {
	now := time.Date(2026, time.July, 19, 8, 0, 0, 0, time.UTC)
	store := &recordingQueryStore{sessions: []storage.RuntimeSession{{
		ID: "must-not-escape", StartedAt: 10, StartReason: "startup", LastSeenAt: 20,
		SingBoxVersion: "fixture", DataGapBeforeSeconds: 3,
	}}}
	service := newServiceWith(t, store, fakeLiveSource{}, now, 20, attribution.SourcePrefix)
	sessions, err := service.RuntimeSessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].StartedAt != 10 || sessions[0].SingBoxVersion != "fixture" {
		t.Fatalf("RuntimeSessions() = %#v, %v", sessions, err)
	}
}

type fakeLiveSource struct {
	snapshot     attribution.LiveSnapshot
	capabilities clashapi.DimensionCapabilities
}

func (source fakeLiveSource) Snapshot() attribution.LiveSnapshot { return source.snapshot }
func (source fakeLiveSource) Capabilities() clashapi.DimensionCapabilities {
	return source.capabilities
}

func queryDimension(last byte) storage.FlowDimension {
	return storage.FlowDimension{
		SourceFamily: 4, SourceNetwork: []byte{192, 0, 2, 0}, SourcePrefixLen: 24,
		DestinationFamily: 4, DestinationIP: []byte{198, 51, 100, last}, DestinationPort: 443,
		NetworkCode: 1, ClassificationCode: 1,
	}
}

func querySpecialDimension(classification int64) storage.FlowDimension {
	return storage.FlowDimension{SourceNetwork: []byte{}, DestinationIP: []byte{}, DestinationPort: -1, ClassificationCode: classification}
}
