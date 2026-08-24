package modbus

import (
	"errors"
	"net"
	"strings"
)

// Quality reflects the confidence in an acquired value, per the design doc.
// A stale value must never be reused and reported as GOOD.
type Quality string

const (
	Good          Quality = "GOOD"
	Timeout       Quality = "TIMEOUT"
	CRCError      Quality = "CRC_ERROR"
	DeviceOffline Quality = "DEVICE_OFFLINE"
	Invalid       Quality = "INVALID"
)

// QualityFromError classifies a read/connect error into a Quality value.
//
// Connection-level failures are matched against error message substrings
// because Go's net package does not reliably surface a common typed error
// (e.g. syscall.ECONNREFUSED) across platforms for the same underlying OS
// condition — confirmed empirically: errors.Is(err, syscall.ECONNREFUSED)
// does not match Windows's actual dial-refused error, even though the two
// platforms describe the identical condition. The wording itself does
// differ by platform too (Linux: "connection refused"; Windows: "actively
// refused it"), so every known phrasing is listed explicitly below rather
// than relying on one platform's wording alone.
func QualityFromError(err error) Quality {
	if err == nil {
		return Good
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return Timeout
	}

	if errors.Is(err, net.ErrClosed) {
		return DeviceOffline
	}

	// A DNS failure (e.g. the resolver itself unreachable — "server
	// misbehaving" — or the name genuinely not found) means the device
	// can't be reached at all, same as a refused/unreachable connection.
	// Found via chaos testing (Phase 8, §23 "Network Disconnected"): with
	// the container's network fully severed, Docker's embedded DNS at
	// 127.0.0.11 fails with "server misbehaving", which the substring
	// checks below don't match and previously fell through to INVALID.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return DeviceOffline
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "crc"):
		return CRCError
	case strings.Contains(msg, "connection refused"), // Linux/macOS
		strings.Contains(msg, "actively refused"), // Windows
		strings.Contains(msg, "no route to host"), // Linux
		strings.Contains(msg, "host unreachable"), // Windows/BSD
		strings.Contains(msg, "network unreachable"),
		strings.Contains(msg, "broken pipe"),                    // Linux/macOS
		strings.Contains(msg, "connection reset"),               // Linux/macOS/Windows
		strings.Contains(msg, "forcibly closed"),                // Windows (WSAECONNRESET wording)
		strings.Contains(msg, "device or resource busy"),        // serial port in use, Linux
		strings.Contains(msg, "no such device"),                 // serial port missing, Linux
		strings.Contains(msg, "cannot find the file specified"): // serial port missing, Windows
		return DeviceOffline
	default:
		return Invalid
	}
}
