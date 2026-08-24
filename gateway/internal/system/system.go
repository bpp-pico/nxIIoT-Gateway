// Package system reports gateway process/runtime status for the dashboard
// and health endpoints. CPU/RAM/disk collection will be added in Phase 7;
// for now it reports what the Go runtime knows about itself.
package system

import (
	"runtime"
	"time"
)

var startedAt = time.Now()

type Info struct {
	Status     string  `json:"status"`
	UptimeSec  float64 `json:"uptime_seconds"`
	GoVersion  string  `json:"go_version"`
	Goroutines int     `json:"goroutines"`
	NumCPU     int     `json:"num_cpu"`
}

func Current() Info {
	return Info{
		Status:     "ok",
		UptimeSec:  time.Since(startedAt).Seconds(),
		GoVersion:  runtime.Version(),
		Goroutines: runtime.NumGoroutine(),
		NumCPU:     runtime.NumCPU(),
	}
}
