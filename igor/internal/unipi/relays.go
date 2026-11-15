// Package unipi drives the chips of a UniPi 1.1 board. The wiring is fixed
// by the board, so addresses and pins are constants here, not configuration.
package unipi

import (
	"fmt"

	"periph.io/x/conn/v3/i2c"
)

const (
	NumRelays  = 8
	NumInputs  = 12
	NumAnalogs = 2

	addrRelays = 0x20 // MCP23008
	addrOneWir = 0x18 // DS2482-100
	addrADC    = 0x68 // MCP3422

	regIODIR = 0x00
	regOLAT  = 0x0A
)

// InputPins are the BCM GPIO numbers of DI 1..12.
var InputPins = [NumInputs]int{4, 17, 27, 23, 22, 24, 11, 7, 8, 9, 25, 10}

// Relays is the MCP23008. Relay n (1..8) sits on pin 8-n. Nothing is cached:
// another writer (Evok, during observe) may share the chip.
type Relays struct{ dev i2c.Dev }

func NewRelays(bus i2c.Bus) *Relays { return &Relays{dev: i2c.Dev{Bus: bus, Addr: addrRelays}} }

// MakeOutputs sets all pins to outputs if they are not already. After a
// power cut the chip resets to inputs with every latch off, so this never
// switches a relay on.
func (r *Relays) MakeOutputs() error {
	var dir [1]byte
	if err := r.dev.Tx([]byte{regIODIR}, dir[:]); err != nil {
		return fmt.Errorf("mcp23008 read IODIR: %w", err)
	}
	if dir[0] == 0 {
		return nil
	}
	if err := r.dev.Tx([]byte{regIODIR, 0}, nil); err != nil {
		return fmt.Errorf("mcp23008 write IODIR: %w", err)
	}
	return nil
}

func (r *Relays) Read() ([NumRelays]bool, error) {
	var out [NumRelays]bool
	latch, err := r.latch()
	if err != nil {
		return out, err
	}
	for n := 1; n <= NumRelays; n++ {
		out[n-1] = latch&relayBit(n) != 0
	}
	return out, nil
}

func (r *Relays) Set(n int, on bool) error {
	if n < 1 || n > NumRelays {
		return fmt.Errorf("relay %d out of range", n)
	}
	latch, err := r.latch()
	if err != nil {
		return err
	}
	want := latch &^ relayBit(n)
	if on {
		want |= relayBit(n)
	}
	if want == latch {
		return nil
	}
	if err := r.dev.Tx([]byte{regOLAT, want}, nil); err != nil {
		return fmt.Errorf("mcp23008 write OLAT: %w", err)
	}
	return nil
}

func (r *Relays) latch() (byte, error) {
	var b [1]byte
	if err := r.dev.Tx([]byte{regOLAT}, b[:]); err != nil {
		return 0, fmt.Errorf("mcp23008 read OLAT: %w", err)
	}
	return b[0], nil
}

func relayBit(n int) byte { return 1 << (NumRelays - n) }
