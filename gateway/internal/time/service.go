package timeservice

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Quality is the time_quality signal (§14).
type Quality string

const (
	QualitySynced   Quality = "SYNCED"
	QualityRTC      Quality = "RTC"
	QualityUnsynced Quality = "UNSYNCED"
	QualityInvalid  Quality = "INVALID"
)

// Config configures Service. NTPServer empty disables NTP sync outright
// (the service still reports RTC/UNSYNCED/INVALID via the same priority
// as an unreachable server) — useful for a host with no configured NTP
// server yet.
type Config struct {
	NTPServer    string
	SyncInterval time.Duration
	QueryTimeout time.Duration
	RTCDevice    string
}

// Status is a point-in-time snapshot for the API/dashboard (§16 "Time").
type Status struct {
	SystemTime   time.Time
	Timezone     string
	NTPServer    string
	NTPSynced    bool
	LastSync     *time.Time
	ClockOffset  *time.Duration
	RTCAvailable bool
	RTCTime      *time.Time
	TimeQuality  Quality
}

// Service runs the periodic NTP sync described in §11/§12 and tracks the
// resulting quality. It owns no goroutine until Run is called, and Run
// never blocks acquisition — it only shares the process, not the
// acquisition loop.
type Service struct {
	cfg      Config
	timezone string
	rtc      RTC
	log      *slog.Logger

	mu     sync.RWMutex
	status Status
}

func New(cfg Config, timezone string, log *slog.Logger) *Service {
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 5 * time.Minute
	}
	if cfg.QueryTimeout <= 0 {
		cfg.QueryTimeout = 3 * time.Second
	}
	if cfg.RTCDevice == "" {
		cfg.RTCDevice = "/dev/rtc0"
	}
	return &Service{
		cfg:      cfg,
		timezone: timezone,
		rtc:      newRTC(cfg.RTCDevice),
		log:      log,
		status:   Status{NTPServer: cfg.NTPServer, TimeQuality: QualityUnsynced},
	}
}

// Status returns the current snapshot with SystemTime refreshed to now —
// every other field reflects the last sync attempt.
func (s *Service) Status() Status {
	s.mu.RLock()
	st := s.status
	s.mu.RUnlock()
	st.SystemTime = time.Now().UTC()
	st.Timezone = s.timezone
	return st
}

// Run performs an initial sync attempt immediately (§12's boot sequence:
// RTC read, then NTP), then repeats every SyncInterval until ctx is done.
func (s *Service) Run(ctx context.Context) {
	s.syncOnce(ctx)

	ticker := time.NewTicker(s.cfg.SyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		}
	}
}

func (s *Service) syncOnce(ctx context.Context) {
	s.checkRTC()

	if s.cfg.NTPServer == "" {
		s.degrade()
		return
	}

	offset, err := queryNTP(ctx, s.cfg.NTPServer, s.cfg.QueryTimeout)
	if err != nil {
		s.log.Warn("ntp sync failed, falling back", "server", s.cfg.NTPServer, "error", err)
		s.degrade()
		return
	}

	now := time.Now().UTC()
	s.mu.Lock()
	s.status.NTPSynced = true
	s.status.LastSync = &now
	s.status.ClockOffset = &offset
	s.status.TimeQuality = QualitySynced
	s.mu.Unlock()
	s.log.Info("ntp sync ok", "server", s.cfg.NTPServer, "offset", offset)

	// Discipline the hardware RTC on every successful sync so a future
	// boot without NTP reachable starts from a recent, NTP-derived time
	// instead of a stale or dead-battery clock (§12). Write is a no-op
	// failure (logged at Debug) on any host without a real RTC.
	if err := s.rtc.Write(now.Add(offset)); err != nil {
		s.log.Debug("rtc write skipped", "error", err)
	}
}

func (s *Service) checkRTC() {
	t, err := s.rtc.Read()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.status.RTCAvailable = false
		s.status.RTCTime = nil
		return
	}
	s.status.RTCAvailable = true
	s.status.RTCTime = &t
}

// degrade sets TimeQuality per §11's priority (NTP -> RTC -> Local Clock)
// once NTP sync has failed or is unconfigured. It deliberately leaves
// LastSync/ClockOffset from a previous successful sync untouched — how
// long ago the gateway was last SYNCED is useful operator context even
// while degraded.
func (s *Service) degrade() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.NTPSynced = false
	switch {
	case s.status.RTCAvailable:
		s.status.TimeQuality = QualityRTC
	case isPlausible(time.Now()):
		s.status.TimeQuality = QualityUnsynced
	default:
		s.status.TimeQuality = QualityInvalid
	}
}

// isPlausible is a sanity check for a system clock that never got set at
// all (common on a Pi with no RTC and no network yet, which boots at the
// Unix epoch or the firmware build date) — Rule 10 still applies, the
// gateway keeps running, but INVALID tells operators not to trust it.
func isPlausible(t time.Time) bool {
	return t.Year() >= 2020 && t.Year() <= 2100
}
