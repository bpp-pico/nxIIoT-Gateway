// Package device provides the Device configuration model and repository.
// A Device is a slave ID and its data points on a shared connection.Connection
// (see internal/connection) — the physical link (protocol, interface/baud/
// timeout/etc) lives on Connection, not here, specifically so several
// devices can share one physical bus (real Modbus RTU multi-drop) funneled
// through one client instead of each opening its own competing connection.
package device

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("device not found")

type Device struct {
	ID           int64
	Name         string
	ConnectionID int64
	SlaveID      int
	Enabled      bool
}

// Validate checks the fields that are still device-level after the
// connection split — connection-level checks (protocol/interface/baud/etc)
// now live in connection.Connection.Validate.
func (d Device) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	if d.ConnectionID <= 0 {
		return fmt.Errorf("connection_id is required")
	}
	if d.SlaveID < 1 || d.SlaveID > 247 {
		return fmt.Errorf("slave_id must be between 1 and 247")
	}
	return nil
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

const selectColumns = `id, name, connection_id, slave_id, enabled`

func scanDevice(row interface{ Scan(...any) error }) (Device, error) {
	var d Device
	var slaveID sql.NullInt64
	var enabled int
	err := row.Scan(&d.ID, &d.Name, &d.ConnectionID, &slaveID, &enabled)
	if err != nil {
		return Device{}, err
	}
	d.SlaveID = int(slaveID.Int64)
	d.Enabled = enabled != 0
	return d, nil
}

// List returns every device, enabled or not.
func (r *Repository) List(ctx context.Context) ([]Device, error) {
	return r.query(ctx, `SELECT `+selectColumns+` FROM device ORDER BY id`)
}

// ListEnabled returns all devices with enabled = 1, regardless of whether
// their connection is also enabled — callers that care about the
// connection's own enabled flag too (acquisition.Manager) join against
// connection themselves; this method mirrors the pre-split behavior for
// callers (like the dashboard's device count) that only care about the
// device's own flag.
func (r *Repository) ListEnabled(ctx context.Context) ([]Device, error) {
	return r.query(ctx, `SELECT `+selectColumns+` FROM device WHERE enabled = 1 ORDER BY id`)
}

func (r *Repository) query(ctx context.Context, query string, args ...any) ([]Device, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// Count returns the total number of devices, and how many are enabled —
// for the Dashboard "Device Count" widget (§16).
func (r *Repository) Count(ctx context.Context) (total, enabled int64, err error) {
	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE enabled = 1) FROM device`,
	).Scan(&total, &enabled)
	return total, enabled, err
}

// Get returns a single device by id, or ErrNotFound.
func (r *Repository) Get(ctx context.Context, id int64) (Device, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+selectColumns+` FROM device WHERE id = ?`, id)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, err
	}
	return d, nil
}

// Create inserts a new device and returns its id.
func (r *Repository) Create(ctx context.Context, d Device) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO device (name, connection_id, slave_id, enabled)
		VALUES (?, ?, ?, ?)`,
		d.Name, d.ConnectionID, d.SlaveID, boolToInt(d.Enabled))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update replaces all editable fields of an existing device.
func (r *Repository) Update(ctx context.Context, id int64, d Device) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE device SET
			name = ?, connection_id = ?, slave_id = ?, enabled = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ?`,
		d.Name, d.ConnectionID, d.SlaveID, boolToInt(d.Enabled), id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res)
}

// Delete removes a device (and its data points, via ON DELETE CASCADE).
func (r *Repository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM device WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res)
}

func checkRowsAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
