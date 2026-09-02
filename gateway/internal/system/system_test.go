package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentReportsDBSizeSummingMainWalAndShmFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gateway.db")

	if err := os.WriteFile(dbPath, make([]byte, 100), 0o644); err != nil {
		t.Fatalf("write main db file: %v", err)
	}
	if err := os.WriteFile(dbPath+"-wal", make([]byte, 30), 0o644); err != nil {
		t.Fatalf("write -wal file: %v", err)
	}
	if err := os.WriteFile(dbPath+"-shm", make([]byte, 10), 0o644); err != nil {
		t.Fatalf("write -shm file: %v", err)
	}

	info := Current(dbPath)

	if info.DBSizeBytes == nil {
		t.Fatal("expected DBSizeBytes to be set")
	}
	if want := int64(140); *info.DBSizeBytes != want {
		t.Errorf("DBSizeBytes = %d, want %d (100 + 30 + 10)", *info.DBSizeBytes, want)
	}
}

func TestCurrentOmitsDBSizeWhenPathDoesNotExist(t *testing.T) {
	info := Current(filepath.Join(t.TempDir(), "does-not-exist.db"))

	if info.DBSizeBytes != nil {
		t.Errorf("expected DBSizeBytes to be nil for a nonexistent path, got %d", *info.DBSizeBytes)
	}
}

func TestCurrentOmitsDiskAndDBFieldsWhenPathIsEmpty(t *testing.T) {
	info := Current("")

	if info.DBSizeBytes != nil {
		t.Errorf("expected DBSizeBytes to be nil when diskPath is empty, got %d", *info.DBSizeBytes)
	}
	if info.DiskUsedPct != nil {
		t.Error("expected DiskUsedPct to be nil when diskPath is empty")
	}
}
