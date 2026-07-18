package query

import (
	"github.com/Willxup/flowlens/internal/storage"
)

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
