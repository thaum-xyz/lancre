// Package config loads igor's YAML file. Board wiring is not configurable;
// see package unipi.
package config

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/thaum-xyz/lancre/igor/internal/rules"
)

type Config struct {
	Mode  string `yaml:"mode"`  // observe | active
	Board string `yaml:"board"` // unipi1.1 | fake

	MQTT struct {
		URL             string `yaml:"url"`
		Username        string `yaml:"username"`
		PasswordFile    string `yaml:"password_file"`
		ClientID        string `yaml:"client_id"`
		TopicPrefix     string `yaml:"topic_prefix"`
		DiscoveryPrefix string `yaml:"discovery_prefix"`
		Password        string `yaml:"-"` // from IGOR_MQTT_PASSWORD or password_file
	} `yaml:"mqtt"`

	HTTP struct {
		Listen string `yaml:"listen"`
	} `yaml:"http"`

	Silent struct {
		HeartbeatTimeout time.Duration `yaml:"heartbeat_timeout"`
		Grace            time.Duration `yaml:"grace"`
	} `yaml:"silent"`

	// Keys are "defaults" or a relay number "1".."8".
	Relays map[string]RelayRule `yaml:"relays"`

	DigitalInputs struct {
		Debounce time.Duration `yaml:"debounce"`
		Invert   []int         `yaml:"invert"`
	} `yaml:"digital_inputs"`

	AnalogInputs struct {
		Enabled    *bool           `yaml:"enabled"` // default: true in active, false in observe
		Interval   time.Duration   `yaml:"interval"`
		Correction map[int]float64 `yaml:"correction"`
	} `yaml:"analog_inputs"`

	OneWire struct {
		Enabled  *bool         `yaml:"enabled"` // default: true in active, false in observe
		Interval time.Duration `yaml:"interval"`
	} `yaml:"onewire"`
}

// RelayRule fields are pointers so that an unset field falls back to the
// "defaults" entry.
type RelayRule struct {
	OnSilent   *string   `yaml:"on_silent"` // hold | off
	MaxOn      *Duration `yaml:"max_on"`    // 0 = unlimited
	OffOnStart *bool     `yaml:"off_on_start"`
}

// Duration is a time.Duration that also accepts a bare 0.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Value == "0" {
		*d = 0
		return nil
	}
	v, err := time.ParseDuration(n.Value)
	*d = Duration(v)
	return err
}

// evokCorrection is Evok's fallback divider ratio for a board without
// calibration data.
const evokCorrection = 5.564920867

func defaults() Config {
	var c Config
	c.Mode = "observe"
	c.Board = "unipi1.1"
	c.MQTT.URL = "tcp://127.0.0.1:1883"
	c.MQTT.ClientID = "igor"
	c.MQTT.TopicPrefix = "igor"
	c.MQTT.DiscoveryPrefix = "homeassistant"
	c.HTTP.Listen = ":9101"
	c.Silent.HeartbeatTimeout = 10 * time.Minute
	c.Silent.Grace = 2 * time.Minute
	c.DigitalInputs.Debounce = 50 * time.Millisecond
	c.AnalogInputs.Interval = 10 * time.Second
	c.OneWire.Interval = 30 * time.Second
	return c
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	c, err := Parse(raw)
	if err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if pw := os.Getenv("IGOR_MQTT_PASSWORD"); pw != "" {
		c.MQTT.Password = pw
	} else if c.MQTT.PasswordFile != "" {
		b, err := os.ReadFile(c.MQTT.PasswordFile)
		if err != nil {
			return c, err
		}
		c.MQTT.Password = strings.TrimSpace(string(b))
	}
	return c, nil
}

func Parse(raw []byte) (Config, error) {
	c := defaults()
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true) // a typo in a failsafe setting must not pass silently
	if err := dec.Decode(&c); err != nil && err.Error() != "EOF" {
		return c, err
	}
	return c, c.validate()
}

func (c *Config) validate() error {
	if c.Mode != "observe" && c.Mode != "active" {
		return fmt.Errorf("mode %q: want observe or active", c.Mode)
	}
	if c.Board != "unipi1.1" && c.Board != "fake" {
		return fmt.Errorf("board %q: want unipi1.1 or fake", c.Board)
	}
	if c.Silent.HeartbeatTimeout <= 0 || c.Silent.Grace < 0 {
		return fmt.Errorf("silent: heartbeat_timeout must be positive and grace not negative")
	}
	if c.AnalogInputs.Interval < time.Second || c.OneWire.Interval < 2*time.Second {
		return fmt.Errorf("intervals: analog_inputs at least 1s, onewire at least 2s")
	}
	for _, n := range c.DigitalInputs.Invert {
		if n < 1 || n > 12 {
			return fmt.Errorf("digital_inputs.invert: %d is not 1..12", n)
		}
	}
	for ch := range c.AnalogInputs.Correction {
		if ch != 1 && ch != 2 {
			return fmt.Errorf("analog_inputs.correction: %d is not 1 or 2", ch)
		}
	}
	_, err := c.Rules()
	return err
}

// Rules resolves the relay section into the rule engine's configuration.
func (c *Config) Rules() (rules.Config, error) {
	out := rules.Config{
		Active:           c.Mode == "active",
		HeartbeatTimeout: c.Silent.HeartbeatTimeout,
		Grace:            c.Silent.Grace,
	}
	def, err := resolve(rules.Rule{}, c.Relays["defaults"])
	if err != nil {
		return out, fmt.Errorf("relays.defaults: %w", err)
	}
	for i := range out.Rules {
		out.Rules[i] = def
	}
	for key, r := range c.Relays {
		if key == "defaults" {
			continue
		}
		n, err := strconv.Atoi(key)
		if err != nil || n < 1 || n > rules.N {
			return out, fmt.Errorf("relays.%s: want defaults or 1..%d", key, rules.N)
		}
		if out.Rules[n-1], err = resolve(def, r); err != nil {
			return out, fmt.Errorf("relays.%s: %w", key, err)
		}
	}
	return out, nil
}

func resolve(base rules.Rule, r RelayRule) (rules.Rule, error) {
	if r.OnSilent != nil {
		switch *r.OnSilent {
		case "off":
			base.OffOnSilent = true
		case "hold":
			base.OffOnSilent = false
		default:
			// YAML 1.1 parsers turn a bare off into false; this one does not,
			// but the message spells out the quoting to be safe.
			return base, fmt.Errorf(`on_silent %q: want "hold" or "off"`, *r.OnSilent)
		}
	}
	if r.MaxOn != nil {
		if *r.MaxOn < 0 {
			return base, fmt.Errorf("max_on must not be negative")
		}
		base.MaxOn = time.Duration(*r.MaxOn)
	}
	if r.OffOnStart != nil {
		base.OffOnStart = *r.OffOnStart
	}
	return base, nil
}

// AnalogEnabled and OneWireEnabled default to off in observe mode: both
// chips need multi-step transactions that would collide with Evok's.
func (c *Config) AnalogEnabled() bool  { return c.enabled(c.AnalogInputs.Enabled) }
func (c *Config) OneWireEnabled() bool { return c.enabled(c.OneWire.Enabled) }

func (c *Config) enabled(set *bool) bool {
	if set != nil {
		return *set
	}
	return c.Mode == "active"
}

// Correction returns the divider ratio for analog input ch.
func (c *Config) Correction(ch int) float64 {
	if v, ok := c.AnalogInputs.Correction[ch]; ok && v > 0 {
		return v
	}
	return evokCorrection
}

// Inverted reports whether digital input n reads backwards.
func (c *Config) Inverted(n int) bool {
	for _, i := range c.DigitalInputs.Invert {
		if i == n {
			return true
		}
	}
	return false
}
