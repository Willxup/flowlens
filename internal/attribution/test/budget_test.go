package attribution_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestAllocatePreservesOrdinaryBudgetsAndMergesDimensions(t *testing.T) {
	dimensionA := budgetDimension(1)
	dimensionB := budgetDimension(2)
	allocation, err := attribution.Allocate([]attribution.Candidate{
		{UUID: "a", Dimension: dimensionA, Raw: storage.ByteTotals{Upload: 2, Download: 3}},
		{UUID: "b", Dimension: dimensionA, Raw: storage.ByteTotals{Upload: 4, Download: 5}},
		{UUID: "c", Dimension: dimensionB, Raw: storage.ByteTotals{Upload: 1, Download: 0}},
	}, storage.ByteTotals{Upload: 10, Download: 10})
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if allocation.Clipped || allocation.Unattributed != (storage.ByteTotals{Upload: 3, Download: 2}) {
		t.Errorf("allocation summary = %#v", allocation)
	}
	want := []storage.FlowRollup{
		{Dimension: dimensionA, UploadBytes: 6, DownloadBytes: 8, FlowObservationCount: 2},
		{Dimension: dimensionB, UploadBytes: 1, FlowObservationCount: 1},
	}
	if !reflect.DeepEqual(allocation.Flows, want) {
		t.Errorf("flows = %#v, want %#v", allocation.Flows, want)
	}
}

func TestAllocateClipsDirectionsIndependentlyWithStableRemainder(t *testing.T) {
	candidates := []attribution.Candidate{
		{UUID: "b", Dimension: budgetDimension(2), Raw: storage.ByteTotals{Upload: 5, Download: 1}},
		{UUID: "a", Dimension: budgetDimension(1), Raw: storage.ByteTotals{Upload: 5, Download: 9}},
	}
	want := []storage.FlowRollup{
		{Dimension: budgetDimension(1), UploadBytes: 2, DownloadBytes: 9, FlowObservationCount: 1},
		{Dimension: budgetDimension(2), UploadBytes: 1, DownloadBytes: 1, FlowObservationCount: 1},
	}
	for _, input := range [][]attribution.Candidate{candidates, {candidates[1], candidates[0]}} {
		allocation, err := attribution.Allocate(input, storage.ByteTotals{Upload: 3, Download: 10})
		if err != nil {
			t.Fatalf("Allocate() error = %v", err)
		}
		if !allocation.Clipped || allocation.Unattributed != (storage.ByteTotals{}) || !reflect.DeepEqual(allocation.Flows, want) {
			t.Errorf("allocation = %#v, want flows %#v", allocation, want)
		}
	}
}

func TestAllocateHandlesInt64BoundaryWithoutMultiplicationOverflow(t *testing.T) {
	allocation, err := attribution.Allocate([]attribution.Candidate{
		{UUID: "a", Dimension: budgetDimension(1), Raw: storage.ByteTotals{Upload: math.MaxInt64 - 1}},
		{UUID: "b", Dimension: budgetDimension(2), Raw: storage.ByteTotals{Upload: 1}},
	}, storage.ByteTotals{Upload: math.MaxInt64 - 1})
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	var total int64
	for _, flow := range allocation.Flows {
		if flow.UploadBytes < 0 || total > math.MaxInt64-flow.UploadBytes {
			t.Fatalf("invalid flow = %#v", flow)
		}
		total += flow.UploadBytes
	}
	if total != math.MaxInt64-1 || !allocation.Clipped {
		t.Errorf("allocated total = %d, clipped = %t", total, allocation.Clipped)
	}
}

func TestAllocateRejectsNegativeAndAggregateOverflow(t *testing.T) {
	tests := []struct {
		name       string
		candidates []attribution.Candidate
		budget     storage.ByteTotals
	}{
		{name: "negative budget", budget: storage.ByteTotals{Upload: -1}},
		{name: "negative candidate", candidates: []attribution.Candidate{{UUID: "a", Dimension: budgetDimension(1), Raw: storage.ByteTotals{Download: -1}}}},
		{name: "aggregate overflow", candidates: []attribution.Candidate{
			{UUID: "a", Dimension: budgetDimension(1), Raw: storage.ByteTotals{Upload: math.MaxInt64}},
			{UUID: "b", Dimension: budgetDimension(2), Raw: storage.ByteTotals{Upload: 1}},
		}, budget: storage.ByteTotals{Upload: math.MaxInt64}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allocation, err := attribution.Allocate(test.candidates, test.budget)
			if err == nil || !reflect.DeepEqual(allocation, attribution.Allocation{}) {
				t.Fatalf("Allocate() = %#v, %v", allocation, err)
			}
		})
	}
}

func budgetDimension(last byte) storage.FlowDimension {
	return storage.FlowDimension{
		DestinationFamily: 4, DestinationIP: []byte{198, 51, 100, last},
		DestinationPort: 443, NetworkCode: 1, ClassificationCode: 1,
	}
}
