package unipi

import (
	"testing"

	"periph.io/x/conn/v3/i2c/i2ctest"
)

// NewProbes must open the real DS2482 driver without panicking: an empty
// playback bus refuses the reset, which has to come back as an error.
func TestNewProbesOpensDriver(t *testing.T) {
	p := NewProbes(&i2ctest.Playback{DontPanic: true})
	if _, err := p.Read(); err == nil {
		t.Fatal("Read on a bus with no DS2482: want an error")
	}
}
