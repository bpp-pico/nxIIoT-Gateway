// Package device provides the Device configuration model and repository.
package device

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type Protocol string

const (
	RTU Protocol = "RTU"
	TCP Protocol = "TCP"
)

var ErrNotFound = errors.New("device not found")

type Device struct {
	ID                int64
	Name              string
	Protocol          Protocol
	Interface         string
	IPAddress         string
	SlaveID           int
	Port              int
	PollingIntervalMs int
	TimeoutMs         int
	Retry             int
	Enabled           bool

	// RTU-only (FR-001)
	BaudRate int
	DataBits int
	Parity   string
	StopBits int
}

// Validate checks the fields required by FR-001 (RTU) / FR-002 (TCP).
func (d Device) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch d.Protocol {
	case TCP:
		if d.IPAddress == "" {
			return fmt.Errorf("ip_address is required for TCP devices")
		}
		if d.Port <= 0 || d.Port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	case RTU:
		if d.Interface == "" {
			return fmt.Errorf("interface is required for RTU devices")
		}
		if d.BaudRate <= 0 {
			return fmt.Errorf("baud_rate must be positive")
		}
		if d.Parity != "N" && d.Parity != "E" && d.Parity != "O" {
			return fmt.Errorf("parity must be one of N, E, O")
		}
	default:
		return fmt.Errorf("protocol must be RTU or TCP")
	}
	if d.SlaveID < 1 || d.SlaveID > 247 {
		return fmt.Errorf("slave_id must be between 1 and 247")
	}
	if d.PollingIntervalMs <= 0 {
		return fmt.Errorf("polling_interval_ms must be positive")
	}
	if d.TimeoutMs <= 0 {
		return fmt.Errorf("timeout_ms must be positive")
	}
	if d.Retry < 0 {
		return fmt.Errorf("retry must not be negative")
	}
	return nil
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

const selectColumns = `id, name, protocol, interface, ip_address, slave_id, port,
	polling_interval_ms, timeout_ms, retry, enabled,
	baud_rate, data_bits, parity, stop_bits`

func scanDevice(row interface{ Scan(...any) error }) (Device, error) {
	var d Device
	var iface, ip sql.NullString
	var slaveID, port sql.NullInt64
	var enabled int
	err := row.Scan(&d.ID, &d.Name, &d.Protocol, &iface, &ip, &slaveID, &port,
		&d.PollingIntervalMs, &d.TimeoutMs, &d.Retry, &enabled,
		&d.BaudRate, &d.DataBits, &d.Parity, &d.StopBits)
	if err != nil {
		return Device{}, err
	}
	d.Interface = iface.String
	d.IPAddress = ip.String
	d.SlaveID = int(slaveID.Int64)
	d.Port = int(port.Int64)
	d.Enabled = enabled != 0
	return d, nil
}

// List returns every device, enabled or not.
func (r *Repository) List(ctx context.Context) ([]Device, error) {
	return r.query(ctx, `SELECT `+selectColumns+` FROM device ORDER BY id`)
}

// ListEnabled returns all devices with enabled = 1.
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
		INSERT INTO device (name, protocol, interface, ip_address, slave_id, port,
		                     polling_interval_ms, timeout_ms, retry, enabled,
		                     baud_rate, data_bits, parity, stop_bits)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.Name, d.Protocol, nullable(d.Interface), nullable(d.IPAddress), d.SlaveID, d.Port,
		d.PollingIntervalMs, d.TimeoutMs, d.Retry, boolToInt(d.Enabled),
		d.BaudRate, d.DataBits, d.Parity, d.StopBits)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update replaces all editable fields of an existing device.
func (r *Repository) Update(ctx context.Context, id int64, d Device) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE device SET
			name = ?, protocol = ?, interface = ?, ip_address = ?, slave_id = ?, port = ?,
			polling_interval_ms = ?, timeout_ms = ?, retry = ?, enabled = ?,
			baud_rate = ?, data_bits = ?, parity = ?, stop_bits = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ?`,
		d.Name, d.Protocol, nullable(d.Interface), nullable(d.IPAddress), d.SlaveID, d.Port,
		d.PollingIntervalMs, d.TimeoutMs, d.Retry, boolToInt(d.Enabled),
		d.BaudRate, d.DataBits, d.Parity, d.StopBits, id)
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

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
