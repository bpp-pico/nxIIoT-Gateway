package netconfig

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Service wraps a Controller with the confirm-or-revert safety net every
// router/switch-style admin UI uses for network changes: Apply takes
// effect immediately, but is automatically rolled back after
// confirmWindow unless Confirm is called first. This is the actual point
// of this package — a typo'd static IP or gateway can otherwise lock an
// operator out of a headless device with no way back in short of
// physical/console access.
type Service struct {
	ctrl Controller
	log  *slog.Logger

	mu       sync.Mutex
	previous *Info
	timer    *time.Timer
}

func NewService(ctrl Controller, log *slog.Logger) *Service {
	return &Service{ctrl: ctrl, log: log}
}

func (s *Service) Current() (Info, error) {
	return s.ctrl.Current()
}

// Apply snapshots the current config, applies cfg, and schedules an
// automatic revert after confirmWindow unless Confirm is called first.
// Calling Apply again while a previous change is still unconfirmed
// replaces the pending revert with a fresh one targeting the config
// captured just now — each Apply is its own checkpoint, not stacked on
// the original pre-change state.
func (s *Service) Apply(cfg StaticConfig, confirmWindow time.Duration) error {
	prev, err := s.ctrl.Current()
	if err != nil {
		return fmt.Errorf("read current network config: %w", err)
	}

	if err := s.ctrl.ApplyStatic(cfg); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	prevCopy := prev
	s.previous = &prevCopy
	s.timer = time.AfterFunc(confirmWindow, func() { s.revert(prevCopy) })
	return nil
}

func (s *Service) revert(prev Info) {
	s.log.Warn("network change not confirmed in time, reverting", "interface", prev.Interface)

	var err error
	if prev.Method == "manual" {
		err = s.ctrl.ApplyStatic(StaticConfig{
			Interface: prev.Interface,
			Address:   prev.Address,
			Prefix:    prev.Prefix,
			Gateway:   prev.Gateway,
			DNS:       prev.DNS,
		})
	} else {
		err = s.ctrl.ApplyDHCP(prev.Interface)
	}
	if err != nil {
		s.log.Error("failed to revert network change — manual recovery may be required", "error", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.previous = nil
	s.timer = nil
}

// Confirm cancels the pending revert, making the last Apply permanent.
func (s *Service) Confirm() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer == nil {
		return fmt.Errorf("no pending network change to confirm")
	}
	s.timer.Stop()
	s.timer = nil
	s.previous = nil
	return nil
}

// Pending reports whether an unconfirmed change is currently awaiting
// confirmation (or automatic revert).
func (s *Service) Pending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.timer != nil
}
