package forwarder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"nxiiot-gateway/internal/queue"
)

// HTTPAdapter is a minimal Adapter implementation used until the MQTT
// adapter (Phase 5) exists. §15 requires the transport to sit behind this
// interface specifically so it can be swapped later without touching the
// state machine — this is that seam exercised for real, not just designed.
type HTTPAdapter struct {
	url    string
	client *http.Client
}

func NewHTTPAdapter(url string, timeout time.Duration) *HTTPAdapter {
	return &HTTPAdapter{url: url, client: &http.Client{Timeout: timeout}}
}

// WireEntry is the JSON representation of a queue.DispatchEntry sent to
// the server. gateway_id + sequence_id is the idempotency key (Rule 6/10).
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

func (a *HTTPAdapter) Send(ctx context.Context, batch []queue.DispatchEntry) error {
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

	body, err := json.Marshal(wire)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}
