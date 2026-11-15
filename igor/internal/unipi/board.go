package unipi

import (
	"fmt"
	"log/slog"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"
)

// Board is a real UniPi 1.1 on I2C bus 1 and the Pi's GPIO header.
type Board struct {
	*Relays
	adc    *ADC
	out    *AnalogOut
	active bool
	probes *Probes
	inputs Inputs
}

// Open needs /dev/i2c-1 and /dev/gpiochip0. With active false (observe
// mode) it changes nothing on the board, not even the input pull-ups.
func Open(log *slog.Logger, active bool) (*Board, error) {
	if _, err := host.Init(); err != nil {
		return nil, fmt.Errorf("periph init: %w", err)
	}
	bus, err := i2creg.Open("1")
	if err != nil {
		return nil, fmt.Errorf("open I2C bus 1: %w", err)
	}
	b := &Board{Relays: NewRelays(bus), adc: NewADC(bus), out: NewAnalogOut(), probes: NewProbes(bus), active: active}
	pull := gpio.PullNoChange
	if active {
		pull = gpio.PullUp // as Evok sets it; the optocoupler pulls low
	}
	for i, num := range InputPins {
		line, err := inputLine(num)
		if err == nil {
			err = line.In(pull, gpio.NoEdge)
		}
		if err != nil {
			log.Warn("digital input unavailable", "di", i+1, "gpio", num, "err", err)
			continue
		}
		b.inputs.pins[i] = line
	}
	return b, nil
}

// ReadRelays, when igor owns the chip, first makes sure the pins are still
// outputs: a brown-out resets the MCP23008 to inputs with every latch off.
func (b *Board) ReadRelays() ([NumRelays]bool, error) {
	if b.active {
		if err := b.MakeOutputs(); err != nil {
			return [NumRelays]bool{}, err
		}
	}
	return b.Relays.Read()
}
func (b *Board) SetRelay(n int, on bool) error                    { return b.Relays.Set(n, on) }
func (b *Board) ReadInputs() [NumInputs]Input                     { return b.inputs.Read() }
func (b *Board) ReadAnalog(ch int, corr float64) (float64, error) { return b.adc.Read(ch, corr) }
func (b *Board) ReadTemps() (map[string]float64, error)           { return b.probes.Read() }
func (b *Board) ReadAnalogOut() (float64, error)                  { return b.out.Read() }
func (b *Board) SetAnalogOut(volts float64) error                 { return b.out.Set(volts) }
