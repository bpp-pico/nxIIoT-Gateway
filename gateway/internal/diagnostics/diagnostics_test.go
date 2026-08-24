package diagnostics_test

import (
	"testing"
	"time"

	"nxiiot-gateway/internal/diagnostics"
	"nxiiot-gateway/internal/modbus"
)

func TestRecordResultCountsSuccessfulRead(t *testing.T) {
	s := diagnostics.NewStore()
	s.RecordResult(modbus.Good, 50*time.Millisecond, 1)

	got := s.Snapshot()
	if got.TXCount != 1 || got.RXCount != 1 {
		t.Errorf("TX/RX = %d/%d, want 1/1", got.TXCount, got.RXCount)
	}
	if got.RetryCount != 0 {
		t.Errorf("RetryCount = %d, want 0", got.RetryCount)
	}
	if got.AvgResponseTimeMs != 50 {
		t.Errorf("AvgResponseTimeMs = %v, want 50", got.AvgResponseTimeMs)
	}
}

func TestRecordResultCountsRetriesOnEventualSuccess(t *testing.T) {
	s := diagnostics.NewStore()
	s.RecordResult(modbus.Good, 10*time.Millisecond, 3) // 1 initial + 2 retries

	got := s.Snapshot()
	if got.TXCount != 3 {
		t.Errorf("TXCount = %d, want 3", got.TXCount)
	}
	if got.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", got.RetryCount)
	}
	if got.RXCount != 1 {
		t.Errorf("RXCount = %d, want 1", got.RXCount)
	}
}

func TestRecordResultClassifiesFailures(t *testing.T) {
	s := diagnostics.NewStore()
	s.RecordResult(modbus.Timeout, 0, 4)
	s.RecordResult(modbus.CRCError, 0, 1)

	got := s.Snapshot()
	if got.TimeoutCount != 1 {
		t.Errorf("TimeoutCount = %d, want 1", got.TimeoutCount)
	}
	if got.CRCErrorCount != 1 {
		t.Errorf("CRCErrorCount = %d, want 1", got.CRCErrorCount)
	}
	if got.RXCount != 0 {
		t.Errorf("RXCount = %d, want 0 (no successful reads)", got.RXCount)
	}
	if got.TXCount != 5 {
		t.Errorf("TXCount = %d, want 5 (4 attempts + 1 attempt)", got.TXCount)
	}
	// A failed read's elapsed time must not pollute the "healthy read"
	// average response time.
	if got.AvgResponseTimeMs != 0 {
		t.Errorf("AvgResponseTimeMs = %v, want 0 (no successful reads timed)", got.AvgResponseTimeMs)
	}
}

func TestSnapshotIsIndependentOfLaterWrites(t *testing.T) {
	s := diagnostics.NewStore()
	s.RecordResult(modbus.Good, time.Millisecond, 1)
	first := s.Snapshot()

	s.RecordResult(modbus.Good, time.Millisecond, 1)

	if first.RXCount != 1 {
		t.Errorf("earlier snapshot mutated: RXCount = %d, want 1", first.RXCount)
	}
}
