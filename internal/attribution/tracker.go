package attribution

import (
	"strings"
	"sync"
	"time"

	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/storage"
)

type baseline struct {
	upload    int64
	download  int64
	dimension storage.FlowDimension
}

// Contribution is one prepared connection-level contribution to a global
// counter observation.
type Contribution struct {
	Flows        []storage.FlowRollup
	Unattributed storage.ByteTotals
	Observed     bool
	Clipped      bool
}

// Prepared holds a pure next tracker state. Its internals are intentionally
// inaccessible outside this package except for the contribution value.
type Prepared struct {
	contribution Contribution
	live         LiveSnapshot
	next         map[string]baseline
	capabilities clashapi.DimensionCapabilities
	at           time.Time
}

// Contribution returns a detached copy for atomic bucket application.
func (prepared Prepared) Contribution() Contribution {
	return cloneContribution(prepared.contribution)
}

// Tracker owns process-memory-only UUID baselines. One runtime goroutine
// prepares and commits state while HTTP readers access immutable snapshots.
type Tracker struct {
	options   Options
	baselines map[string]baseline
	lastAt    time.Time

	publicMu     sync.RWMutex
	live         LiveSnapshot
	capabilities clashapi.DimensionCapabilities
}

// NewTracker validates the bounded attribution policy.
func NewTracker(options Options) (*Tracker, error) {
	if options.TopK < 1 || options.TopK > 100 || options.IPv4Prefix < 0 || options.IPv4Prefix > 32 ||
		options.IPv6Prefix < 0 || options.IPv6Prefix > 128 ||
		(options.SourceMode != SourceFull && options.SourceMode != SourcePrefix && options.SourceMode != SourceDisabled) {
		return nil, ErrAttribution
	}
	return &Tracker{
		options: options, baselines: make(map[string]baseline), capabilities: options.Capabilities,
	}, nil
}

// Prepare computes connection deltas, the next baseline set, and the next
// public live snapshot without mutating tracker state.
func (tracker *Tracker) Prepare(
	at time.Time,
	connections []clashapi.Connection,
	budget storage.ByteTotals,
	reset bool,
) (Prepared, error) {
	if at.IsZero() || budget.Upload < 0 || budget.Download < 0 ||
		(!tracker.lastAt.IsZero() && !at.After(tracker.lastAt)) {
		return Prepared{}, ErrAttribution
	}
	capabilities := observeCapabilities(tracker.capabilities, connections)
	counts := make(map[string]int, len(connections))
	for _, connection := range connections {
		id := strings.TrimSpace(connection.ID)
		if id != "" {
			counts[id]++
		}
		if connection.Upload < 0 || connection.Download < 0 {
			return Prepared{}, ErrAttribution
		}
	}
	next := make(map[string]baseline, len(connections))
	candidates := make([]Candidate, 0, len(connections))
	comparable := false
	baselineOnly := reset || tracker.lastAt.IsZero() || !capabilities.ConnectionID
	for _, connection := range connections {
		id := strings.TrimSpace(connection.ID)
		if id == "" || counts[id] != 1 {
			continue
		}
		dimension := NormalizeDimension(connection, tracker.options)
		next[id] = baseline{
			upload: connection.Upload, download: connection.Download, dimension: cloneDimension(dimension),
		}
		if baselineOnly {
			continue
		}
		previous, exists := tracker.baselines[id]
		if !exists || connection.Upload < previous.upload || connection.Download < previous.download ||
			DimensionKey(previous.dimension) != DimensionKey(dimension) {
			continue
		}
		comparable = true
		upload := connection.Upload - previous.upload
		download := connection.Download - previous.download
		if upload != 0 || download != 0 {
			candidates = append(candidates, Candidate{
				UUID: id, Dimension: dimension, Raw: storage.ByteTotals{Upload: upload, Download: download},
			})
		}
	}
	allocation := Allocation{Unattributed: budget}
	if !baselineOnly {
		var err error
		allocation, err = Allocate(candidates, budget)
		if err != nil {
			return Prepared{}, err
		}
	}
	contribution := Contribution{
		Flows: allocation.Flows, Unattributed: allocation.Unattributed,
		Observed: comparable && !baselineOnly, Clipped: allocation.Clipped,
	}
	live, err := tracker.prepareLive(at, int64(len(connections)), contribution, budget)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{
		contribution: cloneContribution(contribution), live: live, next: next,
		capabilities: capabilities, at: at,
	}, nil
}

// Commit atomically publishes one already accepted prepared state.
func (tracker *Tracker) Commit(prepared Prepared) {
	tracker.baselines = cloneBaselines(prepared.next)
	tracker.lastAt = prepared.at
	tracker.publicMu.Lock()
	tracker.live = cloneLiveSnapshot(prepared.live)
	tracker.capabilities = prepared.capabilities
	tracker.publicMu.Unlock()
}

// Snapshot returns a detached immutable aggregate live snapshot.
func (tracker *Tracker) Snapshot() LiveSnapshot {
	tracker.publicMu.RLock()
	defer tracker.publicMu.RUnlock()
	return cloneLiveSnapshot(tracker.live)
}

// Capabilities returns the monotonic dimension capability matrix.
func (tracker *Tracker) Capabilities() clashapi.DimensionCapabilities {
	tracker.publicMu.RLock()
	defer tracker.publicMu.RUnlock()
	return tracker.capabilities
}

func observeCapabilities(
	current clashapi.DimensionCapabilities,
	connections []clashapi.Connection,
) clashapi.DimensionCapabilities {
	for _, connection := range connections {
		metadata := connection.Metadata
		current.ConnectionID = current.ConnectionID || strings.TrimSpace(connection.ID) != ""
		current.SourceIP = current.SourceIP || strings.TrimSpace(metadata.SourceIP) != ""
		current.DestinationIP = current.DestinationIP || strings.TrimSpace(metadata.DestinationIP) != ""
		current.DestinationPort = current.DestinationPort || strings.TrimSpace(metadata.DestinationPort) != ""
		current.Network = current.Network || strings.TrimSpace(metadata.Network) != ""
		current.Host = current.Host || strings.TrimSpace(metadata.Host) != ""
	}
	return current
}

func cloneBaselines(values map[string]baseline) map[string]baseline {
	cloned := make(map[string]baseline, len(values))
	for key, value := range values {
		value.dimension = cloneDimension(value.dimension)
		cloned[key] = value
	}
	return cloned
}

func cloneContribution(value Contribution) Contribution {
	value.Flows = cloneFlows(value.Flows)
	return value
}

func cloneFlows(values []storage.FlowRollup) []storage.FlowRollup {
	if values == nil {
		return nil
	}
	cloned := make([]storage.FlowRollup, len(values))
	for index, value := range values {
		value.Dimension = cloneDimension(value.Dimension)
		cloned[index] = value
	}
	return cloned
}
