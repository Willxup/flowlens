package storage_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/Willxup/flowlens/internal/rollup"
)

func TestBreakdownSeriesReadsGlobalAndFlowsTogether(t *testing.T) {
	store, _ := migratedTestStore(t)
	seedTenSecondRollupCount(t, store, 2)
	segments := []rollup.Segment{{
		ResolutionSec: rollup.ResolutionTenSeconds, From: firstBucketAt, To: firstBucketAt + 20,
	}}
	wantTraffic, err := store.TrafficSeries(context.Background(), segments)
	if err != nil {
		t.Fatalf("TrafficSeries() error = %v", err)
	}
	wantFlows, err := store.FlowSeries(context.Background(), segments)
	if err != nil {
		t.Fatalf("FlowSeries() error = %v", err)
	}
	traffic, flows, err := store.BreakdownSeries(context.Background(), segments)
	if err != nil || !reflect.DeepEqual(traffic, wantTraffic) || !reflect.DeepEqual(flows, wantFlows) {
		t.Fatalf("BreakdownSeries() = %#v, %#v, %v", traffic, flows, err)
	}
}

func TestFlowSeriesReadsExactNonOverlappingSegments(t *testing.T) {
	store, _ := migratedTestStore(t)
	seedTenSecondRollupCount(t, store, 2)
	points, err := store.FlowSeries(context.Background(), []rollup.Segment{{
		ResolutionSec: rollup.ResolutionTenSeconds, From: firstBucketAt, To: firstBucketAt + 20,
	}})
	if err != nil {
		t.Fatalf("FlowSeries() error = %v", err)
	}
	if len(points) != 2 || points[0].BucketStart != firstBucketAt || points[1].BucketStart != firstBucketAt+10 ||
		points[0].Dimension.ClassificationCode != 3 {
		t.Fatalf("FlowSeries() = %#v", points)
	}
	points[0].Dimension.DestinationIP = append(points[0].Dimension.DestinationIP, 1)
	repeated, err := store.FlowSeries(context.Background(), []rollup.Segment{{
		ResolutionSec: rollup.ResolutionTenSeconds, From: firstBucketAt, To: firstBucketAt + 10,
	}})
	if err != nil || len(repeated) != 1 || len(repeated[0].Dimension.DestinationIP) != 0 {
		t.Fatalf("repeated FlowSeries() = %#v, %v", repeated, err)
	}
}

func TestFlowSeriesRejectsOverlappingOrInvalidSegments(t *testing.T) {
	store, _ := migratedTestStore(t)
	for _, segments := range [][]rollup.Segment{
		nil,
		{{ResolutionSec: 7, From: 10, To: 20}},
		{{ResolutionSec: 10, From: 10, To: 30}, {ResolutionSec: 60, From: 20, To: 80}},
	} {
		if _, err := store.FlowSeries(context.Background(), segments); err == nil {
			t.Fatalf("FlowSeries(%#v) error = nil", segments)
		}
	}
}
