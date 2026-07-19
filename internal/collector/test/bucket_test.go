package collector_test

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/collector"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestNewGlobalBucketRequiresPositiveAlignedStart(t *testing.T) {
	for _, start := range []int64{-10, 0, 1001} {
		if bucket, err := collector.NewGlobalBucket(start, 20); !errors.Is(err, collector.ErrInvalidBucket) || bucket != nil {
			t.Errorf("NewGlobalBucket(%d) = %#v, %v", start, bucket, err)
		}
	}
}

func TestGlobalBucketAggregatesCompleteGlobalValues(t *testing.T) {
	bucket, err := collector.NewGlobalBucket(1000, 20)
	if err != nil {
		t.Fatalf("NewGlobalBucket() error = %v", err)
	}
	observations := []struct {
		observation collector.CounterObservation
		active      int64
	}{
		{observation: collector.CounterObservation{Delta: collector.ByteTotals{Upload: 100, Download: 400}}, active: 2},
		{observation: collector.CounterObservation{Delta: collector.ByteTotals{Upload: 50, Download: 100}}, active: 4},
	}
	for index, input := range observations {
		if err := bucket.ObserveCounter(1001+int64(index), input.observation, input.active); err != nil {
			t.Fatalf("ObserveCounter() error = %v", err)
		}
	}
	for _, input := range []struct {
		at     int64
		sample clashapi.TrafficSample
	}{
		{at: 1001, sample: clashapi.TrafficSample{Up: 10, Down: 20}},
		{at: 1002, sample: clashapi.TrafficSample{Up: 30, Down: 15}},
	} {
		if err := bucket.ObserveTraffic(input.at, input.sample); err != nil {
			t.Fatalf("ObserveTraffic() error = %v", err)
		}
	}
	for _, value := range []int64{100, 200} {
		if err := bucket.ObserveMemory(clashapi.MemorySample{Inuse: value}); err != nil {
			t.Fatalf("ObserveMemory() error = %v", err)
		}
	}

	rollup := bucket.Rollup()
	if rollup.ResolutionSec != 10 || rollup.BucketStart != 1000 || rollup.BucketEnd != 1010 ||
		rollup.UploadBytes != 150 || rollup.DownloadBytes != 500 ||
		rollup.SpeedUploadSampleSum != 40 || rollup.SpeedDownloadSampleSum != 35 || rollup.SpeedSampleCount != 2 ||
		rollup.PeakUploadBytesPerSecond != 30 || rollup.PeakUploadAt == nil || *rollup.PeakUploadAt != 1002 ||
		rollup.PeakDownloadBytesPerSecond != 20 || rollup.PeakDownloadAt == nil || *rollup.PeakDownloadAt != 1001 ||
		rollup.CounterObservedSeconds != 2 || rollup.ActiveConnectionsSum != 6 ||
		rollup.ActiveConnectionsSamples != 2 || rollup.ActiveConnectionsMax != 4 ||
		rollup.MemoryBytesSum != 300 || rollup.MemorySamples != 2 || rollup.MemoryBytesMax != 200 ||
		rollup.UnattributedUploadBytes != 150 || rollup.UnattributedDownloadBytes != 500 ||
		rollup.QualityFlags != collector.QualityFlagAttributionIncomplete {
		t.Errorf("Rollup() = %#v", rollup)
	}

	flows := bucket.Flows()
	if len(flows) != 2 || flows[0].Dimension.ClassificationCode != 2 ||
		flows[0].UploadBytes != 0 || flows[0].DownloadBytes != 0 ||
		flows[1].UploadBytes != 150 || flows[1].DownloadBytes != 500 ||
		flows[1].Dimension.ClassificationCode != 3 || flows[1].Dimension.DestinationPort != -1 ||
		len(flows[1].Dimension.SourceNetwork) != 0 || len(flows[1].Dimension.DestinationIP) != 0 {
		t.Errorf("Flows() = %#v", flows)
	}
}

func TestGlobalBucketMarksRecoveredResetBytes(t *testing.T) {
	bucket, _ := collector.NewGlobalBucket(1000, 20)
	observation := collector.CounterObservation{
		Delta:           collector.ByteTotals{Upload: 25, Download: 50},
		NewSession:      true,
		AfterGap:        true,
		TimeApproximate: true,
	}
	if err := bucket.ObserveCounter(1001, observation, 1); err != nil {
		t.Fatalf("ObserveCounter() error = %v", err)
	}
	rollup := bucket.Rollup()
	wantFlags := int64(collector.QualityFlagGap | collector.QualityFlagCounterReset |
		collector.QualityFlagAttributionIncomplete | collector.QualityFlagRecoveredTimeApproximate)
	if rollup.RecoveredUploadBytes != 25 || rollup.RecoveredDownloadBytes != 50 ||
		rollup.ResetCount != 1 || rollup.QualityFlags != wantFlags {
		t.Errorf("Rollup() = %#v, want flags %d", rollup, wantFlags)
	}
}

func TestGlobalBucketZeroBytesHaveNoFlowRows(t *testing.T) {
	bucket, _ := collector.NewGlobalBucket(1000, 20)
	if err := bucket.ObserveCounter(1001, collector.CounterObservation{}, 0); err != nil {
		t.Fatalf("ObserveCounter() error = %v", err)
	}
	if flows := bucket.Flows(); len(flows) != 0 {
		t.Errorf("Flows() = %#v, want empty", flows)
	}
	rollup := bucket.Rollup()
	if rollup.UploadBytes != 0 || rollup.DownloadBytes != 0 || rollup.QualityFlags != 0 {
		t.Errorf("Rollup() = %#v", rollup)
	}
}

func TestGlobalBucketCountsObservedSecondsSeparatelyFromSubsecondSamples(t *testing.T) {
	bucket, _ := collector.NewGlobalBucket(1000, 20)
	for index := range 20 {
		at := int64(1001 + index/10)
		if err := bucket.ObserveCounter(at, collector.CounterObservation{}, 1); err != nil {
			t.Fatalf("ObserveCounter() error = %v", err)
		}
	}
	rollup := bucket.Rollup()
	if rollup.CounterObservedSeconds != 2 || rollup.ActiveConnectionsSamples != 20 {
		t.Errorf("Rollup() seconds/samples = %d/%d, want 2/20", rollup.CounterObservedSeconds, rollup.ActiveConnectionsSamples)
	}
}

func TestGlobalBucketRejectsInvalidAndOverflowingSamplesWithoutMutation(t *testing.T) {
	tests := []struct {
		name  string
		first func(*collector.GlobalBucket) error
		fail  func(*collector.GlobalBucket) error
	}{
		{name: "counter overflow", first: func(bucket *collector.GlobalBucket) error {
			return bucket.ObserveCounter(1001, collector.CounterObservation{Delta: collector.ByteTotals{Upload: math.MaxInt64}}, 0)
		}, fail: func(bucket *collector.GlobalBucket) error {
			return bucket.ObserveCounter(1002, collector.CounterObservation{Delta: collector.ByteTotals{Upload: 1}}, 0)
		}},
		{name: "traffic overflow", first: func(bucket *collector.GlobalBucket) error {
			return bucket.ObserveTraffic(1001, clashapi.TrafficSample{Up: math.MaxInt64})
		}, fail: func(bucket *collector.GlobalBucket) error {
			return bucket.ObserveTraffic(1002, clashapi.TrafficSample{Up: 1})
		}},
		{name: "memory overflow", first: func(bucket *collector.GlobalBucket) error {
			return bucket.ObserveMemory(clashapi.MemorySample{Inuse: math.MaxInt64})
		}, fail: func(bucket *collector.GlobalBucket) error {
			return bucket.ObserveMemory(clashapi.MemorySample{Inuse: 1})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bucket, _ := collector.NewGlobalBucket(1000, 20)
			if err := test.first(bucket); err != nil {
				t.Fatalf("first sample error = %v", err)
			}
			before := bucket.Rollup()
			if err := test.fail(bucket); !errors.Is(err, collector.ErrInvalidBucket) {
				t.Fatalf("failing sample error = %v, want ErrInvalidBucket", err)
			}
			if after := bucket.Rollup(); !reflect.DeepEqual(after, before) {
				t.Errorf("Rollup() changed after rejection: before=%#v after=%#v", before, after)
			}
		})
	}

	bucket, _ := collector.NewGlobalBucket(1000, 20)
	for name, fail := range map[string]func() error{
		"negative active":        func() error { return bucket.ObserveCounter(1001, collector.CounterObservation{}, -1) },
		"traffic outside bucket": func() error { return bucket.ObserveTraffic(1010, clashapi.TrafficSample{}) },
		"negative traffic":       func() error { return bucket.ObserveTraffic(1001, clashapi.TrafficSample{Up: -1}) },
		"negative memory":        func() error { return bucket.ObserveMemory(clashapi.MemorySample{Inuse: -1}) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := fail(); !errors.Is(err, collector.ErrInvalidBucket) {
				t.Errorf("error = %v, want ErrInvalidBucket", err)
			}
		})
	}
}

func TestGlobalBucketConservesAttributionAndSelectsStableTopK(t *testing.T) {
	bucket, err := collector.NewGlobalBucket(1000, 1)
	if err != nil {
		t.Fatalf("NewGlobalBucket() error = %v", err)
	}
	dimensionA := storage.FlowDimension{DestinationFamily: 4, DestinationIP: []byte{198, 51, 100, 1}, DestinationPort: 443, NetworkCode: 1, ClassificationCode: 1}
	dimensionB := storage.FlowDimension{DestinationFamily: 4, DestinationIP: []byte{198, 51, 100, 2}, DestinationPort: 80, NetworkCode: 1, ClassificationCode: 1}
	if err := bucket.ObserveConnections(1001,
		collector.CounterObservation{Delta: collector.ByteTotals{Upload: 10, Download: 10}}, 2,
		attribution.Contribution{
			Flows: []storage.FlowRollup{
				{Dimension: dimensionA, UploadBytes: 6, DownloadBytes: 3, FlowObservationCount: 1},
				{Dimension: dimensionB, UploadBytes: 2, DownloadBytes: 5, FlowObservationCount: 1},
			},
			Unattributed: storage.ByteTotals{Upload: 2, Download: 2}, Observed: true,
		},
	); err != nil {
		t.Fatalf("ObserveConnections() error = %v", err)
	}
	flows := bucket.Flows()
	if len(flows) != 3 {
		t.Fatalf("Flows() = %#v", flows)
	}
	if !reflect.DeepEqual(flows[0].Dimension, dimensionA) || flows[0].UploadBytes != 6 || flows[0].DownloadBytes != 3 ||
		flows[1].Dimension.ClassificationCode != 2 || flows[1].UploadBytes != 2 || flows[1].DownloadBytes != 5 ||
		flows[2].Dimension.ClassificationCode != 3 || flows[2].UploadBytes != 2 || flows[2].DownloadBytes != 2 {
		t.Errorf("Flows() = %#v", flows)
	}
	rollup := bucket.Rollup()
	if rollup.UploadBytes != 10 || rollup.DownloadBytes != 10 ||
		rollup.UnattributedUploadBytes != 2 || rollup.UnattributedDownloadBytes != 2 ||
		rollup.AttributionObservedSeconds != 1 || rollup.QualityFlags != collector.QualityFlagAttributionIncomplete {
		t.Errorf("Rollup() = %#v", rollup)
	}
	flows[0].Dimension.DestinationIP[0] = 1
	if reflect.DeepEqual(flows, bucket.Flows()) {
		t.Fatal("Flows() exposed mutable bucket state")
	}
}

func TestGlobalBucketRejectsInvalidContributionAtomically(t *testing.T) {
	bucket, _ := collector.NewGlobalBucket(1000, 20)
	beforeRollup := bucket.Rollup()
	beforeFlows := bucket.Flows()
	err := bucket.ObserveConnections(1001,
		collector.CounterObservation{Delta: collector.ByteTotals{Upload: 5, Download: 5}}, 1,
		attribution.Contribution{Flows: []storage.FlowRollup{{
			Dimension:   storage.FlowDimension{DestinationPort: 443, ClassificationCode: 1},
			UploadBytes: 6, DownloadBytes: 5,
		}}},
	)
	if !errors.Is(err, collector.ErrInvalidBucket) {
		t.Fatalf("ObserveConnections() error = %v", err)
	}
	if !reflect.DeepEqual(bucket.Rollup(), beforeRollup) || !reflect.DeepEqual(bucket.Flows(), beforeFlows) {
		t.Fatal("bucket mutated after invalid contribution")
	}
}
