package timeservice

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

// ntpEpoch is the NTP wire format's zero point (1900-01-01), 70 years
// before the Unix epoch.
var ntpEpoch = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

// queryNTP performs a minimal SNTP request/response exchange (RFC 4330)
// against server ("host" or "host:port", defaulting to port 123) and
// returns the clock offset (server time minus local time) computed from
// the four exchange timestamps (Mills formula):
//
//	offset = ((T2-T1) + (T3-T4)) / 2
//
// where T1/T4 are local send/receive times and T2/T3 are the server's
// receive/transmit times from the reply.
func queryNTP(ctx context.Context, server string, timeout time.Duration) (time.Duration, error) {
	addr := server
	if _, _, err := net.SplitHostPort(server); err != nil {
		addr = net.JoinHostPort(server, "123")
	}

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "udp", addr)
	if err != nil {
		return 0, fmt.Errorf("dial ntp server %s: %w", addr, err)
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return 0, fmt.Errorf("set ntp deadline: %w", err)
	}

	var req [48]byte
	req[0] = 0x1B // LI=0 (no warning), VN=3, Mode=3 (client)

	t1 := time.Now().UTC()
	if _, err := conn.Write(req[:]); err != nil {
		return 0, fmt.Errorf("send ntp request: %w", err)
	}

	var resp [48]byte
	n, err := conn.Read(resp[:])
	t4 := time.Now().UTC()
	if err != nil {
		return 0, fmt.Errorf("read ntp response: %w", err)
	}
	if n < 48 {
		return 0, fmt.Errorf("short ntp response: %d bytes", n)
	}
	if mode := resp[0] & 0x07; mode != 4 { // 4 = server
		return 0, fmt.Errorf("unexpected ntp mode %d in response", mode)
	}
	if stratum := resp[1]; stratum == 0 {
		return 0, fmt.Errorf("ntp server reported kiss-of-death (stratum 0)")
	}

	t2 := ntpTimestampToTime(resp[32:40])
	t3 := ntpTimestampToTime(resp[40:48])

	offset := (t2.Sub(t1) + t3.Sub(t4)) / 2
	return offset, nil
}

func ntpTimestampToTime(b []byte) time.Time {
	secs := binary.BigEndian.Uint32(b[0:4])
	frac := binary.BigEndian.Uint32(b[4:8])
	d := time.Duration(secs)*time.Second + time.Duration(uint64(frac)*uint64(time.Second)>>32)
	return ntpEpoch.Add(d)
}
