package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"nxiiot-gateway/internal/acquisition"
	"nxiiot-gateway/internal/config"
	"nxiiot-gateway/internal/datapoint"
	"nxiiot-gateway/internal/device"
	"nxiiot-gateway/internal/forwarder"
	"nxiiot-gateway/internal/queue"
	"nxiiot-gateway/internal/status"
	"nxiiot-gateway/internal/system"
)

type Server struct {
	cfg           *config.Config
	db            *sql.DB
	log           *slog.Logger
	deviceRepo    *device.Repository
	datapointRepo *datapoint.Repository
	status        *status.Store
	manager       *acquisition.Manager
	queueRepo     *queue.Repository
	forwarder     *forwarder.Forwarder
}

func NewRouter(cfg *config.Config, db *sql.DB, log *slog.Logger, statusStore *status.Store, manager *acquisition.Manager, queueRepo *queue.Repository, fwd *forwarder.Forwarder) http.Handler {
	s := &Server{
		cfg:           cfg,
		db:            db,
		log:           log,
		deviceRepo:    device.NewRepository(db),
		datapointRepo: datapoint.NewRepository(db),
		status:        statusStore,
		manager:       manager,
		queueRepo:     queueRepo,
		forwarder:     fwd,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(log))

	r.Route("/api", func(r chi.Router) {
		r.Get("/system", s.getSystem)
		r.Get("/system/serial-ports", s.getSerialPorts)
		r.Get("/time", s.getTime)

		r.Get("/devices", s.listDevices)
		r.Post("/devices", s.createDevice)
		r.Put("/devices/{id}", s.updateDevice)
		r.Delete("/devices/{id}", s.deleteDevice)

		r.Get("/devices/{id}/datapoints", s.listDataPoints)
		r.Post("/devices/{id}/datapoints", s.createDataPoint)
		r.Put("/datapoints/{id}", s.updateDataPoint)
		r.Delete("/datapoints/{id}", s.deleteDataPoint)

		r.Post("/devices/{id}/test", s.testDeviceConnection)
		r.Post("/datapoints/{id}/test", s.testDataPointRead)

		r.Get("/store-forward/status", s.getStoreForwardStatus)
		r.Get("/store-forward/statistics", s.getStoreForwardStatistics)

		r.Get("/logs", s.notImplemented)

		r.Get("/config/export", s.notImplemented)
		r.Post("/config/import", s.notImplemented)
	})

	return r
}

func (s *Server) getSystem(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, system.Current())
}

// getSerialPorts lists serial ports available on the gateway host, for the
// RTU device "Interface" dropdown. An empty list is normal when running
// without a physical RS-485 adapter (e.g. in the dev container).
func (s *Server) getSerialPorts(w http.ResponseWriter, r *http.Request) {
	ports, err := system.ListSerialPorts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"ports": ports})
}

func (s *Server) getTime(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"time_quality": "UNSYNCED",
		"timezone":     s.cfg.Time.Timezone,
	})
}

func (s *Server) notImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "not implemented yet",
	})
}

// reload asks the acquisition Manager to pick up config changes. Errors are
// logged but not surfaced to the caller: the write to the database already
// succeeded, and the next Reload (or gateway restart) will retry.
func (s *Server) reload(r *http.Request) {
	if s.manager == nil {
		return
	}
	if err := s.manager.Reload(r.Context()); err != nil {
		s.log.Error("acquisition reload failed", "error", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Debug("request", "method", r.Method, "path", r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}
}
