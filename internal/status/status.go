// Package status owns the current minimal process and readiness snapshot.
package status

import (
	"errors"
	"sync"
)

// Level is the stable public service status value.
type Level string

const (
	LevelOK       Level = "ok"
	LevelDegraded Level = "degraded"
	LevelFailed   Level = "failed"
)

// ErrInvalidSnapshot means a status transition is not safe to publish.
var ErrInvalidSnapshot = errors.New("invalid FlowLens status snapshot")

// Snapshot is the complete current status value.
type Snapshot struct {
	Level  Level
	Reason string
	Ready  bool
}

// Tracker stores one concurrency-safe process snapshot.
type Tracker struct {
	mutex      sync.RWMutex
	components map[string]Snapshot
	snapshot   Snapshot
}

// NewTracker returns the fixed startup state.
func NewTracker() *Tracker {
	starting := Snapshot{Level: LevelDegraded, Reason: "starting"}
	return &Tracker{
		components: map[string]Snapshot{"runtime": starting},
		snapshot:   starting,
	}
}

// Set replaces the complete snapshot after validating its public values.
func (t *Tracker) Set(level Level, reason string, ready bool) error {
	return t.SetComponent("runtime", level, reason, ready)
}

// SetComponent replaces one component snapshot and derives the public process state.
func (t *Tracker) SetComponent(component string, level Level, reason string, ready bool) error {
	if !validReason(component) || !validLevel(level) || !validReason(reason) {
		return ErrInvalidSnapshot
	}
	t.mutex.Lock()
	t.components[component] = Snapshot{Level: level, Reason: reason, Ready: ready}
	t.snapshot = combinedSnapshot(t.components)
	t.mutex.Unlock()
	return nil
}

// Snapshot returns a value copy of the current state.
func (t *Tracker) Snapshot() Snapshot {
	t.mutex.RLock()
	snapshot := t.snapshot
	t.mutex.RUnlock()
	return snapshot
}

func validLevel(level Level) bool {
	return level == LevelOK || level == LevelDegraded || level == LevelFailed
}

func combinedSnapshot(components map[string]Snapshot) Snapshot {
	result := Snapshot{Level: LevelOK, Reason: "ready", Ready: true}
	selectedPriority := 0
	for _, snapshot := range components {
		priority := levelPriority(snapshot.Level)
		if priority > selectedPriority ||
			(priority == selectedPriority && snapshot.Level != LevelOK && snapshot.Reason < result.Reason) {
			result.Level = snapshot.Level
			result.Reason = snapshot.Reason
			selectedPriority = priority
		}
		result.Ready = result.Ready && snapshot.Ready
	}
	return result
}

func levelPriority(level Level) int {
	switch level {
	case LevelFailed:
		return 2
	case LevelDegraded:
		return 1
	default:
		return 0
	}
}

func validReason(reason string) bool {
	if len(reason) == 0 || len(reason) > 64 || reason[0] < 'a' || reason[0] > 'z' {
		return false
	}
	for _, character := range reason[1:] {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}
