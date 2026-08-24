// modbus-sim is a minimal Modbus TCP slave used only for local development
// and testing of the acquisition engine (internal/modbus, internal/acquisition).
// It is not part of the gateway binary and is never deployed to production.
//
// Seeded registers mirror the Device Model example in the design doc
// (PM001: Voltage_L1, Active_Power, Current_L1).
package main

import (
	"encoding/binary"
	"flag"
	"io"
	"log"
	"math"
	"net"
)

const (
	fcReadCoils            = 1
	fcReadDiscreteInputs   = 2
	fcReadHoldingRegisters = 3
	fcReadInputRegisters   = 4

	exIllegalFunction  = 1
	exIllegalDataAddr  = 2
	exServerDeviceFail = 4
)

func main() {
	addr := flag.String("addr", ":502", "listen address")
	flag.Parse()

	holding := make([]uint16, 200)
	holding[100] = 2302                  // Voltage_L1, INT16, scale 0.1 -> 230.2 V
	holding[101] = 1523                  // Voltage_L2, INT16, scale 0.1 -> 152.3 V (filler)
	putFloat32ABCD(holding, 102, 1500.5) // Active_Power, FLOAT32 ABCD
	holding[104] = 45                    // Current_L1, UINT16, scale 0.1 -> 4.5 A

	input := make([]uint16, 200)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("modbus-sim listening on %s", *addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handleConn(conn, holding, input)
	}
}

func putFloat32ABCD(regs []uint16, addr int, v float32) {
	bits := math.Float32bits(v)
	regs[addr] = uint16(bits >> 16)
	regs[addr+1] = uint16(bits)
}

func handleConn(conn net.Conn, holding, input []uint16) {
	defer conn.Close()

	header := make([]byte, 7)
	for {
		if _, err := io.ReadFull(conn, header); err != nil {
			if err != io.EOF {
				log.Printf("read header: %v", err)
			}
			return
		}

		transactionID := header[0:2]
		length := binary.BigEndian.Uint16(header[4:6])
		unitID := header[6]

		if length < 1 || length > 253 {
			return
		}
		pdu := make([]byte, length-1)
		if _, err := io.ReadFull(conn, pdu); err != nil {
			log.Printf("read pdu: %v", err)
			return
		}

		resp := handlePDU(pdu, holding, input)

		out := make([]byte, 0, 8+len(resp))
		out = append(out, transactionID...)
		out = append(out, 0x00, 0x00) // protocol ID
		respLen := uint16(len(resp) + 1)
		out = append(out, byte(respLen>>8), byte(respLen))
		out = append(out, unitID)
		out = append(out, resp...)

		if _, err := conn.Write(out); err != nil {
			log.Printf("write response: %v", err)
			return
		}
	}
}

func handlePDU(pdu []byte, holding, input []uint16) []byte {
	if len(pdu) < 1 {
		return []byte{0x80, exIllegalFunction}
	}
	fc := pdu[0]

	switch fc {
	case fcReadHoldingRegisters, fcReadInputRegisters:
		if len(pdu) != 5 {
			return []byte{fc | 0x80, exIllegalDataAddr}
		}
		address := binary.BigEndian.Uint16(pdu[1:3])
		quantity := binary.BigEndian.Uint16(pdu[3:5])

		bank := holding
		if fc == fcReadInputRegisters {
			bank = input
		}
		if int(address)+int(quantity) > len(bank) || quantity == 0 || quantity > 125 {
			return []byte{fc | 0x80, exIllegalDataAddr}
		}

		out := []byte{fc, byte(quantity * 2)}
		for i := uint16(0); i < quantity; i++ {
			v := bank[address+i]
			out = append(out, byte(v>>8), byte(v))
		}
		return out

	case fcReadCoils, fcReadDiscreteInputs:
		if len(pdu) != 5 {
			return []byte{fc | 0x80, exIllegalDataAddr}
		}
		quantity := binary.BigEndian.Uint16(pdu[3:5])
		if quantity == 0 || quantity > 2000 {
			return []byte{fc | 0x80, exIllegalDataAddr}
		}
		byteCount := (quantity + 7) / 8
		out := []byte{fc, byte(byteCount)}
		out = append(out, make([]byte, byteCount)...) // all zero
		return out

	default:
		return []byte{fc | 0x80, exIllegalFunction}
	}
}
