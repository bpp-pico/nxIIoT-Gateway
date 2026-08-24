package logger

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Entry is one captured log record, for GET /api/logs (§16 "Logs").
type Entry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// ringStorage is shared (by pointer) across every handler WithAttrs/
// WithGroup derives, so a logger built via log.With(...) still writes
// into the same buffer the API reads from.
type ringStorage struct {
	mu       sync.Mutex
	entries  []Entry
	capacity int
}

func (s *ringStorage) add(e Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	if len(s.entries) > s.capacity {
		s.entries = s.entries[len(s.entries)-s.capacity:]
	}
}

func (s *ringStorage) snapshot() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

// RingBuffer is an slog.Handler that tees every record into a fixed-size
// in-memory ring, then delegates to next (the real stdout handler) —
// logging still goes to stdout/docker logs exactly as before, this is
// purely additive so the Web UI can show recent logs without a log file.
type RingBuffer struct {
	next    slog.Handler
	storage *ringStorage
}

func NewRingBufferHandler(next slog.Handler, capacity int) *RingBuffer {
	return &RingBuffer{next: next, storage: &ringStorage{capacity: capacity}}
}

func (h *RingBuffer) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *RingBuffer) Handle(ctx context.Context, r slog.Record) error {
	attrs := make(map[string]any, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.storage.add(Entry{Time: r.Time, Level: r.Level.String(), Message: r.Message, Attrs: attrs})
	return h.next.Handle(ctx, r)
}

func (h *RingBuffer) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &RingBuffer{next: h.next.WithAttrs(attrs), storage: h.storage}
}

func (h *RingBuffer) WithGroup(name string) slog.Handler {
	return &RingBuffer{next: h.next.WithGroup(name), storage: h.storage}
}

// Entries returns a snapshot of the captured log, oldest first.
func (h *RingBuffer) Entries() []Entry {
	return h.storage.snapshot()
}
