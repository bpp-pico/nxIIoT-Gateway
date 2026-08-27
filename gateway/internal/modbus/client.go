// Package modbus provides Modbus RTU/TCP master clients, register decoding,
// and data-quality classification, isolated from acquisition scheduling and
// storage (Design Principle #11: protocol-specific code must be isolated
// from business logic).
package modbus

import "context"

// FunctionCode identifies which Modbus read operation to perform.
type FunctionCode uint8

const (
	FuncReadCoils            FunctionCode = 1
	FuncReadDiscreteInputs   FunctionCode = 2
	FuncReadHoldingRegisters FunctionCode = 3
	FuncReadInputRegisters   FunctionCode = 4
)

// Client is a protocol-agnostic Modbus master connection. Implementations
// (RTU, TCP) manage their own transport connection lifecycle.
type Client interface {
	// Connect establishes the underlying transport connection, if not
	// already connected. It is safe to call repeatedly.
	Connect() error

	// Close releases the underlying transport connection.
	Close() error

	// SetUnitID changes which slave/unit ID subsequent Read calls address,
	// without touching the underlying transport connection. This is what
	// lets several logical devices on one physical connection.Connection
	// (real Modbus RTU multi-drop, or several TCP unit IDs behind one
	// gateway) share a single Client: the acquisition Poller owns one
	// Client per connection and calls SetUnitID before each device's own
	// batch of reads (see internal/acquisition/poller.go's runConnection).
	// Not safe to call concurrently with Read/Connect/Close from another
	// goroutine — callers must serialize access to one Client themselves,
	// which runConnection does by construction (one goroutine per Client).
	SetUnitID(id byte)

	// Read performs a single Modbus read request and returns the raw
	// register/coil bytes as returned on the wire.
	Read(ctx context.Context, fc FunctionCode, address, quantity uint16) ([]byte, error)
}

// ReadWithRetry retries a Read up to maxRetries times (in addition to the
// first attempt) on failure, per FR-001/FR-002 retry configuration.
// attempts is how many requests were actually sent on the wire (always
// >= 1), for diagnostics counters (§16) — it is meaningful even on error.
func ReadWithRetry(ctx context.Context, c Client, fc FunctionCode, address, quantity uint16, maxRetries int) (raw []byte, attempts int, err error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		attempts = attempt + 1
		if attempt > 0 {
			// Reconnect before retrying: a failed read often means the
			// connection itself is dead (closed socket, serial port error).
			_ = c.Close()
			if err := c.Connect(); err != nil {
				lastErr = err
				continue
			}
		}

		raw, err := c.Read(ctx, fc, address, quantity)
		if err == nil {
			return raw, attempts, nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return nil, attempts, ctx.Err()
		default:
		}
	}
	return nil, attempts, lastErr
}
