package attribution

import (
	"sort"
	"time"

	"github.com/Willxup/flowlens/internal/storage"
)

// LiveTarget is one endpoint-level aggregate. It never contains a UUID or a
// source address.
type LiveTarget struct {
	RawEndpoint            string
	NetworkCode            int64
	Host                   string
	UploadBytesPerSecond   int64
	DownloadBytesPerSecond int64
}

// LiveSnapshot is the bounded immutable realtime attribution view.
type LiveSnapshot struct {
	ObservedAt                   int64
	IntervalMillis               int64
	ActiveConnections            int64
	GlobalUploadBytesPerSecond   int64
	GlobalDownloadBytesPerSecond int64
	ConnectionCoverage           *float64
	Targets                      []LiveTarget
}

type liveAggregate struct {
	rawEndpoint string
	networkCode int64
	networkSet  bool
	host        string
	hostSet     bool
	upload      int64
	download    int64
}

func (tracker *Tracker) prepareLive(
	at time.Time,
	activeConnections int64,
	contribution Contribution,
	budget storage.ByteTotals,
) (LiveSnapshot, error) {
	snapshot := LiveSnapshot{ObservedAt: at.UTC().Unix(), ActiveConnections: activeConnections}
	if tracker.lastAt.IsZero() {
		return snapshot, nil
	}
	intervalMillis := at.Sub(tracker.lastAt).Milliseconds()
	if intervalMillis <= 0 {
		return LiveSnapshot{}, ErrAttribution
	}
	snapshot.IntervalMillis = intervalMillis
	globalUploadRate, ok := mulDivNonnegative(budget.Upload, 1000, intervalMillis)
	if !ok {
		return LiveSnapshot{}, ErrAttribution
	}
	globalDownloadRate, ok := mulDivNonnegative(budget.Download, 1000, intervalMillis)
	if !ok {
		return LiveSnapshot{}, ErrAttribution
	}
	snapshot.GlobalUploadBytesPerSecond = globalUploadRate
	snapshot.GlobalDownloadBytesPerSecond = globalDownloadRate
	if !contribution.Observed {
		return snapshot, nil
	}
	globalTotal := float64(budget.Upload) + float64(budget.Download)
	if globalTotal > 0 {
		attributed := float64(budget.Upload-contribution.Unattributed.Upload) +
			float64(budget.Download-contribution.Unattributed.Download)
		coverage := attributed / globalTotal
		snapshot.ConnectionCoverage = &coverage
	}
	aggregates := make(map[string]*liveAggregate)
	for _, flow := range contribution.Flows {
		endpoint := EndpointValue(flow.Dimension)
		if endpoint == "" {
			continue
		}
		aggregate := aggregates[endpoint]
		if aggregate == nil {
			aggregate = &liveAggregate{rawEndpoint: endpoint}
			aggregates[endpoint] = aggregate
		}
		if !safeInt64Add(&aggregate.upload, flow.UploadBytes) || !safeInt64Add(&aggregate.download, flow.DownloadBytes) {
			return LiveSnapshot{}, ErrAttribution
		}
		if !aggregate.networkSet {
			aggregate.networkCode = flow.Dimension.NetworkCode
			aggregate.networkSet = true
		} else if aggregate.networkCode != flow.Dimension.NetworkCode {
			aggregate.networkCode = 0
		}
		if !aggregate.hostSet {
			aggregate.host = flow.Dimension.Host
			aggregate.hostSet = true
		} else if aggregate.host != flow.Dimension.Host {
			aggregate.host = ""
		}
	}
	targets := make([]LiveTarget, 0, len(aggregates))
	for _, aggregate := range aggregates {
		uploadRate, ok := mulDivNonnegative(aggregate.upload, 1000, intervalMillis)
		if !ok {
			return LiveSnapshot{}, ErrAttribution
		}
		downloadRate, ok := mulDivNonnegative(aggregate.download, 1000, intervalMillis)
		if !ok {
			return LiveSnapshot{}, ErrAttribution
		}
		targets = append(targets, LiveTarget{
			RawEndpoint: aggregate.rawEndpoint, NetworkCode: aggregate.networkCode, Host: aggregate.host,
			UploadBytesPerSecond: uploadRate, DownloadBytesPerSecond: downloadRate,
		})
	}
	sort.Slice(targets, func(left, right int) bool {
		leftTotal := uint64(targets[left].UploadBytesPerSecond) + uint64(targets[left].DownloadBytesPerSecond)
		rightTotal := uint64(targets[right].UploadBytesPerSecond) + uint64(targets[right].DownloadBytesPerSecond)
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		return targets[left].RawEndpoint < targets[right].RawEndpoint
	})
	if len(targets) > tracker.options.TopK {
		targets = targets[:tracker.options.TopK]
	}
	snapshot.Targets = targets
	return snapshot, nil
}

func cloneLiveSnapshot(value LiveSnapshot) LiveSnapshot {
	if value.ConnectionCoverage != nil {
		coverage := *value.ConnectionCoverage
		value.ConnectionCoverage = &coverage
	}
	value.Targets = append([]LiveTarget(nil), value.Targets...)
	return value
}
