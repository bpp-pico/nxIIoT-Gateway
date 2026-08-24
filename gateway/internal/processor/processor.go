// Package processor sits between acquisition and storage: it takes each
// Reading produced by the Modbus engine (which already carries its
// event timestamp and quality — Design Principle #6) and persists it,
// assigning the gateway's next sequence_id along the way.
package processor

import (
	"context"
	"log/slog"

	"nxiiot-gateway/internal/acquisition"
	"nxiiot-gateway/internal/queue"
)

type Processor struct {
	queueRepo *queue.Repository
	gatewayID string
	log       *slog.Logger
}

func New(queueRepo *queue.Repository, gatewayID string, log *slog.Logger) *Processor {
	return &Processor{queueRepo: queueRepo, gatewayID: gatewayID, log: log}
}

// Process persists one acquired Reading as a PENDING data_queue row. It
// never mutates r.EventTimestamp (Rule 5) and persists readings of every
// quality, not just GOOD — a failed read is itself meaningful history.
func (p *Processor) Process(ctx context.Context, r acquisition.Reading) {
	entry := queue.Entry{
		GatewayID:      p.gatewayID,
		DeviceID:       r.DeviceID,
		DatapointID:    r.DatapointID,
		Value:          r.Value,
		Quality:        string(r.Quality),
		EventTimestamp: r.EventTimestamp,
		Priority:       r.Priority,
	}

	stored, err := p.queueRepo.Insert(ctx, entry)
	if err != nil {
		p.log.Error("failed to persist reading", "device", r.DeviceName, "tag", r.Tag, "error", err)
		return
	}
	p.log.Debug("persisted reading", "device", r.DeviceName, "tag", r.Tag, "sequence_id", stored.SequenceID, "quality", r.Quality)
}
