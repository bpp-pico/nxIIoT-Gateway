package system

import "go.bug.st/serial"

// ListSerialPorts returns the serial ports available on the gateway host
// (e.g. /dev/ttyUSB0 on Linux, COM3 on Windows), for the RTU device
// "Interface" dropdown in the Web UI. An empty list is common when running
// in a container without device passthrough, or on a host with no RS-485
// adapter attached — callers should fall back to manual entry.
func ListSerialPorts() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}
	if ports == nil {
		ports = []string{}
	}
	return ports, nil
}
