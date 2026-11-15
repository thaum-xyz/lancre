package unipi

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"
	"time"

	"periph.io/x/conn/v3/i2c/i2ctest"
	"periph.io/x/conn/v3/onewire"
	"periph.io/x/conn/v3/onewire/onewiretest"
)

func TestRelayOneIsPinSeven(t *testing.T) {
	bus := &i2ctest.Playback{Ops: []i2ctest.IO{
		{Addr: 0x20, W: []byte{0x0A}, R: []byte{0x08}}, // relay 5 already on
		{Addr: 0x20, W: []byte{0x0A, 0x88}},            // relay 1 on, relay 5 kept
		{Addr: 0x20, W: []byte{0x0A}, R: []byte{0x88}}, // set again: no write
		{Addr: 0x20, W: []byte{0x0A}, R: []byte{0x88}}, // relay 8 on = pin 0
		{Addr: 0x20, W: []byte{0x0A, 0x89}},
		{Addr: 0x20, W: []byte{0x0A}, R: []byte{0x89}}, // relay 5 off
		{Addr: 0x20, W: []byte{0x0A, 0x81}},
		{Addr: 0x20, W: []byte{0x0A}, R: []byte{0x81}}, // Read
	}}
	r := NewRelays(bus)
	for _, step := range []struct {
		n  int
		on bool
	}{{1, true}, {1, true}, {8, true}, {5, false}} {
		if err := r.Set(step.n, step.on); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if want := [NumRelays]bool{0: true, 7: true}; got != want {
		t.Fatalf("relays = %v, want %v", got, want)
	}
	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMakeOutputsOnlyWritesWhenNeeded(t *testing.T) {
	bus := &i2ctest.Playback{Ops: []i2ctest.IO{
		{Addr: 0x20, W: []byte{0x00}, R: []byte{0x00}},
		{Addr: 0x20, W: []byte{0x00}, R: []byte{0xFF}}, // after a power cut
		{Addr: 0x20, W: []byte{0x00, 0x00}},
	}}
	r := NewRelays(bus)
	for i := 0; i < 2; i++ {
		if err := r.MakeOutputs(); err != nil {
			t.Fatal(err)
		}
	}
	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestADCMatchesEvokArithmetic(t *testing.T) {
	const corr = 5.549712657928467 // AI 1 on the live board
	cases := []struct {
		name string
		ch   int
		ops  []i2ctest.IO
		want float64
	}{
		{"ch1 mid-scale", 1, []i2ctest.IO{
			{Addr: 0x68, W: []byte{0x8C}},
			{Addr: 0x68, R: []byte{0x00, 0x80, 0x00, 0x0C}},
		}, 2.048 * 32768 / 131071 * corr},
		{"ch2 not ready, then ready", 2, []i2ctest.IO{
			{Addr: 0x68, W: []byte{0xAC}},
			{Addr: 0x68, R: []byte{0x00, 0x00, 0x00, 0xAC}},
			{Addr: 0x68, R: []byte{0x01, 0xFF, 0xFF, 0x2C}},
		}, 2.048 * corr},
		{"negative clamps to zero", 1, []i2ctest.IO{
			{Addr: 0x68, W: []byte{0x8C}},
			{Addr: 0x68, R: []byte{0xFF, 0xFF, 0xF0, 0x0C}},
		}, 0},
	}
	for _, c := range cases {
		bus := &i2ctest.Playback{Ops: c.ops}
		adc := NewADC(bus)
		adc.sleep = func(time.Duration) {}
		got, err := adc.Read(c.ch, corr)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("%s: got %v, want %v", c.name, got, c.want)
		}
		if err := bus.Close(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
}

func TestADCGivesUpWhenNeverReady(t *testing.T) {
	notReady := i2ctest.IO{Addr: 0x68, R: []byte{0, 0, 0, 0x8C}}
	bus := &i2ctest.Playback{Ops: []i2ctest.IO{{Addr: 0x68, W: []byte{0x8C}}, notReady, notReady, notReady}}
	adc := NewADC(bus)
	adc.sleep = func(time.Duration) {}
	if _, err := adc.Read(1, 1); err == nil {
		t.Fatal("want an error")
	}
}

func address(t *testing.T, evok string) onewire.Address {
	t.Helper()
	b, err := hex.DecodeString(evok)
	if err != nil || len(b) != 8 {
		t.Fatalf("bad address %q", evok)
	}
	return onewire.Address(binary.LittleEndian.Uint64(b))
}

func scratchpad(raw int16, config byte) []byte {
	s := []byte{byte(raw), byte(raw >> 8), 0x4B, 0x46, config, 0xFF, 0x0C, 0x10, 0}
	s[8] = onewire.CalcCRC(s[:8])
	return s
}

func TestAddressStringMatchesEvok(t *testing.T) {
	const tank = "28FF4C30041503A7"
	if got := AddressString(address(t, tank)); got != tank {
		t.Fatalf("got %s", got)
	}
}

func TestProbesReadWithoutWritingEEPROM(t *testing.T) {
	const tank, outside = "28FF4C30041503A7", "287CECBF060000DA"
	a, b := address(t, tank), address(t, outside)
	read := func(addr onewire.Address, spad []byte) onewiretest.IO {
		w := binary.LittleEndian.AppendUint64([]byte{0x55}, uint64(addr))
		return onewiretest.IO{W: append(w, 0xBE), R: spad}
	}
	// known is kept sorted by address, so the read order is fixed.
	first, second := a, b
	if b < a {
		first, second = b, a
	}
	temps := map[onewire.Address][]byte{
		a: scratchpad(940, 0x7F), // 58.75 °C at 12 bit
		b: scratchpad(-8, 0x7F),  // -0.5 °C
	}
	bus := &onewiretest.Playback{
		Devices: []onewire.Address{a, b},
		Ops: []onewiretest.IO{
			{W: []byte{0xF0}},
			{W: []byte{0xF0}}, // one search pass per device
			{W: []byte{0xCC, 0x44}, Pull: onewire.StrongPullup},
			read(first, temps[first]),
			read(second, temps[second]),
		},
	}
	p := newProbes(func() (onewire.BusSearcher, error) { return bus, nil })
	got, err := p.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got[tank] != 58.75 || got[outside] != -0.5 || len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProbeRejectsPowerOnValueAndBadCRC(t *testing.T) {
	a := address(t, "28FF4C30041503A7")
	bad := scratchpad(320, 0x7F)
	bad[8] ^= 0xFF
	for name, spad := range map[string][]byte{"power-on 85": scratchpad(85*16, 0x7F), "crc": bad, "bus held low": make([]byte, 9), "out of range": scratchpad(130*16, 0x7F)} {
		w := binary.LittleEndian.AppendUint64([]byte{0x55}, uint64(a))
		bus := &onewiretest.Playback{Ops: []onewiretest.IO{{W: append(w, 0xBE), R: spad}}}
		p := newProbes(nil)
		p.bus = bus
		if _, err := p.temperature(a); err == nil {
			t.Fatalf("%s: want an error", name)
		}
	}
}

func TestProbeMasksUndefinedBitsBelow12Bit(t *testing.T) {
	a := address(t, "28FF4C30041503A7")
	w := binary.LittleEndian.AppendUint64([]byte{0x55}, uint64(a))
	// 9 bit (config 0x1F): only 0.5 °C steps are defined; raw 327 -> 320.
	bus := &onewiretest.Playback{Ops: []onewiretest.IO{{W: append(w, 0xBE), R: scratchpad(327, 0x1F)}}}
	p := newProbes(nil)
	p.bus = bus
	got, err := p.temperature(a)
	if err != nil || got != 20 {
		t.Fatalf("got %v, %v", got, err)
	}
}
