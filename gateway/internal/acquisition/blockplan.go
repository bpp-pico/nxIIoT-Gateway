package acquisition

import (
	"sort"

	"nxiiot-gateway/internal/datapoint"
	"nxiiot-gateway/internal/modbus"
)

// maxRegistersPerBlock is the Modbus PDU limit for a single FC3/FC4
// request's register count (spec-mandated, not this project's choice).
const maxRegistersPerBlock = 125

// maxGapRegisters is how many unused registers a block read is allowed to
// span across before planBlockReads starts a new block instead of
// extending the current one. A small gap costs a few wasted bytes on the
// wire but saves a whole extra round-trip; a large one wastes more than it
// saves. Chosen to cover realistic small gaps between a device's tags
// (e.g. two FLOAT32 tags 5 registers apart) without merging genuinely
// unrelated, far-apart tags into one request.
const maxGapRegisters = 8

// readBlock is one planned Modbus read covering one or more datapoints
// whose register ranges are contiguous or close enough together (within
// maxGapRegisters) on the same function code.
type readBlock struct {
	functionCode modbus.FunctionCode
	startAddress uint16
	quantity     uint16
	points       []datapoint.DataPoint
}

// planBlockReads groups a device's enabled datapoints into as few Modbus
// block reads as possible, replacing the old one-request-per-datapoint
// model. Datapoints are grouped by function code (FC3/FC4 only — coils/
// discrete inputs, FC1/FC2, are left one-per-block below; see the doc
// comment on that branch for why), sorted by register address, and merged
// into contiguous-or-near-adjacent ranges capped at maxRegistersPerBlock.
//
// Real production motivation: one live device has 4 datapoints that are
// actually just 2 distinct registers read under 4 tag names (temperature@1,
// humidity@2, Temp_02@1, Humidity_02@2, all FC4) — this collapses that
// from 4 sequential round-trips to 1.
func planBlockReads(dps []datapoint.DataPoint) []readBlock {
	byFC := make(map[modbus.FunctionCode][]datapoint.DataPoint)
	for _, dp := range dps {
		byFC[modbus.FunctionCode(dp.FunctionCode)] = append(byFC[modbus.FunctionCode(dp.FunctionCode)], dp)
	}

	var blocks []readBlock
	for fc, group := range byFC {
		if fc == modbus.FuncReadCoils || fc == modbus.FuncReadDiscreteInputs {
			// Batching is scoped to FC3/FC4 only. FC1/FC2 responses are
			// bit-packed (ceil(quantity/8) bytes), but DataType.RegisterCount()
			// (what both the offset math below and the pre-existing decode
			// path use) assumes 16-bit-register-width byte counts — mixing
			// the two would decode garbage. This is a pre-existing gap
			// (coil datapoints are already broken end-to-end today, see
			// MEMORY.md), not something this change should paper over by
			// applying register-width math to bit-width data. Keep today's
			// one-block-per-datapoint behavior for these unchanged.
			for _, dp := range group {
				qty, err := modbus.DataType(dp.DataType).RegisterCount()
				if err != nil {
					continue // surfaced as a decode error at read time, same as today
				}
				blocks = append(blocks, readBlock{
					functionCode: fc,
					startAddress: dp.RegisterAddress,
					quantity:     qty,
					points:       []datapoint.DataPoint{dp},
				})
			}
			continue
		}

		sort.Slice(group, func(i, j int) bool { return group[i].RegisterAddress < group[j].RegisterAddress })

		// pending is built up across iterations and only appended to blocks
		// once it's finalized (either a new range starts, or the group
		// ends) — deliberately not a pointer into blocks itself, since
		// blocks keeps growing via append() and a pointer into it can be
		// silently invalidated by a later reallocation.
		var pending *readBlock
		for _, dp := range group {
			qty, err := modbus.DataType(dp.DataType).RegisterCount()
			if err != nil {
				continue // surfaced as a decode error at read time, same as today
			}
			dpEnd := dp.RegisterAddress + qty // one past the last register this datapoint occupies

			if pending != nil &&
				dp.RegisterAddress <= pending.startAddress+pending.quantity+maxGapRegisters &&
				dpEnd-pending.startAddress <= maxRegistersPerBlock {
				if dpEnd > pending.startAddress+pending.quantity {
					pending.quantity = dpEnd - pending.startAddress
				}
				pending.points = append(pending.points, dp)
				continue
			}

			if pending != nil {
				blocks = append(blocks, *pending)
			}
			pending = &readBlock{
				functionCode: fc,
				startAddress: dp.RegisterAddress,
				quantity:     qty,
				points:       []datapoint.DataPoint{dp},
			}
		}
		if pending != nil {
			blocks = append(blocks, *pending)
		}
	}

	return blocks
}
