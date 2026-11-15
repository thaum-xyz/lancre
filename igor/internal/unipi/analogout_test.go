package unipi

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakePWM lays out a pwmchip directory the way sysfs does after an export.
func fakePWM(t *testing.T, exported bool) *AnalogOut {
	t.Helper()
	chip := filepath.Join(t.TempDir(), "pwmchip0")
	if err := os.MkdirAll(chip, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(chip, "export"), nil, 0o644)
	if exported {
		os.Mkdir(filepath.Join(chip, "pwm0"), 0o755)
		for _, f := range []string{"period", "duty_cycle", "enable"} {
			os.WriteFile(filepath.Join(chip, "pwm0", f), []byte("0\n"), 0o644)
		}
	}
	return &AnalogOut{chip: chip, wait: 50 * time.Millisecond}
}

func TestAnalogOutSetAndReadBack(t *testing.T) {
	a := fakePWM(t, true)
	if v, err := a.Read(); err != nil || v != 0 {
		t.Fatalf("fresh channel: %v, %v", v, err)
	}
	for _, c := range []struct{ set, want float64 }{{3.3, 3.3}, {12, 10}, {-1, 0}, {10, 10}} {
		if err := a.Set(c.set); err != nil {
			t.Fatal(err)
		}
		got, err := a.Read()
		if err != nil || math.Abs(got-c.want) > 1e-6 {
			t.Fatalf("set %v: read %v, %v", c.set, got, err)
		}
	}
	period, _ := a.get("period")
	duty, _ := a.get("duty_cycle")
	if period != 10_000_000 || duty != 10_000_000 {
		t.Fatalf("period=%d duty=%d", period, duty)
	}
}

func TestAnalogOutReadWritesNothing(t *testing.T) {
	a := fakePWM(t, false)
	if v, err := a.Read(); err != nil || v != 0 {
		t.Fatalf("unexported channel: %v, %v", v, err)
	}
	if b, _ := os.ReadFile(filepath.Join(a.chip, "export")); len(b) != 0 {
		t.Fatal("Read exported the channel")
	}
}

func TestAnalogOutWithoutOverlay(t *testing.T) {
	a := &AnalogOut{chip: filepath.Join(t.TempDir(), "missing")}
	if _, err := a.Read(); err == nil {
		t.Fatal("want an error when the PWM chip is missing")
	}
}

func TestAnalogOutExportThatNeverAppears(t *testing.T) {
	a := fakePWM(t, false)
	if err := a.Set(5); err == nil {
		t.Fatal("want an error when the channel does not appear")
	}
	if b, _ := os.ReadFile(filepath.Join(a.chip, "export")); string(b) != "0" {
		t.Fatalf("export got %q", b)
	}
}
