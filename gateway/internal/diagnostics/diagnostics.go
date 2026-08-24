// Package diagnostics aggregates gateway-wide Modbus counters (§16
// "Diagnostics": TX, RX, response time, timeout/CRC error/retry counts)
// for the Web UI. It is a single process-wide aggregate, not per-device,
// matching the Diagnostics panel in the design doc.
package diagnostics

import (
	"sync/atomic"
	"time"

	"nxiiot-gateway/internal/modbus"
)

type Store struct {
	tx              atomic.Int64
	rx              atomic.Int64
	timeouts        atomic.Int64
	crcErrors       atomic.Int64
	retries         atomic.Int64
	responseTotalNs atomic.Int64
	responseCount   atomic.Int64
}

func NewStore() *Store {
	return &Store{}
}

// RecordResult is called once per acquisition.Poller read attempt
// (modbus.ReadWithRetry outcome): attempts is how many requests were
// actually sent on the wire (1 + retries), quality is the final outcome,
// and duration is the time the successful exchange took — timed samples
// only count successful reads, so "average response time" reflects a
// healthy read, not a timeout's full backoff.
func (s *Store) RecordResult(quality modbus.Quality, duration time.Duration, attempts int) {
	if attempts < 1 {
		attempts = 1
	}
	s.tx.Add(int64(attempts))
	if retries := attempts - 1; retries > 0 {
		s.retries.Add(int64(retries))
	}

	switch quality {
	case modbus.Good:
		s.rx.Add(1)
		s.responseTotalNs.Add(duration.Nanoseconds())
		s.responseCount.Add(1)
	case modbus.Timeout:
		s.timeouts.Add(1)
	case modbus.CRCError:
		s.crcErrors.Add(1)
	}
}

// Snapshot is a point-in-time read of the counters for the API.
type Snapshot struct {
	TXCount           int64
	RXCount           int64
	TimeoutCount      int64
	CRCErrorCount     int64
	RetryCount        int64
	AvgResponseTimeMs float64
}

func (s *Store) Snapshot() Snapshot {
	count := s.responseCount.Load()
	var avg float64
	if count > 0 {
		avg = float64(s.responseTotalNs.Load()) / float64(count) / float64(time.Millisecond)
	}
	return Snapshot{
		TXCount:           s.tx.Load(),
		RXCount:           s.rx.Load(),
		TimeoutCount:      s.timeouts.Load(),
		CRCErrorCount:     s.crcErrors.Load(),
		RetryCount:        s.retries.Load(),
		AvgResponseTimeMs: avg,
	}
}
