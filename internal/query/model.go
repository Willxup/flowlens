package query

import (
	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/storage"
)

// BreakdownBy selects one supported dimensional projection.
type BreakdownBy string

const (
	ByTarget   BreakdownBy = "target"
	ByEndpoint BreakdownBy = "endpoint"
	ByPort     BreakdownBy = "port"
	ByNetwork  BreakdownBy = "network"
	BySource   BreakdownBy = "source"
	ByDomain   BreakdownBy = "domain"
)

// LiveSource exposes only immutable attribution state needed by queries.
type LiveSource interface {
	Snapshot() attribution.LiveSnapshot
	Capabilities() clashapi.DimensionCapabilities
}

// Breakdown is one conserving approximate historical projection.
type Breakdown struct {
	By                  BreakdownBy
	Available           bool
	BoundaryApproximate bool
	NoTraffic           bool
	ConnectionCoverage  *float64
	DimensionRetention  *float64
	Global              storage.ByteTotals
	Other               storage.ByteTotals
	Unattributed        storage.ByteTotals
	Items               []BreakdownItem
}

// BreakdownItem is one concrete returned Top K item.
type BreakdownItem struct {
	Key           string
	RawValue      string
	DisplayName   string
	NetworkCode   int64
	UploadBytes   int64
	DownloadBytes int64
}

// LiveTarget is the query-level display model for one aggregate endpoint.
type LiveTarget struct {
	RawEndpoint            string
	DisplayName            string
	NetworkCode            int64
	Host                   string
	UploadBytesPerSecond   int64
	DownloadBytesPerSecond int64
}

// LiveTargets contains the current bounded realtime target view.
type LiveTargets struct {
	ObservedAt         int64
	IntervalMillis     int64
	ActiveConnections  int64
	ConnectionCoverage *float64
	Targets            []LiveTarget
}

// RuntimeSessionRecord omits the internal session identifier and totals.
type RuntimeSessionRecord struct {
	StartedAt            int64
	EndedAt              *int64
	StartReason          string
	EndReason            *string
	LastSeenAt           int64
	SingBoxVersion       string
	DataGapBeforeSeconds int64
}

// Label is one public alias record.
type Label struct {
	ID          int64
	LabelType   string
	MatchValue  string
	DisplayName string
	CreatedAt   int64
	UpdatedAt   int64
}

// CreateLabel is the validated user-controlled create input.
type CreateLabel struct {
	LabelType   string
	MatchValue  string
	DisplayName string
}

// LabelCandidate is one observed target ranked over the latest 30 days.
type LabelCandidate struct {
	LabelType     string
	MatchValue    string
	DisplayName   string
	UploadBytes   int64
	DownloadBytes int64
}

// Totals is one exact historical summary.
type Totals struct {
	UploadBytes     int64
	DownloadBytes   int64
	ElapsedSeconds  int64
	ObservedSeconds int64
}

// Overview contains the requested and previous equal-duration summaries.
type Overview struct {
	Current             Totals
	Previous            Totals
	BoundaryApproximate bool
}

// Series contains exact storage points and boundary approximation metadata.
type Series struct {
	Points              []storage.TrafficRollup
	BoundaryApproximate bool
}

// Quality contains public-safe quality events.
type Quality struct {
	Events []storage.QualityEventRecord
}

// Storage contains non-sensitive capacity and maintenance state.
type Storage struct {
	Capacity          storage.CapacityStatus
	LastRollupCleanup *storage.MaintenanceRun
}
