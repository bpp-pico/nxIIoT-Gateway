package modbus

import (
	"context"
	"time"

	goburrow "github.com/goburrow/modbus"
)

// RTUConfig configures a Modbus RTU (RS-485) client connection (FR-001).
type RTUConfig struct {
	Interface string // serial device path, e.g. "/dev/ttyUSB0" or "COM3"
	BaudRate  int
	DataBits  int
	Parity    string // "N", "E", or "O"
	StopBits  int
	SlaveID   byte
	Timeout   time.Duration
}

type rtuClient struct {
	handler *goburrow.RTUClientHandler
	client  goburrow.Client
}

// NewRTUClient creates a Modbus RTU master client. Connect must be called
// before the first Read.
func NewRTUClient(cfg RTUConfig) Client {
	handler := goburrow.NewRTUClientHandler(cfg.Interface)
	handler.BaudRate = cfg.BaudRate
	handler.DataBits = cfg.DataBits
	handler.Parity = cfg.Parity
	handler.StopBits = cfg.StopBits
	handler.SlaveId = cfg.SlaveID
	handler.Timeout = cfg.Timeout

	return &rtuClient{
		handler: handler,
		client:  goburrow.NewClient(handler),
	}
}

func (t *rtuClient) Connect() error {
	return t.handler.Connect()
}

func (t *rtuClient) Close() error {
	return t.handler.Close()
}

func (t *rtuClient) Read(ctx context.Context, fc FunctionCode, address, quantity uint16) ([]byte, error) {
	return readFunc(t.client, fc, address, quantity)
}
