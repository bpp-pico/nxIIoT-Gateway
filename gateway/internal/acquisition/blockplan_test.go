package acquisition

import (
	"testing"

	"nxiiot-gateway/internal/datapoint"
	"nxiiot-gateway/internal/modbus"
)

func dp(id int64, tag string, fc uint8, addr uint16, dataType modbus.DataType) datapoint.DataPoint {
	return datapoint.DataPoint{
		ID: id, TagName: tag, FunctionCode: fc, RegisterAddress: addr,
		DataType: string(dataType), ByteOrder: "AB", Scale: 1, Enabled: true,
	}
}

func TestPlanBlockReadsSingleDatapoint(t *testing.T) {
	blocks := planBlockReads([]datapoint.DataPoint{
		dp(1, "a", uint8(modbus.FuncReadHoldingRegisters), 10, modbus.UInt16),
	})
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d: %+v", len(blocks), blocks)
	}
	if blocks[0].startAddress != 10 || blocks[0].quantity != 1 || len(blocks[0].points) != 1 {
		t.Fatalf("unexpected block: %+v", blocks[0])
	}
}

func TestPlanBlockReadsMergesContiguousSameFunctionCode(t *testing.T) {
	blocks := planBlockReads([]datapoint.DataPoint{
		dp(1, "a", uint8(modbus.FuncReadHoldingRegisters), 10, modbus.UInt16),
		dp(2, "b", uint8(modbus.FuncReadHoldingRegisters), 11, modbus.UInt16),
	})
	if len(blocks) != 1 {
		t.Fatalf("expected the two contiguous datapoints to merge into 1 block, got %d: %+v", len(blocks), blocks)
	}
	if blocks[0].startAddress != 10 || blocks[0].quantity != 2 || len(blocks[0].points) != 2 {
		t.Fatalf("unexpected merged block: %+v", blocks[0])
	}
}

func TestPlanBlockReadsMergesSmallGapUnderThreshold(t *testing.T) {
	// addresses 0-1 (Volt, FLOAT32) and 7-8 (Current, FLOAT32) — a 5
	// register gap (registers 2-6), the real "PM" device shape.
	blocks := planBlockReads([]datapoint.DataPoint{
		dp(1, "Volt", uint8(modbus.FuncReadInputRegisters), 0, modbus.Float32),
		dp(2, "Current", uint8(modbus.FuncReadInputRegisters), 7, modbus.Float32),
	})
	if len(blocks) != 1 {
		t.Fatalf("expected a 5-register gap (under maxGapRegisters=%d) to merge into 1 block, got %d: %+v", maxGapRegisters, len(blocks), blocks)
	}
	if blocks[0].startAddress != 0 || blocks[0].quantity != 9 {
		t.Fatalf("unexpected merged block spanning the gap: %+v", blocks[0])
	}
}

func TestPlanBlockReadsDoesNotMergeGapOverThreshold(t *testing.T) {
	// addresses 7-8 (Current) and 72-73 (Energy) — a 63-register gap, the
	// real "PM" device shape that should NOT merge (too wasteful).
	blocks := planBlockReads([]datapoint.DataPoint{
		dp(1, "Current", uint8(modbus.FuncReadInputRegisters), 7, modbus.Float32),
		dp(2, "Energy", uint8(modbus.FuncReadInputRegisters), 72, modbus.Float32),
	})
	if len(blocks) != 2 {
		t.Fatalf("expected a 63-register gap (over maxGapRegisters=%d) to stay as 2 separate blocks, got %d: %+v", maxGapRegisters, len(blocks), blocks)
	}
}

func TestPlanBlockReadsNeverMergesDifferentFunctionCodes(t *testing.T) {
	blocks := planBlockReads([]datapoint.DataPoint{
		dp(1, "holding", uint8(modbus.FuncReadHoldingRegisters), 10, modbus.UInt16),
		dp(2, "input", uint8(modbus.FuncReadInputRegisters), 10, modbus.UInt16),
	})
	if len(blocks) != 2 {
		t.Fatalf("expected different function codes to never merge, got %d: %+v", len(blocks), blocks)
	}
}

func TestPlanBlockReadsSplitsAtDatapointBoundaryNotMidDatapoint(t *testing.T) {
	// 62 contiguous UINT16 datapoints (registers 0-61, 62 registers) then a
	// FLOAT64 (4 registers) at address 62-65, which would push the running
	// total from 62 to 66 registers -- still under 125, so it SHOULD merge.
	// Add more UINT16s after it to push the total over 125, forcing a
	// split; the split must land before the FLOAT64, never inside it.
	var dps []datapoint.DataPoint
	for i := 0; i < 62; i++ {
		dps = append(dps, dp(int64(i), "u", uint8(modbus.FuncReadHoldingRegisters), uint16(i), modbus.UInt16))
	}
	dps = append(dps, dp(100, "wide", uint8(modbus.FuncReadHoldingRegisters), 62, modbus.Float64)) // regs 62-65
	for i := 0; i < 60; i++ {
		dps = append(dps, dp(int64(200+i), "u2", uint8(modbus.FuncReadHoldingRegisters), uint16(66+i), modbus.UInt16))
	}

	blocks := planBlockReads(dps)

	for _, b := range blocks {
		for _, p := range b.points {
			qty, err := modbus.DataType(p.DataType).RegisterCount()
			if err != nil {
				t.Fatalf("unexpected data type error: %v", err)
			}
			dpStart, dpEnd := p.RegisterAddress, p.RegisterAddress+qty
			blockEnd := b.startAddress + b.quantity
			if dpStart < b.startAddress || dpEnd > blockEnd {
				t.Fatalf("datapoint %s (regs %d-%d) is not fully contained in its block (regs %d-%d) -- a datapoint was bisected", p.TagName, dpStart, dpEnd, b.startAddress, blockEnd)
			}
		}
		if b.quantity > maxRegistersPerBlock {
			t.Fatalf("block exceeds maxRegistersPerBlock=%d: %+v", maxRegistersPerBlock, b)
		}
	}
}

// TestPlanBlockReadsRealProductionShapeMergesToOneRead is the actual live
// shape motivating this redesign: device "Temp-Humidity Sensor" has 4
// datapoints that are really just 2 distinct registers read under 4 tag
// names (temperature@1, humidity@2, Temp_02@1, Humidity_02@2, all FC4) --
// this must collapse to exactly 1 block read covering all 4.
func TestPlanBlockReadsRealProductionShapeMergesToOneRead(t *testing.T) {
	blocks := planBlockReads([]datapoint.DataPoint{
		dp(1, "temperature", uint8(modbus.FuncReadInputRegisters), 1, modbus.UInt16),
		dp(2, "humidity", uint8(modbus.FuncReadInputRegisters), 2, modbus.UInt16),
		dp(3, "Temp_02", uint8(modbus.FuncReadInputRegisters), 1, modbus.UInt16),
		dp(4, "Humidity_02", uint8(modbus.FuncReadInputRegisters), 2, modbus.Int16),
	})
	if len(blocks) != 1 {
		t.Fatalf("expected the real production shape to merge into exactly 1 block, got %d: %+v", len(blocks), blocks)
	}
	if blocks[0].startAddress != 1 || blocks[0].quantity != 2 || len(blocks[0].points) != 4 {
		t.Fatalf("expected Read(fc=4, addr=1, qty=2) covering all 4 datapoints, got %+v", blocks[0])
	}
}

func TestPlanBlockReadsLeavesCoilsAndDiscreteInputsUngrouped(t *testing.T) {
	blocks := planBlockReads([]datapoint.DataPoint{
		dp(1, "coilA", uint8(modbus.FuncReadCoils), 0, modbus.UInt16),
		dp(2, "coilB", uint8(modbus.FuncReadCoils), 1, modbus.UInt16),
		dp(3, "discreteA", uint8(modbus.FuncReadDiscreteInputs), 0, modbus.UInt16),
	})
	if len(blocks) != 3 {
		t.Fatalf("expected FC1/FC2 datapoints to stay one-per-block (unchanged from today), got %d: %+v", len(blocks), blocks)
	}
	for _, b := range blocks {
		if len(b.points) != 1 {
			t.Fatalf("expected each FC1/FC2 block to cover exactly 1 datapoint, got %+v", b)
		}
	}
}
