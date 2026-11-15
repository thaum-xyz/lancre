package unipi

import (
	"periph.io/x/conn/v3/gpio"
)

// Inputs are DI 1..12. A nil pin is one the kernel would not hand out
// (GPIO4 while the w1-gpio overlay is loaded); it reads as unavailable.
type Inputs struct {
	pins [NumInputs]gpio.PinIn
}

// Input is one reading. On means the terminal is driven: the board's
// optocoupler pulls the GPIO low.
type Input struct {
	On        bool
	Available bool
}

func (in *Inputs) Read() [NumInputs]Input {
	var out [NumInputs]Input
	for i, p := range in.pins {
		if p != nil {
			out[i] = Input{On: p.Read() == gpio.Low, Available: true}
		}
	}
	return out
}
