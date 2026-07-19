package attribution_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestTrackerUUIDLifecycleAndPrepareCommit(t *testing.T) {
	tracker := newTracker(t, 20)
	start := time.Date(2026, time.July, 19, 1, 0, 0, 0, time.UTC)
	first := []clashapi.Connection{trackerConnection("id-a", 10, 20, "198.51.100.1", "443", "tcp", "a.example.test")}
	prepared, err := tracker.Prepare(start, first, storage.ByteTotals{}, false)
	if err != nil {
		t.Fatalf("first Prepare() error = %v", err)
	}
	if contribution := prepared.Contribution(); contribution.Observed || len(contribution.Flows) != 0 {
		t.Fatalf("first contribution = %#v", contribution)
	}
	if snapshot := tracker.Snapshot(); snapshot.ObservedAt != 0 {
		t.Fatalf("Snapshot changed before Commit: %#v", snapshot)
	}
	tracker.Commit(prepared)

	second := []clashapi.Connection{trackerConnection("id-a", 14, 29, "198.51.100.1", "443", "tcp", "a.example.test")}
	prepared, err = tracker.Prepare(start.Add(time.Second), second, storage.ByteTotals{Upload: 4, Download: 9}, false)
	if err != nil {
		t.Fatalf("second Prepare() error = %v", err)
	}
	want := []storage.FlowRollup{{
		Dimension:   attribution.NormalizeDimension(second[0], attribution.Options{TopK: 20, SourceMode: attribution.SourcePrefix, IPv4Prefix: 24, IPv6Prefix: 64}),
		UploadBytes: 4, DownloadBytes: 9, FlowObservationCount: 1,
	}}
	if contribution := prepared.Contribution(); !contribution.Observed || !reflect.DeepEqual(contribution.Flows, want) {
		t.Fatalf("second contribution = %#v, want %#v", contribution, want)
	}
	repeated, err := tracker.Prepare(start.Add(time.Second), second, storage.ByteTotals{Upload: 4, Download: 9}, false)
	if err != nil || !reflect.DeepEqual(repeated.Contribution(), prepared.Contribution()) {
		t.Fatalf("discarded Prepare changed state: %#v, %v", repeated.Contribution(), err)
	}
	tracker.Commit(prepared)

	changed := []clashapi.Connection{
		trackerConnection("id-a", 20, 30, "198.51.100.2", "443", "tcp", "changed.example.test"),
		trackerConnection("id-new", 100, 200, "198.51.100.3", "53", "udp", "dns.example.test"),
	}
	prepared, err = tracker.Prepare(start.Add(2*time.Second), changed, storage.ByteTotals{Upload: 6, Download: 1}, false)
	if err != nil {
		t.Fatalf("dimension-change Prepare() error = %v", err)
	}
	if contribution := prepared.Contribution(); contribution.Observed || len(contribution.Flows) != 0 ||
		contribution.Unattributed != (storage.ByteTotals{Upload: 6, Download: 1}) {
		t.Fatalf("dimension-change contribution = %#v", contribution)
	}
}

func TestTrackerResetDuplicateRollbackAndDisappearanceRebaseline(t *testing.T) {
	tracker := newTracker(t, 20)
	start := time.Date(2026, time.July, 19, 2, 0, 0, 0, time.UTC)
	initial := []clashapi.Connection{trackerConnection("id-a", 10, 20, "198.51.100.1", "443", "tcp", "")}
	prepared, _ := tracker.Prepare(start, initial, storage.ByteTotals{}, false)
	tracker.Commit(prepared)

	reset := []clashapi.Connection{trackerConnection("id-a", 100, 200, "198.51.100.1", "443", "tcp", "")}
	prepared, err := tracker.Prepare(start.Add(time.Second), reset, storage.ByteTotals{Upload: 90, Download: 180}, true)
	if err != nil {
		t.Fatalf("reset Prepare() error = %v", err)
	}
	if contribution := prepared.Contribution(); contribution.Observed || len(contribution.Flows) != 0 ||
		contribution.Unattributed != (storage.ByteTotals{Upload: 90, Download: 180}) {
		t.Fatalf("reset contribution = %#v", contribution)
	}
	tracker.Commit(prepared)

	duplicate := []clashapi.Connection{
		trackerConnection("id-a", 101, 201, "198.51.100.1", "443", "tcp", ""),
		trackerConnection("id-a", 102, 202, "198.51.100.1", "443", "tcp", ""),
	}
	prepared, err = tracker.Prepare(start.Add(2*time.Second), duplicate, storage.ByteTotals{Upload: 2, Download: 2}, false)
	if err != nil {
		t.Fatalf("duplicate Prepare() error = %v", err)
	}
	if contribution := prepared.Contribution(); contribution.Observed || len(contribution.Flows) != 0 {
		t.Fatalf("duplicate contribution = %#v", contribution)
	}
	tracker.Commit(prepared)

	rollback := []clashapi.Connection{trackerConnection("id-a", 1, 1, "198.51.100.1", "443", "tcp", "")}
	prepared, err = tracker.Prepare(start.Add(3*time.Second), rollback, storage.ByteTotals{Upload: 1, Download: 1}, false)
	if err != nil {
		t.Fatalf("rollback Prepare() error = %v", err)
	}
	if contribution := prepared.Contribution(); contribution.Observed || len(contribution.Flows) != 0 {
		t.Fatalf("rollback contribution = %#v", contribution)
	}
	tracker.Commit(prepared)

	prepared, err = tracker.Prepare(start.Add(4*time.Second), nil, storage.ByteTotals{}, false)
	if err != nil {
		t.Fatalf("disappearance Prepare() error = %v", err)
	}
	tracker.Commit(prepared)
	prepared, err = tracker.Prepare(start.Add(5*time.Second), rollback, storage.ByteTotals{}, false)
	if err != nil || prepared.Contribution().Observed {
		t.Fatalf("reappearance contribution = %#v, %v", prepared.Contribution(), err)
	}
}

func newTracker(t *testing.T, topK int) *attribution.Tracker {
	t.Helper()
	tracker, err := attribution.NewTracker(attribution.Options{
		TopK: topK, SourceMode: attribution.SourcePrefix, IPv4Prefix: 24, IPv6Prefix: 64,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}
	return tracker
}

func trackerConnection(id string, upload, download int64, destination, port, network, host string) clashapi.Connection {
	return clashapi.Connection{
		ID: id, Upload: upload, Download: download,
		Metadata: clashapi.Metadata{
			SourceIP: "192.0.2.10", DestinationIP: destination, DestinationPort: port,
			Network: network, Host: host,
		},
	}
}
