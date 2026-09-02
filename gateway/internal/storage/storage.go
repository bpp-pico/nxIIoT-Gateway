package storage

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

// Open opens the SQLite database at path (creating parent directories and
// the file if needed), configures WAL mode, and applies migrations found in
// migrationsDir.
func Open(path string, migrationsDir string, log *slog.Logger) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	// _txlock=immediate: every explicit BeginTx in this codebase (queue.Insert,
	// FetchBatch, MarkFailed) reads rows and then writes to them in the same
	// transaction. With more than one open connection (see SetMaxOpenConns
	// below), SQLite's default "deferred" transaction only takes a read lock
	// up front and tries to upgrade to a write lock later — if another
	// connection commits a write in between, that upgrade fails with
	// SQLITE_BUSY_SNAPSHOT (517), which busy_timeout does NOT retry (the
	// read snapshot is genuinely stale, not just contended). "immediate"
	// takes the write lock at BEGIN, before any read, so no other writer can
	// land in the middle — this was caught live, in production, immediately
	// after raising the pool below (see MEMORY.md's 2026-09-02 entries).
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_txlock=immediate", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// WAL mode lets any number of readers run concurrently with the single
	// writer SQLite itself enforces — capping the pool at 1 connection (the
	// previous setting) threw that away and serialized every read (Web UI
	// API calls, forwarder fetches, retention/storage sweeps) behind every
	// write too, including the time-critical acquisition insert. A small
	// pool restores real read/write concurrency; busy_timeout(5000) above
	// still makes the rare concurrent-writer collision wait-and-retry
	// instead of surfacing SQLITE_BUSY. See MEMORY.md's 2026-09-02
	// SD-card I/O contention entry.
	db.SetMaxOpenConns(4)

	if err := migrate(db, migrationsDir, log); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func migrate(db *sql.DB, migrationsDir string, log *slog.Logger) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		var applied int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		sqlBytes, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		// PRAGMA foreign_keys is a no-op inside a transaction, so it must
		// be toggled here, before tx.Begin() — this is SQLite's own
		// documented procedure for a table rebuild (CREATE new shape,
		// copy, DROP old, RENAME), which a migration may need to do to
		// change a column a CHECK constraint depends on (DROP COLUMN
		// alone can't). Without this, DROP TABLE on a table that's an FK
		// parent triggers any ON DELETE CASCADE on its children as if
		// every row were individually deleted — confirmed the hard way:
		// 0004_connection_split.sql's DROP TABLE device silently wiped
		// every datapoint row (device is datapoint's FK parent) until
		// this fix, caught only because the migration was tested against
		// seeded data, not an empty schema. See MEMORY.md.
		if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
			return fmt.Errorf("disable foreign_keys for migration %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}

		if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
			return fmt.Errorf("re-enable foreign_keys after migration %s: %w", name, err)
		}
		// Belt-and-suspenders: relaxing FK enforcement during the
		// migration could in principle let a bad migration commit a
		// dangling reference. Verify none exists before trusting the
		// result, same check the test suite runs.
		if violation, err := hasForeignKeyViolation(db); err != nil {
			return fmt.Errorf("check foreign keys after migration %s: %w", name, err)
		} else if violation {
			return fmt.Errorf("migration %s introduced a foreign key violation", name)
		}

		log.Info("applied migration", "file", name)
	}

	return nil
}

func hasForeignKeyViolation(db *sql.DB) (bool, error) {
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}
