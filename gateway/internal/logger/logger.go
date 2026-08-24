package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New builds a structured logger. format is "text" or "json"; level is one
// of debug, info, warn, error.
func New(level, format string) *slog.Logger {
	log, _ := NewWithRingBuffer(level, format, 0)
	return log
}

// NewWithRingBuffer is New, plus an in-memory ring buffer of the last
// capacity records for GET /api/logs (§16 "Logs") — logging to
// stdout/docker logs is unaffected, the buffer is purely additive.
// capacity <= 0 disables the buffer (Entries always empty).
func NewWithRingBuffer(level, format string, capacity int) (*slog.Logger, *RingBuffer) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	if capacity <= 0 {
		capacity = 1
	}
	rb := NewRingBufferHandler(handler, capacity)
	return slog.New(rb), rb
}
