package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

const maxSessions = 64

type session struct {
	expiresAt time.Time
	sequence  uint64
}

// SessionStore holds the bounded process-local login sessions.
type SessionStore struct {
	ttl time.Duration

	mutex    sync.Mutex
	sessions map[string]session
	next     uint64
}

// NewSessionStore returns an empty session store with absolute expiry.
func NewSessionStore(ttl time.Duration) (*SessionStore, error) {
	if ttl <= 0 {
		return nil, errors.New("FlowLens session TTL must be positive")
	}
	return &SessionStore{ttl: ttl, sessions: make(map[string]session)}, nil
}

// String prevents session disclosure through formatting.
func (*SessionStore) String() string { return "SessionStore{redacted}" }

// GoString prevents session disclosure through Go-syntax formatting.
func (s *SessionStore) GoString() string { return s.String() }

// Create inserts one unpredictable session, evicting only when required.
func (s *SessionStore) Create(now time.Time) (string, error) {
	for range 4 {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return "", errors.New("cannot create FlowLens session")
		}
		id := base64.RawURLEncoding.EncodeToString(bytes)
		s.mutex.Lock()
		s.removeExpiredLocked(now)
		if _, exists := s.sessions[id]; exists {
			s.mutex.Unlock()
			continue
		}
		for len(s.sessions) >= maxSessions {
			s.evictOldestLocked()
		}
		s.next++
		s.sessions[id] = session{expiresAt: now.Add(s.ttl), sequence: s.next}
		s.mutex.Unlock()
		return id, nil
	}
	return "", errors.New("cannot create FlowLens session")
}

// Valid reports whether an ID is present and not absolutely expired.
func (s *SessionStore) Valid(id string, now time.Time) bool {
	if id == "" {
		return false
	}
	s.mutex.Lock()
	entry, exists := s.sessions[id]
	if exists && !now.Before(entry.expiresAt) {
		delete(s.sessions, id)
		exists = false
	}
	s.mutex.Unlock()
	return exists
}

// Delete revokes one session.
func (s *SessionStore) Delete(id string) {
	s.mutex.Lock()
	delete(s.sessions, id)
	s.mutex.Unlock()
}

// Len removes expired sessions and returns the live count.
func (s *SessionStore) Len(now time.Time) int {
	s.mutex.Lock()
	s.removeExpiredLocked(now)
	count := len(s.sessions)
	s.mutex.Unlock()
	return count
}

func (s *SessionStore) removeExpiredLocked(now time.Time) {
	for id, entry := range s.sessions {
		if !now.Before(entry.expiresAt) {
			delete(s.sessions, id)
		}
	}
}

func (s *SessionStore) evictOldestLocked() {
	var oldestID string
	var oldestSequence uint64
	for id, entry := range s.sessions {
		if oldestID == "" || entry.sequence < oldestSequence {
			oldestID = id
			oldestSequence = entry.sequence
		}
	}
	delete(s.sessions, oldestID)
}
