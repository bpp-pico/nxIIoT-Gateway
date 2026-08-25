// Command internal-server is the production "Internal Server" the
// gateway's MQTTAdapter has always been designed to talk to (§15) — it
// subscribes to gateway/+/data, persists every reading to Postgres keyed
// by gateway_id+sequence_id (idempotent under retry, Rule 6), and
// publishes an application-level ack back so Store & Forward can retire
// the batch. Unlike cmd/server-sim (gateway/cmd/server-sim), which is
// explicitly dev-only and in-memory, this is meant to run continuously.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := loadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := newStore(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect database failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.migrate(ctx); err != nil {
		log.Error("migrate failed", "error", err)
		os.Exit(1)
	}

	c := newConsumer(cfg, st, log)
	if err := c.Connect(ctx); err != nil {
		log.Error("mqtt connect failed", "error", err)
		os.Exit(1)
	}
	defer c.Disconnect()

	srv := newHTTPServer(cfg.HTTPAddr, st, c, log)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server failed", "error", err)
		}
	}()

	log.Info("internal-server started", "http_addr", cfg.HTTPAddr, "broker", cfg.MQTTBrokerURL, "topic_filter", cfg.MQTTDataTopicFilter)
	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
