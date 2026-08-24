package modbus

import (
	"math"
	"testing"
)

func TestDecodeInt16(t *testing.T) {
	// 2302 => Voltage_L1 raw value from the design doc example (scale 0.1 => 230.2V)
	raw := []byte{0x08, 0xFE} // 2302
	v, err := Decode(raw, Int16, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 2302 {
		t.Fatalf("got %v, want 2302", v)
	}
}

func TestDecodeInt16Negative(t *testing.T) {
	raw := []byte{0xFF, 0xFF} // -1
	v, err := Decode(raw, Int16, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != -1 {
		t.Fatalf("got %v, want -1", v)
	}
}

func TestDecodeUInt16(t *testing.T) {
	raw := []byte{0xFF, 0xFF}
	v, err := Decode(raw, UInt16, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 65535 {
		t.Fatalf("got %v, want 65535", v)
	}
}

func TestDecodeFloat32ABCD(t *testing.T) {
	bits := math.Float32bits(1500.5)
	raw := []byte{byte(bits >> 24), byte(bits >> 16), byte(bits >> 8), byte(bits)}

	v, err := Decode(raw, Float32, "ABCD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 1500.5 {
		t.Fatalf("got %v, want 1500.5", v)
	}
}

func TestDecodeFloat32ByteOrderVariants(t *testing.T) {
	// Natural big-endian bytes for 1500.5: A B C D
	bits := math.Float32bits(1500.5)
	a := byte(bits >> 24)
	b := byte(bits >> 16)
	c := byte(bits >> 8)
	d := byte(bits)

	cases := []struct {
		name  string
		order string
		raw   []byte
	}{
		{"ABCD", "ABCD", []byte{a, b, c, d}},
		{"BADC", "BADC", []byte{b, a, d, c}},
		{"CDAB", "CDAB", []byte{c, d, a, b}},
		{"DCBA", "DCBA", []byte{d, c, b, a}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := Decode(tc.raw, Float32, tc.order)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v != 1500.5 {
				t.Fatalf("got %v, want 1500.5", v)
			}
		})
	}
}

func TestDecodeWrongWidth(t *testing.T) {
	_, err := Decode([]byte{0x00}, Int16, "")
	if err == nil {
		t.Fatal("expected error for wrong byte width")
	}
}

func TestApplyScale(t *testing.T) {
	// Voltage_L1 example: raw=2302, scale=0.1, offset=0 => 230.2
	got := ApplyScale(2302, 0.1, 0)
	if math.Abs(got-230.2) > 0.0001 {
		t.Fatalf("got %v, want 230.2", got)
	}
}

func TestDataTypeRegisterCount(t *testing.T) {
	cases := map[DataType]uint16{
		Int16:   1,
		UInt16:  1,
		Int32:   2,
		UInt32:  2,
		Float32: 2,
		Float64: 4,
	}
	for dt, want := range cases {
		got, err := dt.RegisterCount()
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", dt, err)
		}
		if got != want {
			t.Fatalf("%s: got %d registers, want %d", dt, got, want)
		}
	}
}
