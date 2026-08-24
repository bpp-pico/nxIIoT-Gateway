package timeservice

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

// fakeRTC lets tests control RTC availability/content directly, rather
// than depending on rtc_linux.go/rtc_other.go's real (and, in a test
// environment, always-unavailable) hardware behavior.
type fakeRTC struct {
	available bool
	t         time.Time
	writes    []time.Time
}

func (r *fakeRTC) Read() (time.Time, error) {
	if !r.available {
		return time.Time{}, ErrRTCUnavailable
	}
	return r.t, nil
}

func (r *fakeRTC) Write(t time.Time) error {
	if !r.available {
		return ErrRTCUnavailable
	}
	r.writes = append(r.writes, t)
	return nil
}

func newTestService(cfg Config, rtc RTC) *Service {
	return &Service{
		cfg:      cfg,
		timezone: "UTC",
		rtc:      rtc,
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		status:   Status{NTPServer: cfg.NTPServer, TimeQuality: QualityUnsynced},
	}
}

func TestServiceReportsSyncedAfterSuccessfulNTPSync(t *testing.T) {
	addr := startFakeNTPServer(t, 2*time.Second)
	svc := newTestService(Config{NTPServer: addr, QueryTimeout: time.Second}, &fakeRTC{})

	svc.syncOnce(context.Background())

	st := svc.Status()
	if st.TimeQuality != QualitySynced {
		t.Errorf("TimeQuality = %s, want SYNCED", st.TimeQuality)
	}
	if !st.NTPSynced {
		t.Error("expected NTPSynced = true")
	}
	if st.LastSync == nil {
		t.Fatal("expected LastSync to be set")
	}
	if st.ClockOffset == nil || *st.ClockOffset < time.Second || *st.ClockOffset > 3*time.Second {
		t.Errorf("ClockOffset = %v, want ~2s", st.ClockOffset)
	}
}

func TestServiceDisciplinesRTCOnSuccessfulSync(t *testing.T) {
	addr := startFakeNTPServer(t, time.Second)
	rtc := &fakeRTC{available: true, t: time.Now().Add(-time.Hour)}
	svc := newTestService(Config{NTPServer: addr, QueryTimeout: time.Second}, rtc)

	svc.syncOnce(context.Background())

	if len(rtc.writes) != 1 {
		t.Fatalf("expected exactly 1 RTC write, got %d", len(rtc.writes))
	}
	if diff := time.Since(rtc.writes[0]); diff < -2*time.Second || diff > 2*time.Second {
		t.Errorf("rtc write time %v is not close to now", rtc.writes[0])
	}
}

func TestServiceDegradesToRTCWhenNTPUnreachable(t *testing.T) {
	conn := reserveClosedUDPAddr(t)
	rtcTime := time.Now().Add(-10 * time.Minute)
	svc := newTestService(Config{NTPServer: conn, QueryTimeout: 200 * time.Millisecond}, &fakeRTC{available: true, t: rtcTime})

	svc.syncOnce(context.Background())

	st := svc.Status()
	if st.TimeQuality != QualityRTC {
		t.Errorf("TimeQuality = %s, want RTC", st.TimeQuality)
	}
	if st.NTPSynced {
		t.Error("expected NTPSynced = false")
	}
	if st.RTCTime == nil || !st.RTCTime.Equal(rtcTime) {
		t.Errorf("RTCTime = %v, want %v", st.RTCTime, rtcTime)
	}
}

func TestServiceDegradesToUnsyncedWhenNoNTPAndNoRTC(t *testing.T) {
	svc := newTestService(Config{NTPServer: "", QueryTimeout: 200 * time.Millisecond}, &fakeRTC{available: false})

	svc.syncOnce(context.Background())

	st := svc.Status()
	if st.TimeQuality != QualityUnsynced {
		t.Errorf("TimeQuality = %s, want UNSYNCED", st.TimeQuality)
	}
	if st.RTCAvailable {
		t.Error("expected RTCAvailable = false")
	}
}

func TestServiceKeepsLastSyncInfoWhileDegraded(t *testing.T) {
	addr := startFakeNTPServer(t, time.Second)
	svc := newTestService(Config{NTPServer: addr, QueryTimeout: 200 * time.Millisecond}, &fakeRTC{})

	svc.syncOnce(context.Background())
	st := svc.Status()
	if st.TimeQuality != QualitySynced {
		t.Fatalf("precondition failed: expected SYNCED after first sync, got %s", st.TimeQuality)
	}
	firstSync := st.LastSync
	firstOffset := st.ClockOffset

	// Now point at an unreachable server and sync again.
	svc.cfg.NTPServer = reserveClosedUDPAddr(t)
	svc.syncOnce(context.Background())

	st = svc.Status()
	if st.TimeQuality != QualityUnsynced {
		t.Errorf("TimeQuality = %s, want UNSYNCED after NTP goes unreachable", st.TimeQuality)
	}
	if st.LastSync == nil || !st.LastSync.Equal(*firstSync) {
		t.Errorf("LastSync changed after a failed sync: got %v, want unchanged %v", st.LastSync, firstSync)
	}
	if st.ClockOffset == nil || *st.ClockOffset != *firstOffset {
		t.Errorf("ClockOffset changed after a failed sync: got %v, want unchanged %v", st.ClockOffset, firstOffset)
	}
}

func TestServiceRunPerformsAnImmediateSync(t *testing.T) {
	addr := startFakeNTPServer(t, 0)
	svc := newTestService(Config{NTPServer: addr, QueryTimeout: time.Second, SyncInterval: time.Hour}, &fakeRTC{})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	svc.Run(ctx)

	if st := svc.Status(); st.TimeQuality != QualitySynced {
		t.Errorf("TimeQuality = %s, want SYNCED after Run's initial sync", st.TimeQuality)
	}
}

// reserveClosedUDPAddr returns a UDP address nothing is listening on, so
// queryNTP fails (by timeout or connection-refused, depending on OS) —
// used to exercise the degrade path without a flaky external dependency.
func reserveClosedUDPAddr(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()
	return addr
}
