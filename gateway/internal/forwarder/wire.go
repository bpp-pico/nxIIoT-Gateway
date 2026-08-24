package forwarder

import (
	"time"

	"nxiiot-gateway/internal/queue"
)

// WireEntry is the JSON representation of a queue.DispatchEntry sent to
// the server, shared by every Adapter. gateway_id + sequence_id is the
// idempotency key (Rule 6/10).
type WireEntry struct {
	GatewayID      string   `json:"gateway_id"`
	SequenceID     int64    `json:"sequence_id"`
	DeviceID       int64    `json:"device_id"`
	DatapointID    int64    `json:"datapoint_id"`
	Value          *float64 `json:"value"`
	Quality        string   `json:"quality"`
	EventTimestamp string   `json:"event_timestamp"`
	Priority       string   `json:"priority"`
}

func toWireEntries(batch []queue.DispatchEntry) []WireEntry {
	wire := make([]WireEntry, len(batch))
	for i, e := range batch {
		wire[i] = WireEntry{
			GatewayID:      e.GatewayID,
			SequenceID:     e.SequenceID,
			DeviceID:       e.DeviceID,
			DatapointID:    e.DatapointID,
			Value:          e.Value,
			Quality:        e.Quality,
			EventTimestamp: e.EventTimestamp.UTC().Format(time.RFC3339Nano),
			Priority:       e.Priority,
		}
	}
	return wire
}
