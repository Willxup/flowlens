package storage

// ByteTotals contains exact cumulative upload and download bytes.
type ByteTotals struct {
	Upload   int64
	Download int64
}

func (ByteTotals) String() string     { return "ByteTotals{redacted}" }
func (v ByteTotals) GoString() string { return v.String() }

// CollectorCursor is the next exact cumulative cursor written by a batch.
type CollectorCursor struct {
	RuntimeSessionID string
	LastTotals       ByteTotals
	LastSampleAt     int64
	BucketTimezone   string
}

func (CollectorCursor) String() string     { return "CollectorCursor{redacted}" }
func (v CollectorCursor) GoString() string { return v.String() }

// CollectorState is the durable cursor plus its stable committed batch ID.
type CollectorState struct {
	CollectorCursor
	LastBatchID string
}

func (CollectorState) String() string     { return "CollectorState{redacted}" }
func (v CollectorState) GoString() string { return v.String() }

// TrafficRollup contains every authoritative global bucket column.
type TrafficRollup struct {
	ResolutionSec              int64
	BucketStart                int64
	BucketEnd                  int64
	UploadBytes                int64
	DownloadBytes              int64
	RecoveredUploadBytes       int64
	RecoveredDownloadBytes     int64
	SpeedUploadSampleSum       int64
	SpeedDownloadSampleSum     int64
	SpeedSampleCount           int64
	PeakUploadBytesPerSecond   int64
	PeakDownloadBytesPerSecond int64
	PeakUploadAt               *int64
	PeakDownloadAt             *int64
	CounterObservedSeconds     int64
	AttributionObservedSeconds int64
	ActiveConnectionsSum       int64
	ActiveConnectionsSamples   int64
	ActiveConnectionsMax       int64
	MemoryBytesSum             int64
	MemorySamples              int64
	MemoryBytesMax             int64
	UnattributedUploadBytes    int64
	UnattributedDownloadBytes  int64
	ResetCount                 int64
	QualityFlags               int64
}

func (TrafficRollup) String() string     { return "TrafficRollup{redacted}" }
func (v TrafficRollup) GoString() string { return v.String() }

// FlowDimension is one structured, privacy-processed dimension dictionary key.
type FlowDimension struct {
	SourceFamily       int64
	SourceNetwork      []byte
	SourcePrefixLen    int64
	DestinationFamily  int64
	DestinationIP      []byte
	DestinationPort    int64
	Host               string
	NetworkCode        int64
	ClassificationCode int64
}

func (FlowDimension) String() string     { return "FlowDimension{redacted}" }
func (v FlowDimension) GoString() string { return v.String() }

// FlowRollup is one complete multidimensional row for the batch bucket.
type FlowRollup struct {
	Dimension            FlowDimension
	UploadBytes          int64
	DownloadBytes        int64
	FlowObservationCount int64
}

func (FlowRollup) String() string     { return "FlowRollup{redacted}" }
func (v FlowRollup) GoString() string { return v.String() }

// RuntimeSessionStart requests a new runtime session in the same batch.
type RuntimeSessionStart struct {
	ID                   string
	StartedAt            int64
	StartReason          string
	SingBoxVersion       string
	HostBootID           *string
	DataGapBeforeSeconds int64
}

func (RuntimeSessionStart) String() string     { return "RuntimeSessionStart{redacted}" }
func (v RuntimeSessionStart) GoString() string { return v.String() }

// RuntimeSessionEnd requests closing an existing runtime session.
type RuntimeSessionEnd struct {
	ID        string
	EndedAt   int64
	EndReason string
}

func (RuntimeSessionEnd) String() string     { return "RuntimeSessionEnd{redacted}" }
func (v RuntimeSessionEnd) GoString() string { return v.String() }

// RuntimeSession is the persisted runtime session view.
type RuntimeSession struct {
	ID                   string
	StartedAt            int64
	EndedAt              *int64
	StartReason          string
	EndReason            *string
	LastTotals           ByteTotals
	LastSeenAt           int64
	SingBoxVersion       string
	HostBootID           *string
	DataGapBeforeSeconds int64
}

func (RuntimeSession) String() string     { return "RuntimeSession{redacted}" }
func (v RuntimeSession) GoString() string { return v.String() }

// QualityEvent is one already-redacted quality event attached to a batch.
type QualityEvent struct {
	Code      string
	StartedAt int64
	EndedAt   *int64
	Flags     int64
	Detail    string
}

func (QualityEvent) String() string     { return "QualityEvent{redacted}" }
func (v QualityEvent) GoString() string { return v.String() }

// Batch is one complete, stable, atomic 10-second storage transition.
type Batch struct {
	BatchID           string
	ExpectedOldTotals *ByteTotals
	NewState          CollectorCursor
	Global            TrafficRollup
	Flows             []FlowRollup
	NewRuntimeSession *RuntimeSessionStart
	EndRuntimeSession *RuntimeSessionEnd
	QualityEvents     []QualityEvent
}

func (Batch) String() string     { return "Batch{redacted}" }
func (v Batch) GoString() string { return v.String() }

// CommitResult reports whether the exact batch was already durable.
type CommitResult struct {
	AlreadyCommitted bool
}
