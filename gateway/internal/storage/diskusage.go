package storage

import (
	"path/filepath"

	"github.com/shirou/gopsutil/v3/disk"
)

// DiskUsagePercent returns the used-space percentage (0-100) of the volume
// containing dbPath, for the storage threshold policy (§17). dbPath itself
// need not exist yet — only its parent directory is inspected.
func DiskUsagePercent(dbPath string) (float64, error) {
	dir := filepath.Dir(dbPath)
	usage, err := disk.Usage(dir)
	if err != nil {
		return 0, err
	}
	return usage.UsedPercent, nil
}
