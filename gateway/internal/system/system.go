// Package system reports gateway process/runtime and host resource status
// for the dashboard and health endpoints (§16 "Dashboard": CPU, RAM,
// Storage, Network).
package system

import (
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

var startedAt = time.Now()

type Info struct {
	Status     string  `json:"status"`
	UptimeSec  float64 `json:"uptime_seconds"`
	GoVersion  string  `json:"go_version"`
	Goroutines int     `json:"goroutines"`
	NumCPU     int     `json:"num_cpu"`

	CPUPercent   *float64 `json:"cpu_percent,omitempty"`
	MemUsedPct   *float64 `json:"mem_used_percent,omitempty"`
	MemTotalMB   *float64 `json:"mem_total_mb,omitempty"`
	MemUsedMB    *float64 `json:"mem_used_mb,omitempty"`
	DiskUsedPct  *float64 `json:"disk_used_percent,omitempty"`
	DiskTotalGB  *float64 `json:"disk_total_gb,omitempty"`
	DiskUsedGB   *float64 `json:"disk_used_gb,omitempty"`
	NetBytesSent *uint64  `json:"net_bytes_sent,omitempty"`
	NetBytesRecv *uint64  `json:"net_bytes_recv,omitempty"`
	DBSizeBytes  *int64   `json:"database_size_bytes,omitempty"`
}

// Current samples host resource usage in addition to Go runtime info.
// diskPath is any path on the volume to report usage for (typically the
// database directory). Every gopsutil call degrades independently and
// silently on failure (e.g. no permission, unsupported platform) — a
// dashboard with one blank tile is far better than one that won't load.
func Current(diskPath string) Info {
	info := Info{
		Status:     "ok",
		UptimeSec:  time.Since(startedAt).Seconds(),
		GoVersion:  runtime.Version(),
		Goroutines: runtime.NumGoroutine(),
		NumCPU:     runtime.NumCPU(),
	}

	if pct, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(pct) > 0 {
		info.CPUPercent = &pct[0]
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		usedPct := vm.UsedPercent
		totalMB := float64(vm.Total) / (1024 * 1024)
		usedMB := float64(vm.Used) / (1024 * 1024)
		info.MemUsedPct = &usedPct
		info.MemTotalMB = &totalMB
		info.MemUsedMB = &usedMB
	}

	if diskPath != "" {
		if du, err := disk.Usage(diskPath); err == nil {
			usedPct := du.UsedPercent
			totalGB := float64(du.Total) / (1024 * 1024 * 1024)
			usedGB := float64(du.Used) / (1024 * 1024 * 1024)
			info.DiskUsedPct = &usedPct
			info.DiskTotalGB = &totalGB
			info.DiskUsedGB = &usedGB
		}

		// SQLite in WAL mode (see storage.Open) keeps most of a file's
		// actual on-disk footprint in the main file, plus whatever hasn't
		// been checkpointed yet in -wal (and a small -shm index) — sum all
		// three for a size that matches what `ls -la` on the data
		// directory actually shows, not just the main file alone.
		var dbSize int64
		found := false
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if fi, err := os.Stat(diskPath + suffix); err == nil {
				dbSize += fi.Size()
				found = true
			}
		}
		if found {
			info.DBSizeBytes = &dbSize
		}
	}

	if counters, err := net.IOCounters(false); err == nil && len(counters) > 0 {
		sent := counters[0].BytesSent
		recv := counters[0].BytesRecv
		info.NetBytesSent = &sent
		info.NetBytesRecv = &recv
	}

	return info
}
