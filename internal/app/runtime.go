package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/collector"
	flowstatus "github.com/Willxup/flowlens/internal/status"
	"github.com/Willxup/flowlens/internal/storage"
)

// ErrRuntimeState means the Stage 1 collector cannot safely advance.
var ErrRuntimeState = errors.New("invalid FlowLens collector runtime state")

// RuntimeOptions binds the already-validated Stage 1 dependencies.
type RuntimeOptions struct {
	Client              *clashapi.Client
	Store               *storage.Store
	Ring                *collector.Ring
	Status              *flowstatus.Tracker
	BucketTimezone      string
	ConnectionsInterval time.Duration
	Attribution         *attribution.Tracker
	TopK                int
}

// Runtime owns the single-source global collector state machine.
type Runtime struct {
	client      *clashapi.Client
	store       *storage.Store
	ring        *collector.Ring
	status      *flowstatus.Tracker
	timezone    string
	interval    time.Duration
	version     string
	attribution *attribution.Tracker
	topK        int

	counter      *collector.CounterTracker
	durable      storage.CollectorState
	hasDurable   bool
	sessionID    string
	bucket       *collector.GlobalBucket
	lastSampleAt int64
	newSession   *storage.RuntimeSessionStart
	endSession   *storage.RuntimeSessionEnd
	pending      *storage.Batch
	trafficQueue []deferredTrafficSample
}

type deferredTrafficSample struct {
	at     int64
	sample clashapi.TrafficSample
}

const maxDeferredTrafficSamples = 16

// NewRuntime loads the durable cursor without advancing it.
func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	if options.Client == nil || options.Store == nil || options.Ring == nil || options.Status == nil ||
		options.Attribution == nil || options.BucketTimezone == "" || options.ConnectionsInterval <= 0 ||
		options.TopK < 1 || options.TopK > 100 {
		return nil, ErrRuntimeState
	}
	version, err := options.Client.Version(ctx)
	if err != nil || version.Version == "" {
		return nil, errors.New("cannot initialize FlowLens collector runtime")
	}
	state, found, err := options.Store.LoadCollectorState(ctx)
	if err != nil {
		return nil, err
	}
	if found && state.BucketTimezone != options.BucketTimezone {
		return nil, storage.ErrTimezoneMismatch
	}
	var persisted *collector.ByteTotals
	if found {
		persisted = &collector.ByteTotals{Upload: state.LastTotals.Upload, Download: state.LastTotals.Download}
	}
	counter, err := collector.NewCounterTracker(persisted)
	if err != nil {
		return nil, ErrRuntimeState
	}
	return &Runtime{
		client: options.Client, store: options.Store, ring: options.Ring, status: options.Status,
		timezone: options.BucketTimezone, interval: options.ConnectionsInterval, version: version.Version,
		attribution: options.Attribution, topK: options.TopK,
		counter: counter, durable: state, hasDurable: found, sessionID: state.RuntimeSessionID,
		lastSampleAt: state.LastSampleAt,
	}, nil
}

// ObserveConnections fetches and applies one authoritative cumulative snapshot.
func (r *Runtime) ObserveConnections(ctx context.Context, at time.Time, afterGap bool) error {
	snapshot, err := r.fetchConnections(ctx)
	if err != nil {
		return err
	}
	return r.applyConnections(ctx, at, afterGap, snapshot)
}

func (r *Runtime) observeConnectionsNow(ctx context.Context, afterGap bool) error {
	snapshot, err := r.fetchConnections(ctx)
	if err != nil {
		return err
	}
	return r.applyConnections(ctx, time.Now(), afterGap, snapshot)
}

func (r *Runtime) fetchConnections(ctx context.Context) (clashapi.ConnectionsSnapshot, error) {
	if r.pending != nil {
		return clashapi.ConnectionsSnapshot{}, ErrRuntimeState
	}
	snapshot, err := r.client.Connections(ctx)
	if err != nil {
		_ = r.status.Set(flowstatus.LevelDegraded, "clash_unavailable", true)
		return clashapi.ConnectionsSnapshot{}, errors.New("cannot collect FlowLens connection snapshot")
	}
	return snapshot, nil
}

func (r *Runtime) applyConnections(
	ctx context.Context,
	at time.Time,
	afterGap bool,
	snapshot clashapi.ConnectionsSnapshot,
) error {
	seconds := at.UTC().Unix()
	start := seconds / 10 * 10
	if start <= 0 {
		return ErrRuntimeState
	}
	if r.lastSampleAt > 0 && seconds < r.lastSampleAt {
		_ = r.status.Set(flowstatus.LevelDegraded, "clock_unstable", true)
		return ErrRuntimeState
	}
	if r.bucket != nil && start > r.bucket.Rollup().BucketStart {
		if err := r.Seal(ctx, time.Unix(r.bucket.Rollup().BucketEnd, 0)); err != nil {
			return err
		}
	}
	bucket := r.bucket
	var err error
	if bucket == nil {
		bucket, err = collector.NewGlobalBucket(start, r.topK)
		if err != nil {
			return ErrRuntimeState
		}
	}
	currentTotals := collector.ByteTotals{
		Upload: snapshot.UploadTotal, Download: snapshot.DownloadTotal,
	}
	previousTotals, hasPrevious := r.counter.Last()
	willReset := hasPrevious &&
		(currentTotals.Upload < previousTotals.Upload || currentTotals.Download < previousTotals.Download)
	if willReset && r.endSession != nil {
		return ErrRuntimeState
	}
	observation, err := r.counter.Preview(currentTotals, afterGap)
	if err != nil {
		return ErrRuntimeState
	}
	preparedAttribution, err := r.attribution.Prepare(
		at,
		snapshot.Connections,
		storage.ByteTotals{Upload: observation.Delta.Upload, Download: observation.Delta.Download},
		observation.Baseline || afterGap || observation.NewSession,
	)
	if err != nil {
		_ = r.status.Set(flowstatus.LevelDegraded, "attribution_unavailable", true)
		return ErrRuntimeState
	}
	nextSessionID := r.sessionID
	nextNewSession := r.newSession
	nextEndSession := r.endSession
	if nextSessionID == "" {
		id, err := randomID()
		if err != nil {
			return err
		}
		nextSessionID = id
		nextNewSession = &storage.RuntimeSessionStart{
			ID: id, StartedAt: seconds, StartReason: "startup", SingBoxVersion: r.version,
		}
	} else if observation.NewSession {
		id, err := randomID()
		if err != nil {
			return err
		}
		if nextNewSession == nil {
			nextEndSession = &storage.RuntimeSessionEnd{ID: nextSessionID, EndedAt: seconds, EndReason: "counter_reset"}
		}
		nextNewSession = &storage.RuntimeSessionStart{
			ID: id, StartedAt: seconds, StartReason: "counter_reset", SingBoxVersion: r.version,
		}
		nextSessionID = id
	}
	if err := bucket.ObserveConnections(
		seconds, observation, int64(len(snapshot.Connections)), preparedAttribution.Contribution(),
	); err != nil {
		return err
	}
	r.bucket = bucket
	r.counter.Commit(observation)
	r.attribution.Commit(preparedAttribution)
	r.sessionID = nextSessionID
	r.newSession = nextNewSession
	r.endSession = nextEndSession
	if err := r.flushDeferredTraffic(); err != nil {
		return err
	}
	r.lastSampleAt = seconds
	_ = r.status.Set(flowstatus.LevelOK, "ready", true)
	return nil
}

// ObserveTraffic writes one live sample and, when possible, its current bucket value.
func (r *Runtime) ObserveTraffic(at time.Time, sample clashapi.TrafficSample) error {
	level := r.status.Snapshot().Level
	sampleStatus := collector.SampleStatusOK
	if level != flowstatus.LevelOK {
		sampleStatus = collector.SampleStatusDegraded
	}
	if err := r.ring.Add(collector.SpeedSample{
		Timestamp: at, UploadBytesPerSecond: sample.Up, DownloadBytesPerSecond: sample.Down, Status: sampleStatus,
	}); err != nil {
		return err
	}
	seconds := at.UTC().Unix()
	if r.bucket == nil {
		r.deferTraffic(seconds, sample)
		return nil
	}
	rollup := r.bucket.Rollup()
	if seconds < rollup.BucketStart {
		return nil
	}
	if seconds >= rollup.BucketEnd {
		r.deferTraffic(seconds, sample)
		return nil
	}
	return r.bucket.ObserveTraffic(seconds, sample)
}

func (r *Runtime) deferTraffic(at int64, sample clashapi.TrafficSample) {
	start := at / 10 * 10
	if start <= 0 {
		return
	}
	if len(r.trafficQueue) > 0 && r.trafficQueue[0].at/10*10 != start {
		r.trafficQueue = r.trafficQueue[:0]
	}
	if len(r.trafficQueue) == maxDeferredTrafficSamples {
		copy(r.trafficQueue, r.trafficQueue[1:])
		r.trafficQueue = r.trafficQueue[:len(r.trafficQueue)-1]
	}
	r.trafficQueue = append(r.trafficQueue, deferredTrafficSample{at: at, sample: sample})
}

func (r *Runtime) flushDeferredTraffic() error {
	if r.bucket == nil || len(r.trafficQueue) == 0 {
		return nil
	}
	rollup := r.bucket.Rollup()
	remaining := r.trafficQueue[:0]
	for _, queued := range r.trafficQueue {
		switch {
		case queued.at < rollup.BucketStart:
			continue
		case queued.at >= rollup.BucketEnd:
			remaining = append(remaining, queued)
		default:
			if err := r.bucket.ObserveTraffic(queued.at, queued.sample); err != nil {
				return err
			}
		}
	}
	r.trafficQueue = remaining
	return nil
}

// Seal commits the current or previously failed immutable batch.
func (r *Runtime) Seal(ctx context.Context, at time.Time) error {
	if r.pending == nil {
		if r.bucket == nil || at.UTC().Unix() < r.bucket.Rollup().BucketEnd {
			return nil
		}
		last, found := r.counter.Last()
		if !found || r.sessionID == "" || r.lastSampleAt < r.bucket.Rollup().BucketStart {
			return ErrRuntimeState
		}
		rollup := r.bucket.Rollup()
		batch := storage.Batch{
			BatchID: stableBatchID(r.sessionID, rollup.BucketStart, rollup.BucketEnd),
			NewState: storage.CollectorCursor{
				RuntimeSessionID: r.sessionID,
				LastTotals:       storage.ByteTotals{Upload: last.Upload, Download: last.Download},
				LastSampleAt:     r.lastSampleAt, BucketTimezone: r.timezone,
			},
			Global: rollup, Flows: r.bucket.Flows(), NewRuntimeSession: r.newSession, EndRuntimeSession: r.endSession,
		}
		if r.hasDurable {
			expected := r.durable.LastTotals
			batch.ExpectedOldTotals = &expected
		}
		if rollup.QualityFlags&^collector.QualityFlagAttributionIncomplete != 0 {
			batch.QualityEvents = []storage.QualityEvent{{
				Code: "collector_quality", StartedAt: rollup.BucketStart, Flags: rollup.QualityFlags,
			}}
		}
		r.pending = &batch
	}
	result, err := r.store.CommitBatch(ctx, *r.pending)
	if err != nil {
		_ = r.status.Set(flowstatus.LevelDegraded, "storage_unavailable", true)
		return errors.New("cannot persist FlowLens collector batch")
	}
	_ = result
	r.durable = storage.CollectorState{CollectorCursor: r.pending.NewState, LastBatchID: r.pending.BatchID}
	r.hasDurable = true
	r.pending = nil
	r.bucket = nil
	r.newSession = nil
	r.endSession = nil
	_ = r.status.Set(flowstatus.LevelOK, "ready", true)
	return nil
}

func randomID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.New("cannot create FlowLens runtime identifier")
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func stableBatchID(sessionID string, start, end int64) string {
	sum := sha256.Sum256([]byte(sessionID + ":" + strconv.FormatInt(start, 10) + ":" + strconv.FormatInt(end, 10)))
	return hex.EncodeToString(sum[:])
}
