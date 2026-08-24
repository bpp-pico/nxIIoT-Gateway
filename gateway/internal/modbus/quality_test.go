package modbus

import (
	"net"
	"testing"
	"time"
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
