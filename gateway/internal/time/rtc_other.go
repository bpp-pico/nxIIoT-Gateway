//go:build !linux

package timeservice

import "time"

// unsupportedRTC is used on every non-Linux build (Windows dev host,
// darwin, ...) where there is no portable way to talk to a hardware RTC.
// It always reports ErrRTCUnavailable so Service degrades to UNSYNCED
// instead of failing.
type unsupportedRTC struct{}

func newRTC(_ string) RTC { return unsupportedRTC{} }

func (unsupportedRTC) Read() (time.Time, error) { return time.Time{}, ErrRTCUnavailable }
func (unsupportedRTC) Write(time.Time) error    { return ErrRTCUnavailable }
