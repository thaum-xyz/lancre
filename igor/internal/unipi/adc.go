package unipi

import (
	"fmt"
	"time"

	"periph.io/x/conn/v3/i2c"
)

// ADC is the MCP3422 behind AI 1 and AI 2: one-shot, 18 bit, gain 1.
type ADC struct {
	dev   i2c.Dev
	sleep func(time.Duration)
}

func NewADC(bus i2c.Bus) *ADC {
	return &ADC{dev: i2c.Dev{Bus: bus, Addr: addrADC}, sleep: time.Sleep}
}

const (
	adcConversion = 270 * time.Millisecond // 3.75 samples/s at 18 bit
	adcRetries    = 3
)

// Read returns the terminal voltage of AI ch (1 or 2). correction is the
// board's input divider ratio. The arithmetic matches Evok 2.x so that both
// report the same value.
func (a *ADC) Read(ch int, correction float64) (float64, error) {
	if ch < 1 || ch > NumAnalogs {
		return 0, fmt.Errorf("analog input %d out of range", ch)
	}
	mode := byte(0x0C | (ch-1)<<5) // 18 bit, one-shot, gain 1
	if err := a.dev.Tx([]byte{0x80 | mode}, nil); err != nil {
		return 0, fmt.Errorf("mcp3422 start: %w", err)
	}
	wait := adcConversion
	for try := 0; try < adcRetries; try++ {
		a.sleep(wait)
		wait = 30 * time.Millisecond
		var b [4]byte
		if err := a.dev.Tx(nil, b[:]); err != nil {
			return 0, fmt.Errorf("mcp3422 read: %w", err)
		}
		if b[3]&0x80 != 0 || b[3]&0x7F != mode {
			continue // not ready, or a result for another channel
		}
		code := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
		if code&0x20000 != 0 {
			return 0, nil // negative: Evok clamps to 0 V
		}
		return 2.048 * float64(code&0x1FFFF) / 131071 * correction, nil
	}
	return 0, fmt.Errorf("mcp3422 channel %d: no result", ch)
}
