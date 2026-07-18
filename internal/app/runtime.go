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
}

// Runtime owns the single-source global collector state machine.
type Runtime struct {
	client   *clashapi.Client
	store    *storage.Store
	ring     *collector.Ring
	status   *flowstatus.Tracker
	timezone string
	interval time.Duration
	version  string

	counter      *collector.CounterTracker
	durable      storage.CollectorState
	hasDurable   bool
	sessionID    string
	bucket       *collector.GlobalBucket
	lastSampleAt int64
	newSession   *storage.RuntimeSessionStart
	endSession   *storage.RuntimeSessionEnd
	pending      *storage.Batch
}

// NewRuntime loads the durable cursor without advancing it.
func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	if options.Client == nil || options.Store == nil || options.Ring == nil || options.Status == nil ||
		options.BucketTimezone == "" || options.ConnectionsInterval <= 0 {
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
		counter: counter, durable: state, hasDurable: found, sessionID: state.RuntimeSessionID,
	}, nil
}

// ObserveConnections fetches and applies one authoritative cumulative snapshot.
func (r *Runtime) ObserveConnections(ctx context.Context, at time.Time, afterGap bool) error {
	if r.pending != nil {
		return ErrRuntimeState
	}
	snapshot, err := r.client.Connections(ctx)
	if err != nil {
		_ = r.status.Set(flowstatus.LevelDegraded, "clash_unavailable", true)
		return errors.New("cannot collect FlowLens connection snapshot")
	}
	seconds := at.UTC().Unix()
	start := seconds / 10 * 10
	if start <= 0 {
		return ErrRuntimeState
	}
	if r.bucket != nil && start > r.bucket.Rollup().BucketStart {
		if err := r.Seal(ctx, time.Unix(r.bucket.Rollup().BucketEnd, 0)); err != nil {
			return err
		}
	}
	if r.bucket == nil {
		r.bucket, err = collector.NewGlobalBucket(start)
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
	observation, err := r.counter.Observe(currentTotals, afterGap)
	if err != nil {
		return ErrRuntimeState
	}
	if r.sessionID == "" {
		id, err := randomID()
		if err != nil {
			return err
		}
		r.sessionID = id
		r.newSession = &storage.RuntimeSessionStart{
			ID: id, StartedAt: seconds, StartReason: "startup", SingBoxVersion: r.version,
		}
	} else if observation.NewSession {
		id, err := randomID()
		if err != nil {
			return err
		}
		if r.newSession == nil {
			r.endSession = &storage.RuntimeSessionEnd{ID: r.sessionID, EndedAt: seconds, EndReason: "counter_reset"}
		}
		r.newSession = &storage.RuntimeSessionStart{
			ID: id, StartedAt: seconds, StartReason: "counter_reset", SingBoxVersion: r.version,
		}
		r.sessionID = id
	}
	if err := r.bucket.ObserveCounter(seconds, observation, int64(len(snapshot.Connections))); err != nil {
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
	if r.bucket == nil {
		return nil
	}
	seconds := at.UTC().Unix()
	rollup := r.bucket.Rollup()
	if seconds < rollup.BucketStart || seconds >= rollup.BucketEnd {
		return nil
	}
	return r.bucket.ObserveTraffic(seconds, sample)
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
		if rollup.QualityFlags != 0 {
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
