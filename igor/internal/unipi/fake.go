package unipi

import (
	"math"
	"sync"
	"time"
)

// Fake is a board with no hardware, for trying igor on a laptop. Relays
// latch in memory; DI 5 pulses for 5 s every minute; AI 1 and two probes
// drift slowly.
type Fake struct {
	mu     sync.Mutex
	relays [NumRelays]bool
	start  time.Time
}

func NewFake() *Fake { return &Fake{start: time.Now()} }

func (f *Fake) MakeOutputs() error { return nil }

func (f *Fake) ReadRelays() ([NumRelays]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.relays, nil
}

func (f *Fake) SetRelay(n int, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.relays[n-1] = on
	return nil
}

func (f *Fake) ReadInputs() [NumInputs]Input {
	var out [NumInputs]Input
	for i := range out {
		out[i].Available = true
	}
	out[4].On = int(time.Since(f.start).Seconds())%60 < 5
	return out
}

func (f *Fake) ReadAnalog(ch int, _ float64) (float64, error) {
	if ch == 2 {
		return 0, nil
	}
	return 5 + 5*f.wave(10*time.Minute), nil
}

func (f *Fake) ReadTemps() (map[string]float64, error) {
	return map[string]float64{
		"28FF4C30041503A7": math.Round((50+8*f.wave(20*time.Minute))*16) / 16,
		"287CECBF060000DA": math.Round((3+2*f.wave(time.Hour))*16) / 16,
	}, nil
}

func (f *Fake) wave(period time.Duration) float64 {
	return math.Sin(2 * math.Pi * float64(time.Since(f.start)) / float64(period))
}
