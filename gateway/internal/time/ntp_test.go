package timeservice

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// startFakeNTPServer runs a real UDP socket that replies to any SNTP
// request with a response whose receive/transmit timestamps are
// serverOffset ahead of the local clock — a real wire-format exchange,
// not a mocked queryNTP.
func startFakeNTPServer(t *testing.T, serverOffset time.Duration) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	go func() {
		buf := make([]byte, 48)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n < 48 {
				continue
			}

			serverNow := time.Now().UTC().Add(serverOffset)
			secs, frac := timeToNTPTimestamp(serverNow)

			var resp [48]byte
			resp[0] = (3 << 3) | 4 // LI=0, VN=3, Mode=4 (server)
			resp[1] = 1            // stratum 1
			binary.BigEndian.PutUint32(resp[32:36], secs)
			binary.BigEndian.PutUint32(resp[36:40], frac)
			binary.BigEndian.PutUint32(resp[40:44], secs)
			binary.BigEndian.PutUint32(resp[44:48], frac)

			_, _ = conn.WriteToUDP(resp[:], addr)
		}
	}()

	return conn.LocalAddr().String()
}

func timeToNTPTimestamp(t time.Time) (secs, frac uint32) {
	since := t.Sub(ntpEpoch)
	secs = uint32(since / time.Second)
	frac = uint32(uint64(since%time.Second) << 32 / uint64(time.Second))
	return secs, frac
}

func TestQueryNTPComputesOffsetAgainstRealServer(t *testing.T) {
	want := 5 * time.Second
	addr := startFakeNTPServer(t, want)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	offset, err := queryNTP(ctx, addr, time.Second)
	if err != nil {
		t.Fatalf("queryNTP: %v", err)
	}

	diff := offset - want
	if diff < -200*time.Millisecond || diff > 200*time.Millisecond {
		t.Errorf("offset = %v, want ~%v", offset, want)
	}
}

func TestQueryNTPComputesNegativeOffset(t *testing.T) {
	want := -3 * time.Second
	addr := startFakeNTPServer(t, want)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	offset, err := queryNTP(ctx, addr, time.Second)
	if err != nil {
		t.Fatalf("queryNTP: %v", err)
	}

	diff := offset - want
	if diff < -200*time.Millisecond || diff > 200*time.Millisecond {
		t.Errorf("offset = %v, want ~%v", offset, want)
	}
}

func TestQueryNTPFailsWhenNothingListening(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := conn.LocalAddr().String()
	conn.Close() // nothing is listening on addr now

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := queryNTP(ctx, addr, 300*time.Millisecond); err == nil {
		t.Fatal("expected queryNTP to fail against an unreachable server")
	}
}
