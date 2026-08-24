// Package forwarder implements Store & Forward (§9): it reads PENDING rows
// from the persistent queue, sends them in batches through a pluggable
// Adapter, and marks them SENT or schedules a retry. It never blocks or is
// blocked by acquisition (Rule 1) — the two are independent goroutines
// that only share the database.
package forwarder

import (
	"context"

	"nxiiot-gateway/internal/queue"
)

// Adapter delivers a batch to the server and reports success or failure
// for the whole batch. §15: the architecture must isolate the transport
// (MQTT today, HTTPS later) behind this interface so it can be swapped
// without touching the state machine.
type Adapter interface {
	Send(ctx context.Context, batch []queue.DispatchEntry) error
}
