package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nxiiot-gateway/internal/acquisition"
	"nxiiot-gateway/internal/api"
	"nxiiot-gateway/internal/config"
	"nxiiot-gateway/internal/datapoint"
	"nxiiot-gateway/internal/device"
	"nxiiot-gateway/internal/forwarder"
	"nxiiot-gateway/internal/logger"
	"nxiiot-gateway/internal/processor"
	"nxiiot-gateway/internal/queue"
	"nxiiot-gateway/internal/status"
	"nxiiot-gateway/internal/storage"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	migrationsDir := flag.String("migrations", "migrations", "path to migrations directory")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(err)
	}

	log := logger.New(cfg.Log.Level, cfg.Log.Format)
	log.Info("starting nxIIoT Gateway", "gateway_id", cfg.Gateway.ID)

	db, err := storage.Open(cfg.Database.Path, *migrationsDir, log)
	if err != nil {
		log.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if os.Getenv("SEED_DEMO_DEVICE") == "true" {
		seedDemoDevice(db, log)
	}

	// docker-compose reaches the fake server via its service DNS name
	// ("server-sim"), whereas a native run reaches it via localhost — the
	// config file default is the native/localhost case.
	if url := os.Getenv("FORWARDER_SERVER_URL"); url != "" {
		cfg.Forwarder.ServerURL = url
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// SQLite is the gateway's source of truth (Rule 3); data must be
	// persisted before being forwarded (Rule 2). queueRepo owns the
	// data_queue table and the gateway's persistent sequence counter
	// (Rule 6). EnsureGateway must run before anything assigns a sequence
	// ID, since the counter lives on the gateway's own row.
	queueRepo := queue.NewRepository(db)
	if err := queueRepo.EnsureGateway(ctx, cfg.Gateway.ID, cfg.Gateway.Name); err != nil {
		log.Error("failed to initialize gateway row", "error", err)
		os.Exit(1)
	}
	proc := processor.New(queueRepo, cfg.Gateway.ID, log)

	go queue.RunRetentionSweeper(ctx, queueRepo,
		time.Duration(cfg.Queue.RetentionDays)*24*time.Hour,
		time.Duration(cfg.Queue.SweepInterval)*time.Minute,
		log)

	go queue.RunStoragePressureSweeper(ctx, queueRepo,
		func() (float64, error) { return storage.DiskUsagePercent(cfg.Database.Path) },
		cfg.Queue.StorageFullPercent,
		cfg.Queue.EvictBatchSize,
		time.Duration(cfg.Queue.StorageSweepInterval)*time.Second,
		log)

	// Acquisition must never depend on the API server or any downstream
	// server connectivity (Rule 1). The Manager owns one poller goroutine
	// per enabled device and is reloaded by the API layer whenever a
	// device/data point is created, edited, deleted, or toggled, so the
	// Web UI takes effect without a gateway restart.
	statusStore := status.NewStore()
	deviceRepo := device.NewRepository(db)
	datapointRepo := datapoint.NewRepository(db)
	manager := acquisition.NewManager(ctx, log, deviceRepo, datapointRepo, func(r acquisition.Reading) {
		statusStore.Update(r.DeviceID, string(r.Quality), r.EventTimestamp)
		proc.Process(ctx, r)
		if r.Value != nil {
			log.Info("reading", "device", r.DeviceName, "tag", r.Tag, "value", *r.Value, "unit", r.Unit, "quality", r.Quality)
		} else {
			log.Warn("reading", "device", r.DeviceName, "tag", r.Tag, "quality", r.Quality)
		}
	})
	if err := manager.Reload(ctx); err != nil {
		log.Error("failed to start acquisition", "error", err)
	}

	// Store & Forward runs independently of acquisition (Rule 1): a down
	// server only grows the PENDING backlog, it never blocks Modbus polling.
	adapter := forwarder.NewHTTPAdapter(cfg.Forwarder.ServerURL, time.Duration(cfg.Forwarder.SendTimeoutMs)*time.Millisecond)
	fwd := forwarder.New(queueRepo, adapter, forwarder.Config{
		BatchSize:    cfg.Forwarder.BatchSize,
		PollInterval: time.Duration(cfg.Forwarder.PollIntervalMs) * time.Millisecond,
	}, log)
	go fwd.Run(ctx)

	handler := api.NewRouter(cfg, db, log, statusStore, manager, queueRepo, fwd)
	srv := &http.Server{
		Addr:    cfg.API.ListenAddr,
		Handler: handler,
	}

	go func() {
		log.Info("api server listening", "addr", cfg.API.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("api server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", "error", err)
	}
	manager.Wait()
}
