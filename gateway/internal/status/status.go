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
	s.m[deviceID] = Info{Quality: quality, LastSeen: at}
}

func (s *Store) Get(deviceID int64) (Info, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, ok := s.m[deviceID]
	return info, ok
}
