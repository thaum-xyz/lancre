package unipi

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// pwmChip is the kernel PWM behind GPIO18. It exists only with
	// dtoverlay=pwm,pin=18,func=2 in config.txt.
	pwmChip = "/sys/class/pwm/pwmchip0"

	aoPeriod = 10 * time.Millisecond // 100 Hz, as Evok drives it
	aoMax    = 10.0                  // volts at 100 % duty on a UniPi 1.1
)

// AnalogOut is AO 1: the board turns the PWM duty on GPIO18 into 0-10 V.
// The kernel keeps the PWM running while igor is stopped.
type AnalogOut struct {
	chip string
	wait time.Duration // how long to wait for the channel after exporting it
}

func NewAnalogOut() *AnalogOut { return &AnalogOut{chip: pwmChip, wait: time.Second} }

// Read returns the voltage being driven. It writes nothing; a channel that
// is not exported or not enabled drives 0 V.
func (a *AnalogOut) Read() (float64, error) {
	if _, err := os.Stat(a.chip); err != nil {
		return 0, fmt.Errorf("no PWM chip (is dtoverlay=pwm,pin=18,func=2 set?): %w", err)
	}
	enabled, err := a.get("enable")
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil // not exported yet
	}
	if err != nil {
		return 0, err
	}
	period, err := a.get("period")
	if err != nil {
		return 0, err
	}
	duty, err := a.get("duty_cycle")
	if err != nil {
		return 0, err
	}
	if enabled == 0 || period == 0 {
		return 0, nil
	}
	return aoMax * float64(duty) / float64(period), nil
}

// Set drives volts (clamped to 0-10) on the output.
func (a *AnalogOut) Set(volts float64) error {
	volts = math.Min(math.Max(volts, 0), aoMax)
	if _, err := os.Stat(a.channel("period")); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(filepath.Join(a.chip, "export"), []byte("0"), 0); err != nil {
			return fmt.Errorf("pwm export: %w", err)
		}
		for deadline := time.Now().Add(a.wait); ; time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(a.channel("period")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				return errors.New("pwm channel did not appear after export")
			}
		}
	}
	period, err := a.get("period")
	if err != nil {
		return err
	}
	if period != aoPeriod.Nanoseconds() {
		// The kernel rejects a period shorter than the current duty.
		if err := a.put("duty_cycle", 0); err != nil {
			return err
		}
		if err := a.put("period", aoPeriod.Nanoseconds()); err != nil {
			return err
		}
	}
	if err := a.put("duty_cycle", int64(math.Round(volts/aoMax*float64(aoPeriod.Nanoseconds())))); err != nil {
		return err
	}
	return a.put("enable", 1)
}

func (a *AnalogOut) channel(file string) string { return filepath.Join(a.chip, "pwm0", file) }

func (a *AnalogOut) get(file string) (int64, error) {
	b, err := os.ReadFile(a.channel(file))
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
}

func (a *AnalogOut) put(file string, v int64) error {
	if err := os.WriteFile(a.channel(file), []byte(strconv.FormatInt(v, 10)), 0); err != nil {
		return fmt.Errorf("pwm %s: %w", file, err)
	}
	return nil
}
