package collector_test

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/collector"
)

func TestNewRingRejectsInvalidCapacity(t *testing.T) {
	for _, capacity := range []int{0, -1} {
		t.Run(strconv.Itoa(capacity), func(t *testing.T) {
			ring, err := collector.NewRing(capacity)
			if err == nil {
				t.Fatal("NewRing() error = nil")
			}
			if ring != nil {
				t.Errorf("NewRing() ring = %#v", ring)
			}
		})
	}
}

func TestRingRejectsInvalidSamplesWithoutAdvancing(t *testing.T) {
	valid := speedSample(1)
	tests := map[string]func(*collector.SpeedSample){
		"zero timestamp":       func(s *collector.SpeedSample) { s.Timestamp = time.Time{} },
		"negative upload":      func(s *collector.SpeedSample) { s.UploadBytesPerSecond = -1 },
		"negative download":    func(s *collector.SpeedSample) { s.DownloadBytesPerSecond = -1 },
		"negative connections": func(s *collector.SpeedSample) { s.ActiveConnections = -1 },
		"empty status":         func(s *collector.SpeedSample) { s.Status = "" },
		"unknown status":       func(s *collector.SpeedSample) { s.Status = "unknown" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			ring := newRing(t, 3)
			sample := valid
			mutate(&sample)
			if err := ring.Add(sample); err == nil {
				t.Fatal("Add() error = nil")
			}
			if ring.Len() != 0 || len(ring.Snapshot()) != 0 {
				t.Errorf("ring advanced after rejection: len=%d snapshot=%#v", ring.Len(), ring.Snapshot())
			}
		})
	}
}

func TestRingPreservesAppendOrder(t *testing.T) {
	ring := newRing(t, 3)
	want := []collector.SpeedSample{speedSample(1), speedSample(2), speedSample(3)}
	for _, sample := range want {
		if err := ring.Add(sample); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	}

	assertSamples(t, ring.Snapshot(), want)
	if ring.Len() != 3 {
		t.Errorf("Len() = %d", ring.Len())
	}
}

func TestRingOverwritesOldestInChronologicalOrder(t *testing.T) {
	ring := newRing(t, 3)
	for index := int64(1); index <= 5; index++ {
		if err := ring.Add(speedSample(index)); err != nil {
			t.Fatalf("Add(%d) error = %v", index, err)
		}
	}

	want := []collector.SpeedSample{speedSample(3), speedSample(4), speedSample(5)}
	assertSamples(t, ring.Snapshot(), want)
}

func TestDefaultRingRetainsExactly3600MostRecentSamples(t *testing.T) {
	if collector.DefaultRingCapacity != 3600 {
		t.Fatalf("DefaultRingCapacity = %d", collector.DefaultRingCapacity)
	}
	ring := newRing(t, collector.DefaultRingCapacity)
	for index := int64(0); index <= collector.DefaultRingCapacity; index++ {
		if err := ring.Add(speedSample(index)); err != nil {
			t.Fatalf("Add(%d) error = %v", index, err)
		}
	}

	snapshot := ring.Snapshot()
	if len(snapshot) != collector.DefaultRingCapacity || ring.Len() != collector.DefaultRingCapacity {
		t.Fatalf("retained = snapshot:%d len:%d", len(snapshot), ring.Len())
	}
	if snapshot[0] != speedSample(1) {
		t.Errorf("oldest = %#v", snapshot[0])
	}
	if snapshot[len(snapshot)-1] != speedSample(collector.DefaultRingCapacity) {
		t.Errorf("newest = %#v", snapshot[len(snapshot)-1])
	}
}

func TestRingSnapshotDoesNotExposeInternalStorage(t *testing.T) {
	ring := newRing(t, 2)
	original := speedSample(1)
	if err := ring.Add(original); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	first := ring.Snapshot()
	first[0] = speedSample(99)
	second := ring.Snapshot()
	if len(second) != 1 || second[0] != original {
		t.Errorf("second Snapshot() = %#v", second)
	}
}

func TestRingSupportsConcurrentReadersAndWriters(t *testing.T) {
	const (
		capacity       = 128
		writers        = 4
		readers        = 4
		samplesPerTask = 500
	)
	ring := newRing(t, capacity)
	var wait sync.WaitGroup

	for writer := range writers {
		wait.Add(1)
		go func(writer int) {
			defer wait.Done()
			for index := range samplesPerTask {
				sampleIndex := int64(writer*samplesPerTask + index + 1)
				if err := ring.Add(speedSample(sampleIndex)); err != nil {
					t.Errorf("Add() error = %v", err)
					return
				}
			}
		}(writer)
	}
	for range readers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range samplesPerTask {
				snapshot := ring.Snapshot()
				if len(snapshot) > capacity {
					t.Errorf("len(Snapshot()) = %d", len(snapshot))
					return
				}
				for _, sample := range snapshot {
					if sample.Timestamp.IsZero() {
						t.Error("Snapshot() contains zero timestamp")
						return
					}
				}
			}
		}()
	}
	wait.Wait()
	if ring.Len() != capacity {
		t.Errorf("Len() = %d", ring.Len())
	}
}

func speedSample(index int64) collector.SpeedSample {
	status := collector.SampleStatusOK
	if index%2 == 0 {
		status = collector.SampleStatusDegraded
	}
	return collector.SpeedSample{
		Timestamp:              time.Unix(index+1, 0).UTC(),
		UploadBytesPerSecond:   index * 10,
		DownloadBytesPerSecond: index * 20,
		ActiveConnections:      index % 100,
		Status:                 status,
	}
}

func newRing(t *testing.T, capacity int) *collector.Ring {
	t.Helper()
	ring, err := collector.NewRing(capacity)
	if err != nil {
		t.Fatalf("NewRing() error = %v", err)
	}
	return ring
}

func assertSamples(t *testing.T, got, want []collector.SpeedSample) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(Snapshot()) = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("Snapshot()[%d] = %#v, want %#v", index, got[index], want[index])
		}
	}
}
