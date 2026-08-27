// Package connection provides the Connection configuration model and
// repository — the physical link (protocol, interface/ip+port, baud/parity/
// stop-bits, timeout, retry) that one or more Devices (see internal/device)
// share. Splitting this out from Device is what lets real Modbus RTU
// multi-drop work: several slave IDs on one physical bus now reference one
// Connection, funneled through a single client instead of each opening its
// own competing serial handle (see gateway/migrations/0004_connection_split.sql
// and MEMORY.md for the incident this fixes).
package connection

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

var (
	ErrNotFound = errors.New("connection not found")
	// ErrInUse is returned by Delete when one or more devices still
	// reference the connection — deleting it out from under them would
	// silently stop their acquisition, so that must be an explicit choice
	// (repoint or delete the devices first), not a side effect of this call.
	ErrInUse = errors.New("connection is still in use by one or more devices")
)

type Connection struct {
	ID        int64
	Name      string
	Protocol  Protocol
	Interface string
	IPAddress string
	Port      int
	TimeoutMs int
	Retry     int
	Enabled   bool

	// RTU-only
	BaudRate int
	DataBits int
	Parity   string
	StopBits int
}

// Validate checks the fields required by FR-001 (RTU) / FR-002 (TCP) —
// moved here from device.Device.Validate, unchanged, since these are all
// physical-link properties.
func (c Connection) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch c.Protocol {
	case TCP:
		if c.IPAddress == "" {
			return fmt.Errorf("ip_address is required for TCP connections")
		}
		if c.Port <= 0 || c.Port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	case RTU:
		if c.Interface == "" {
			return fmt.Errorf("interface is required for RTU connections")
		}
		if c.BaudRate <= 0 {
			return fmt.Errorf("baud_rate must be positive")
		}
		if c.Parity != "N" && c.Parity != "E" && c.Parity != "O" {
			return fmt.Errorf("parity must be one of N, E, O")
		}
	default:
		return fmt.Errorf("protocol must be RTU or TCP")
	}
	if c.TimeoutMs <= 0 {
		return fmt.Errorf("timeout_ms must be positive")
	}
	if c.Retry < 0 {
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

const selectColumns = `id, name, protocol, interface, ip_address, port,
	timeout_ms, retry, enabled, baud_rate, data_bits, parity, stop_bits`

func scanConnection(row interface{ Scan(...any) error }) (Connection, error) {
	var c Connection
	var iface, ip sql.NullString
	var port sql.NullInt64
	var enabled int
	err := row.Scan(&c.ID, &c.Name, &c.Protocol, &iface, &ip, &port,
		&c.TimeoutMs, &c.Retry, &enabled,
		&c.BaudRate, &c.DataBits, &c.Parity, &c.StopBits)
	if err != nil {
		return Connection{}, err
	}
	c.Interface = iface.String
	c.IPAddress = ip.String
	c.Port = int(port.Int64)
	c.Enabled = enabled != 0
	return c, nil
}

// List returns every connection, enabled or not.
func (r *Repository) List(ctx context.Context) ([]Connection, error) {
	return r.query(ctx, `SELECT `+selectColumns+` FROM connection ORDER BY id`)
}

// ListEnabled returns all connections with enabled = 1.
func (r *Repository) ListEnabled(ctx context.Context) ([]Connection, error) {
	return r.query(ctx, `SELECT `+selectColumns+` FROM connection WHERE enabled = 1 ORDER BY id`)
}

func (r *Repository) query(ctx context.Context, query string, args ...any) ([]Connection, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []Connection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

// Get returns a single connection by id, or ErrNotFound.
func (r *Repository) Get(ctx context.Context, id int64) (Connection, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+selectColumns+` FROM connection WHERE id = ?`, id)
	c, err := scanConnection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Connection{}, ErrNotFound
	}
	if err != nil {
		return Connection{}, err
	}
	return c, nil
}

// Create inserts a new connection and returns its id.
func (r *Repository) Create(ctx context.Context, c Connection) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO connection (name, protocol, interface, ip_address, port,
		                         timeout_ms, retry, enabled,
		                         baud_rate, data_bits, parity, stop_bits)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Protocol, nullable(c.Interface), nullable(c.IPAddress), c.Port,
		c.TimeoutMs, c.Retry, boolToInt(c.Enabled),
		c.BaudRate, c.DataBits, c.Parity, c.StopBits)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Update replaces all editable fields of an existing connection.
func (r *Repository) Update(ctx context.Context, id int64, c Connection) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE connection SET
			name = ?, protocol = ?, interface = ?, ip_address = ?, port = ?,
			timeout_ms = ?, retry = ?, enabled = ?,
			baud_rate = ?, data_bits = ?, parity = ?, stop_bits = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ?`,
		c.Name, c.Protocol, nullable(c.Interface), nullable(c.IPAddress), c.Port,
		c.TimeoutMs, c.Retry, boolToInt(c.Enabled),
		c.BaudRate, c.DataBits, c.Parity, c.StopBits, id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res)
}

// Delete removes a connection, or returns ErrInUse if any device still
// references it (see the ErrInUse doc comment for why this isn't a cascade).
func (r *Repository) Delete(ctx context.Context, id int64) error {
	var inUse int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM device WHERE connection_id = ?`, id).Scan(&inUse); err != nil {
		return err
	}
	if inUse > 0 {
		return ErrInUse
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM connection WHERE id = ?`, id)
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
