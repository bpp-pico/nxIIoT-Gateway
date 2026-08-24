package forwarder

import (
	"sync"
	"time"
)

// Status is the forwarder's current connectivity state, for the Store &
// Forward dashboard (§16 "Server Status").
type Status struct {
	Connected     bool
	LastError     string
	LastSuccessAt *time.Time
	LastAttemptAt *time.Time
}

type statusTracker struct {
	mu sync.RWMutex
	s  Status
}

func (t *statusTracker) recordSuccess() {
	now := time.Now().UTC()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.s.Connected = true
	t.s.LastError = ""
	t.s.LastSuccessAt = &now
	t.s.LastAttemptAt = &now
}

func (t *statusTracker) recordFailure(err error) {
	now := time.Now().UTC()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.s.Connected = false
	t.s.LastError = err.Error()
	t.s.LastAttemptAt = &now
}

func (t *statusTracker) get() Status {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.s
}
