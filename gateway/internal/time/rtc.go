// Package timeservice implements the Time Service (§11-§14, Phase 6):
// periodic NTP sync against the internal network's NTP server, RTC as a
// fallback time source, and the SYNCED/RTC/UNSYNCED/INVALID quality signal
// the rest of the gateway (dashboard, diagnostics) reads. It never blocks
// or is blocked by acquisition (Rule 1, Rule 10) — an unreachable NTP
// server just degrades the reported quality, it never stops Modbus
// polling.
//
// The package is named timeservice, not time, so it can import the
// standard library's time package under its normal name.
package timeservice

import (
	"errors"
	"time"
)

// ErrRTCUnavailable is returned by every RTC method when no hardware RTC
// is present or accessible — the common case in a container without
// device passthrough, or on any non-Linux dev host. Callers must treat
// this as a normal fallback condition (§11: NTP -> RTC -> Local Clock),
// not an error worth surfacing loudly.
var ErrRTCUnavailable = errors.New("rtc: not available on this host")

// RTC abstracts a hardware real-time clock. Real hardware only exists on
// Linux with an RTC chip attached (e.g. a Raspberry Pi + battery-backed
// RTC HAT) — see rtc_linux.go. rtc_other.go provides the fallback for
// every other platform.
type RTC interface {
	Read() (time.Time, error)
	Write(t time.Time) error
}
