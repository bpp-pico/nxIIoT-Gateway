package modbus

import (
	"fmt"
	"math"
)

// DataType is the wire representation of a Data Point's raw register value.
type DataType string

const (
	Int16   DataType = "INT16"
	UInt16  DataType = "UINT16"
	Int32   DataType = "INT32"
	UInt32  DataType = "UINT32"
	Float32 DataType = "FLOAT32"
	Float64 DataType = "FLOAT64"
)

// ByteWidth returns how many raw bytes DataType occupies on the wire.
func (d DataType) ByteWidth() (int, error) {
	switch d {
	case Int16, UInt16:
		return 2, nil
	case Int32, UInt32, Float32:
		return 4, nil
	case Float64:
		return 8, nil
	default:
		return 0, fmt.Errorf("modbus: unknown data type %q", d)
	}
}

// RegisterCount returns how many 16-bit Modbus registers DataType spans.
func (d DataType) RegisterCount() (uint16, error) {
	width, err := d.ByteWidth()
	if err != nil {
		return 0, err
	}
	return uint16(width / 2), nil
}

// reorder rearranges raw bytes according to a byte-order permutation string
// such as "AB", "BA", "ABCD", "BADC", "CDAB", "DCBA". Each letter names the
// natural (big-endian) position of the byte that belongs at that slot, so
// "BADC" means: output[0]=input[1], output[1]=input[0], output[2]=input[3],
// output[3]=input[2]. An empty order means "natural order" (no reordering).
func reorder(raw []byte, order string) ([]byte, error) {
	if order == "" {
		return raw, nil
	}
	if len(order) != len(raw) {
		return nil, fmt.Errorf("modbus: byte order %q does not match value width %d", order, len(raw))
	}

	out := make([]byte, len(raw))
	for i, c := range order {
		pos := int(c - 'A')
		if pos < 0 || pos >= len(raw) {
			return nil, fmt.Errorf("modbus: invalid byte order character %q in %q", c, order)
		}
		out[i] = raw[pos]
	}
	return out, nil
}

// Decode interprets raw register bytes as dataType, reordering bytes per
// byteOrder (e.g. "ABCD" for a natural-order 32-bit value, "BADC"/"CDAB" for
// common word-swapped variants), and returns the value as float64.
func Decode(raw []byte, dataType DataType, byteOrder string) (float64, error) {
	width, err := dataType.ByteWidth()
	if err != nil {
		return 0, err
	}
	if len(raw) != width {
		return 0, fmt.Errorf("modbus: expected %d raw bytes for %s, got %d", width, dataType, len(raw))
	}

	ordered, err := reorder(raw, byteOrder)
	if err != nil {
		return 0, err
	}

	be := func(b []byte) uint64 {
		var v uint64
		for _, x := range b {
			v = v<<8 | uint64(x)
		}
		return v
	}

	switch dataType {
	case Int16:
		return float64(int16(be(ordered))), nil
	case UInt16:
		return float64(uint16(be(ordered))), nil
	case Int32:
		return float64(int32(be(ordered))), nil
	case UInt32:
		return float64(uint32(be(ordered))), nil
	case Float32:
		return float64(math.Float32frombits(uint32(be(ordered)))), nil
	case Float64:
		return math.Float64frombits(be(ordered)), nil
	default:
		return 0, fmt.Errorf("modbus: unknown data type %q", dataType)
	}
}

// ApplyScale converts a raw decoded value into engineering units.
func ApplyScale(raw, scale, offset float64) float64 {
	return raw*scale + offset
}
