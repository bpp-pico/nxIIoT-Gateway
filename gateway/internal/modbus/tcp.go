package modbus

import (
	"context"
	"fmt"
	"time"

	goburrow "github.com/goburrow/modbus"
)

// TCPConfig configures a Modbus TCP client connection (FR-002).
type TCPConfig struct {
	Address string // host:port, e.g. "10.0.0.5:502"
	UnitID  byte
	Timeout time.Duration
}

type tcpClient struct {
	handler *goburrow.TCPClientHandler
	client  goburrow.Client
}

// NewTCPClient creates a Modbus TCP master client. Connect must be called
// before the first Read.
func NewTCPClient(cfg TCPConfig) Client {
	handler := goburrow.NewTCPClientHandler(cfg.Address)
	handler.SlaveId = cfg.UnitID
	handler.Timeout = cfg.Timeout

	return &tcpClient{
		handler: handler,
		client:  goburrow.NewClient(handler),
	}
}

func (t *tcpClient) Connect() error {
	return t.handler.Connect()
}

func (t *tcpClient) Close() error {
	return t.handler.Close()
}

func (t *tcpClient) Read(ctx context.Context, fc FunctionCode, address, quantity uint16) ([]byte, error) {
	return readFunc(t.client, fc, address, quantity)
}

func readFunc(c goburrow.Client, fc FunctionCode, address, quantity uint16) ([]byte, error) {
	switch fc {
	case FuncReadCoils:
		return c.ReadCoils(address, quantity)
	case FuncReadDiscreteInputs:
		return c.ReadDiscreteInputs(address, quantity)
	case FuncReadHoldingRegisters:
		return c.ReadHoldingRegisters(address, quantity)
	case FuncReadInputRegisters:
		return c.ReadInputRegisters(address, quantity)
	default:
		return nil, fmt.Errorf("modbus: unsupported function code %d", fc)
	}
}
