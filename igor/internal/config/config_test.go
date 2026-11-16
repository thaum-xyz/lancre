package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

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
	sauna := rules.Rule{OnSilent: rules.SilentOff, MaxOn: 3 * time.Hour, OffOnStart: true}
	want := rules.Config{HeartbeatTimeout: 10 * time.Minute, Grace: 2 * time.Minute}
	want.Rules = [rules.N]rules.Rule{0: {OnSilent: rules.SilentOff}, 3: {MaxOn: 2 * time.Minute}, 5: sauna, 6: sauna}
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
	if !r.Active || r.Rules[0].OnSilent != rules.SilentOff || r.Rules[4].OnSilent != rules.Hold || r.Rules[4].MaxOn != time.Hour {
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

func TestAnalogInputsSchema(t *testing.T) {
	c, err := Parse([]byte("analog_inputs:\n  interval: 5s\n  2: {correction: 5.5}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.AnalogInputs.Interval != 5*time.Second || c.Correction(2) != 5.5 || c.Correction(1) != evokCorrection {
		t.Fatalf("%+v", c.AnalogInputs)
	}
	if c, _ := Parse([]byte("analog_inputs:\n  1: {correction: 5.5}\n")); c.AnalogInputs.Interval != 10*time.Second {
		t.Fatalf("default interval lost: %v", c.AnalogInputs.Interval)
	}
	for name, y := range map[string]string{
		"input 3":       "analog_inputs:\n  3: {correction: 1}\n",
		"unknown field": "analog_inputs:\n  1: {corection: 1}\n",
		"old schema":    "analog_inputs:\n  correction: {1: 5.5}\n",
		"negative":      "analog_inputs:\n  1: {correction: -1}\n",
	} {
		if _, err := Parse([]byte(y)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestObserveSensorsWarnAndOutputIsRejected(t *testing.T) {
	c, err := Parse([]byte("mode: observe\nonewire: {enabled: true}\nanalog_inputs: {enabled: true}\n"))
	if err != nil || len(c.Warnings()) != 2 {
		t.Fatalf("err=%v warnings=%v", err, c.Warnings())
	}
	if c, _ := Parse([]byte("mode: observe\n")); len(c.Warnings()) != 0 || c.AnalogOutEnabled() {
		t.Fatalf("plain observe: %v", c.Warnings())
	}
	if _, err := Parse([]byte("mode: observe\nanalog_outputs: {enabled: true}\n")); err == nil {
		t.Fatal("analog output accepted in observe mode")
	}
	if c, _ := Parse([]byte("mode: active\n")); !c.AnalogOutEnabled() {
		t.Fatal("analog output should default to on in active mode")
	}
}

func TestOnSilentOn(t *testing.T) {
	for _, y := range []string{"relays:\n  5: {on_silent: \"on\"}\n", "relays:\n  5: {on_silent: on}\n"} {
		c, err := Parse([]byte(y))
		if err != nil {
			t.Fatal(err)
		}
		if r, _ := c.Rules(); r.Rules[4].OnSilent != rules.SilentOn {
			t.Fatalf("%q: %+v", y, r.Rules[4])
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
	if r, _ := c.Rules(); r.Rules[0].OnSilent != rules.SilentOff {
		t.Fatal("bare off was read as hold")
	}
}

// The HAOS app's default options, as Supervisor writes them to
// /data/options.json, must be a valid config with the live rules.
func TestHAOSAppOptionsGiveTheLiveRules(t *testing.T) {
	raw, err := os.ReadFile("../../../haos/igor/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var app struct {
		Options map[string]any `yaml:"options"`
	}
	if err := yaml.Unmarshal(raw, &app); err != nil {
		t.Fatal(err)
	}
	options, err := json.Marshal(app.Options)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(options)
	if err != nil {
		t.Fatalf("%s\n%v", options, err)
	}
	example, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want, err := Parse(example)
	if err != nil {
		t.Fatal(err)
	}
	wantRules, _ := want.Rules()
	wantRules.Active = true
	if got, _ := c.Rules(); got != wantRules {
		t.Fatalf("rules =\n%+v\nwant\n%+v", got, wantRules)
	}
	if c.Mode != "active" || c.AnalogOutEnabled() || !c.AnalogEnabled() || !c.OneWireEnabled() {
		t.Fatalf("mode %s, ao %v, ai %v, 1-wire %v", c.Mode, c.AnalogOutEnabled(), c.AnalogEnabled(), c.OneWireEnabled())
	}
	if c.Correction(1) != 5.549713 || c.Correction(2) != 5.551579 {
		t.Fatalf("corrections: %v %v", c.Correction(1), c.Correction(2))
	}
	if c.MQTT.TopicPrefix != "igor" || c.MQTT.DiscoveryPrefix != "homeassistant" || c.HTTP.Listen != ":9101" {
		t.Fatalf("mqtt/http: %+v %+v", c.MQTT, c.HTTP)
	}
}
