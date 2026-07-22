package attribution_test

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestTrackerPublishesBoundedImmutableLiveTargets(t *testing.T) {
	tracker := newTracker(t, 1)
	start := time.Date(2026, time.July, 19, 3, 0, 0, 0, time.UTC)
	first := []clashapi.Connection{
		trackerConnection("id-a", 10, 10, "198.51.100.1", "443", "tcp", "one.example.test"),
		trackerConnection("id-b", 10, 10, "198.51.100.1", "443", "udp", "two.example.test"),
		trackerConnection("id-c", 10, 10, "198.51.100.2", "80", "tcp", "three.example.test"),
	}
	prepared, _ := tracker.Prepare(start, first, storage.ByteTotals{}, false)
	tracker.Commit(prepared)
	second := []clashapi.Connection{
		trackerConnection("id-a", 12, 14, "198.51.100.1", "443", "tcp", "one.example.test"),
		trackerConnection("id-b", 14, 12, "198.51.100.1", "443", "udp", "two.example.test"),
		trackerConnection("id-c", 11, 11, "198.51.100.2", "80", "tcp", "three.example.test"),
	}
	prepared, err := tracker.Prepare(start.Add(500*time.Millisecond), second, storage.ByteTotals{Upload: 7, Download: 7}, false)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	tracker.Commit(prepared)
	snapshot := tracker.Snapshot()
	if snapshot.IntervalMillis != 500 || snapshot.ActiveConnections != 3 ||
		snapshot.GlobalUploadBytesPerSecond != 14 || snapshot.GlobalDownloadBytesPerSecond != 14 ||
		len(snapshot.Targets) != 1 {
		t.Fatalf("Snapshot() = %#v", snapshot)
	}
	target := snapshot.Targets[0]
	if target.RawEndpoint != "198.51.100.1:443" || target.UploadBytesPerSecond != 12 ||
		target.DownloadBytesPerSecond != 12 || target.NetworkCode != 0 || target.Host != "" {
		t.Errorf("target = %#v", target)
	}
	if snapshot.ConnectionCoverage == nil || *snapshot.ConnectionCoverage != 1 {
		t.Errorf("coverage = %#v", snapshot.ConnectionCoverage)
	}
	snapshot.Targets[0].RawEndpoint = "mutated"
	if tracker.Snapshot().Targets[0].RawEndpoint == "mutated" {
		t.Fatal("Snapshot returned aliased target storage")
	}
}

func TestTrackerLiveUsesActualIntervalAndRejectsUnrepresentableRate(t *testing.T) {
	tracker := newTracker(t, 20)
	start := time.Date(2026, time.July, 19, 4, 0, 0, 0, time.UTC)
	first := []clashapi.Connection{trackerConnection("id-a", 0, 0, "198.51.100.1", "443", "tcp", "one.example.test")}
	prepared, _ := tracker.Prepare(start, first, storage.ByteTotals{}, false)
	tracker.Commit(prepared)
	second := []clashapi.Connection{trackerConnection("id-a", 20, 10, "198.51.100.1", "443", "tcp", "one.example.test")}
	prepared, err := tracker.Prepare(start.Add(2*time.Second), second, storage.ByteTotals{Upload: 20, Download: 10}, false)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	tracker.Commit(prepared)
	target := tracker.Snapshot().Targets[0]
	if target.UploadBytesPerSecond != 10 || target.DownloadBytesPerSecond != 5 {
		t.Errorf("actual interval target = %#v", target)
	}
	before := tracker.Snapshot()
	overflow := []clashapi.Connection{trackerConnection("id-a", math.MaxInt64, 10, "198.51.100.1", "443", "tcp", "one.example.test")}
	prepared, err = tracker.Prepare(start.Add(2*time.Second+time.Millisecond), overflow, storage.ByteTotals{Upload: math.MaxInt64 - 20}, false)
	if err == nil || !reflect.DeepEqual(tracker.Snapshot(), before) {
		t.Fatalf("overflow Prepare() = %#v, %v; snapshot %#v", prepared, err, tracker.Snapshot())
	}
}
