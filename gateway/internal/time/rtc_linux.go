//go:build linux

package timeservice

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// hwRTC reads/writes a Linux RTC character device (typically /dev/rtc0)
// via the same RTC_RD_TIME/RTC_SET_TIME ioctls hwclock uses. Opening the
// device fails cleanly with ErrRTCUnavailable when there is no RTC chip
// or the process lacks permission — e.g. every container without
// device passthrough — matching how system.ListSerialPorts degrades to
// an empty list rather than erroring.
type hwRTC struct {
	device string
}

func newRTC(device string) RTC {
	return &hwRTC{device: device}
}

func (r *hwRTC) Read() (time.Time, error) {
	f, err := os.OpenFile(r.device, os.O_RDONLY, 0)
	if err != nil {
		return time.Time{}, ErrRTCUnavailable
	}
	defer f.Close()

	rt, err := unix.IoctlGetRTCTime(int(f.Fd()))
	if err != nil {
		return time.Time{}, fmt.Errorf("read rtc %s: %w", r.device, err)
	}
	return time.Date(int(rt.Year)+1900, time.Month(rt.Mon+1), int(rt.Mday),
		int(rt.Hour), int(rt.Min), int(rt.Sec), 0, time.UTC), nil
}

func (r *hwRTC) Write(t time.Time) error {
	f, err := os.OpenFile(r.device, os.O_RDWR, 0)
	if err != nil {
		return ErrRTCUnavailable
	}
	defer f.Close()

	t = t.UTC()
	rt := &unix.RTCTime{
		Sec:  int32(t.Second()),
		Min:  int32(t.Minute()),
		Hour: int32(t.Hour()),
		Mday: int32(t.Day()),
		Mon:  int32(t.Month() - 1),
		Year: int32(t.Year() - 1900),
	}
	if err := unix.IoctlSetRTCTime(int(f.Fd()), rt); err != nil {
		return fmt.Errorf("write rtc %s: %w", r.device, err)
	}
	return nil
}
