// Package status tracks the most recently observed connectivity quality per
// device, fed by the acquisition engine, for display in the Web UI (§16
// Device Management / Dashboard "Device status").
package status

import (
	"sync"
	"time"
)

type Info struct {
	Quality  string
	LastSeen time.Time

	// LastPollDurationMs/DatapointsPolled describe the most recent full
	// poll cycle for this device (every enabled data point whose turn came
	// up on that tick — see acquisition.OnPollCycle), for the Web UI's
	// per-device polling-timing display.
	LastPollDurationMs int64
	DatapointsPolled   int
}

type Store struct {
	mu sync.RWMutex
	m  map[int64]Info
}

func NewStore() *Store {
	return &Store{m: make(map[int64]Info)}
}

func (s *Store) Update(deviceID int64, quality string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.m[deviceID]
	info.Quality = quality
	info.LastSeen = at
	s.m[deviceID] = info
}

// UpdatePollTiming records the most recent poll cycle's timing for a device,
// leaving its Quality/LastSeen (set separately, per data point, via Update)
// untouched.
func (s *Store) UpdatePollTiming(deviceID int64, durationMs int64, datapointsPolled int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.m[deviceID]
	info.LastPollDurationMs = durationMs
	info.DatapointsPolled = datapointsPolled
	s.m[deviceID] = info
}

func (s *Store) Get(deviceID int64) (Info, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, ok := s.m[deviceID]
	return info, ok
}
