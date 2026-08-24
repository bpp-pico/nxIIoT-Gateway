package modbus

import (
	"net"
	"testing"
	"time"

	goburrow "github.com/goburrow/modbus"
)

// TestQualityFromErrorConnectionRefused exercises a real OS-level refused
// TCP connection rather than a mocked error string, because the wording
// differs by platform (Linux: "connection refused"; Windows: "actively
// refused it") — a substring-based classifier can pass on one platform and
// silently misclassify as INVALID on the other.
func TestQualityFromErrorConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // now nothing is listening on addr; dialing it refuses

	_, dialErr := net.DialTimeout("tcp", addr, 2*time.Second)
	if dialErr == nil {
		t.Fatal("expected dial to a closed port to fail")
	}

	got := QualityFromError(dialErr)
	if got != DeviceOffline {
		t.Fatalf("QualityFromError(%v) = %v, want %v", dialErr, got, DeviceOffline)
	}
}

func TestQualityFromErrorNil(t *testing.T) {
	if got := QualityFromError(nil); got != Good {
		t.Fatalf("QualityFromError(nil) = %v, want %v", got, Good)
	}
}

// TestQualityFromErrorTimeout exercises a real network read timeout (§16
// Diagnostics "Timeout", Phase 8) rather than a mocked net.Error: a TCP
// server that accepts the connection but never writes a response is
// exactly what an unresponsive/hung Modbus TCP device looks like on the
// wire, distinct from a refused connection (DeviceOffline).
func TestQualityFromErrorTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		<-t.Context().Done() // hold the connection open without ever responding
	}()

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 8)
	_, readErr := conn.Read(buf)
	if readErr == nil {
		t.Fatal("expected a read timeout, got a response")
	}

	got := QualityFromError(readErr)
	if got != Timeout {
		t.Fatalf("QualityFromError(%v) = %v, want %v", readErr, got, Timeout)
	}
}

// TestQualityFromErrorDNSFailure exercises a real *net.DNSError (found via
// a Phase 8 chaos test: severing a container's network entirely makes
// Docker's embedded DNS resolver fail with "server misbehaving", which
// none of the connection-refused-style substrings matched — it fell
// through to INVALID instead of DEVICE_OFFLINE before this classifier
// gained an explicit DNS check).
func TestQualityFromErrorDNSFailure(t *testing.T) {
	// A name guaranteed not to resolve (RFC 6761 reserved TLD) triggers a
	// real *net.DNSError from the standard resolver — no mock needed.
	_, err := net.LookupHost("this-host-does-not-exist.invalid")
	if err == nil {
		t.Fatal("expected DNS lookup of a .invalid host to fail")
	}

	got := QualityFromError(err)
	if got != DeviceOffline {
		t.Fatalf("QualityFromError(%v) = %v, want %v", err, got, DeviceOffline)
	}
}

// TestQualityFromErrorCRCMismatch exercises goburrow/modbus's real RTU
// frame CRC validation (rtuPackager.Decode, promoted onto
// RTUClientHandler) rather than a hand-rolled "crc" string, since no
// physical RS-485/RTU hardware is available in this dev environment
// (same constraint recorded for Phase 1's RTU work). NewRTUClientHandler
// does no I/O — Decode is pure frame parsing, so this needs no serial
// port to exercise the real corrupted-frame path.
func TestQualityFromErrorCRCMismatch(t *testing.T) {
	handler := goburrow.NewRTUClientHandler("unused")

	// A well-formed-looking response frame (slave=1, fc=3 read holding
	// registers, 2 data bytes) with a deliberately wrong trailing CRC.
	adu := []byte{0x01, 0x03, 0x02, 0x00, 0x64, 0xFF, 0xFF}
	_, decodeErr := handler.Decode(adu)
	if decodeErr == nil {
		t.Fatal("expected a CRC mismatch error from a corrupted frame")
	}

	got := QualityFromError(decodeErr)
	if got != CRCError {
		t.Fatalf("QualityFromError(%v) = %v, want %v", decodeErr, got, CRCError)
	}
}
