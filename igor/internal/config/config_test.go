package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/thaum-xyz/lancre/igor/internal/rules"
)

func TestExampleFileGivesTheLiveRules(t *testing.T) {
	raw, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Rules()
	if err != nil {
		t.Fatal(err)
	}
	sauna := rules.Rule{OffOnSilent: true, MaxOn: 3 * time.Hour, OffOnStart: true}
	want := rules.Config{HeartbeatTimeout: 10 * time.Minute, Grace: 2 * time.Minute}
	want.Rules = [rules.N]rules.Rule{0: {OffOnSilent: true}, 3: {MaxOn: 2 * time.Minute}, 5: sauna, 6: sauna}
	if got != want {
		t.Fatalf("rules =\n%+v\nwant\n%+v", got, want)
	}
	if c.Correction(1) != 5.549713 || c.Correction(2) != 5.551579 {
		t.Fatalf("corrections: %v %v", c.Correction(1), c.Correction(2))
	}
}

func TestEmptyFileIsObserveWithHoldEverywhere(t *testing.T) {
	c, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := c.Rules()
	if r.Active || r.Rules != [rules.N]rules.Rule{} || c.Correction(1) != evokCorrection {
		t.Fatalf("%+v", r)
	}
}

func TestDefaultsApplyToEveryRelayAndCanBeOverridden(t *testing.T) {
	c, err := Parse([]byte("mode: active\nrelays:\n  defaults: {on_silent: \"off\", max_on: 1h}\n  5: {on_silent: hold}\n"))
	if err != nil {
		t.Fatal(err)
	}
	r, _ := c.Rules()
	if !r.Active || !r.Rules[0].OffOnSilent || r.Rules[4].OffOnSilent || r.Rules[4].MaxOn != time.Hour {
		t.Fatalf("%+v", r.Rules)
	}
}

func TestMistakesAreRejected(t *testing.T) {
	for name, y := range map[string]string{
		"unknown key":    "relays:\n  1: {on_silnet: \"off\"}\n",
		"relay 9":        "relays:\n  9: {max_on: 1m}\n",
		"on_silent word": "relays:\n  1: {on_silent: drop}\n",
		"mode":           "mode: live\n",
		"invert":         "digital_inputs: {invert: [13]}\n",
		"negative":       "relays:\n  1: {max_on: -1m}\n",
	} {
		if _, err := Parse([]byte(y)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSensorsDefaultOffInObserve(t *testing.T) {
	for y, want := range map[string]bool{
		"mode: observe\n": false,
		"mode: active\n":  true,
		"mode: observe\nonewire: {enabled: true}\nanalog_inputs: {enabled: true}\n":  true,
		"mode: active\nonewire: {enabled: false}\nanalog_inputs: {enabled: false}\n": false,
	} {
		c, err := Parse([]byte(y))
		if err != nil {
			t.Fatal(err)
		}
		if c.AnalogEnabled() != want || c.OneWireEnabled() != want {
			t.Errorf("%q: analog=%v onewire=%v, want %v", y, c.AnalogEnabled(), c.OneWireEnabled(), want)
		}
	}
}

func TestBareOffMeansOff(t *testing.T) {
	c, err := Parse([]byte("relays:\n  1: {on_silent: off}\n"))
	if err != nil {
		if !strings.Contains(err.Error(), "on_silent") {
			t.Fatal(err)
		}
		return // rejected loudly is acceptable; silently "hold" is not
	}
	if r, _ := c.Rules(); !r.Rules[0].OffOnSilent {
		t.Fatal("bare off was read as hold")
	}
}
