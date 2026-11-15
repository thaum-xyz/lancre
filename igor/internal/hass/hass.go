// Package hass holds the MQTT contract with Home Assistant: topic names and
// discovery payloads (lancre#9).
package hass

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Topics struct {
	Prefix    string
	Discovery string
}

func (t Topics) Status() string              { return t.Prefix + "/status" }
func (t Topics) Heartbeat() string           { return t.Prefix + "/ha/heartbeat" }
func (t Topics) HAStatus() string            { return t.Discovery + "/status" }
func (t Topics) RelaySetAll() string         { return t.Prefix + "/relay/+/set" }
func (t Topics) RelayState(n int) string     { return fmt.Sprintf("%s/relay/%d/state", t.Prefix, n) }
func (t Topics) RelaySet(n int) string       { return fmt.Sprintf("%s/relay/%d/set", t.Prefix, n) }
func (t Topics) RelayAttrs(n int) string     { return fmt.Sprintf("%s/relay/%d/attributes", t.Prefix, n) }
func (t Topics) Input(n int) string          { return fmt.Sprintf("%s/di/%d/state", t.Prefix, n) }
func (t Topics) Analog(n int) string         { return fmt.Sprintf("%s/ai/%d/state", t.Prefix, n) }
func (t Topics) Temp(addr string) string     { return fmt.Sprintf("%s/temp/%s/state", t.Prefix, addr) }
func (t Topics) AnalogOutState(n int) string { return fmt.Sprintf("%s/ao/%d/state", t.Prefix, n) }
func (t Topics) AnalogOutSet(n int) string   { return fmt.Sprintf("%s/ao/%d/set", t.Prefix, n) }
func (t Topics) AnalogOutSetAll() string     { return t.Prefix + "/ao/+/set" }

// RelayFromSet returns the relay number of a ".../relay/N/set" topic.
func (t Topics) RelayFromSet(topic string) (int, bool) {
	rest, ok := strings.CutPrefix(topic, t.Prefix+"/relay/")
	if !ok {
		return 0, false
	}
	num, ok := strings.CutSuffix(rest, "/set")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(num)
	return n, err == nil
}

// Payloads shared with Home Assistant's defaults.
const (
	On      = "ON"
	Off     = "OFF"
	Online  = "online"
	Offline = "offline"
)

func OnOff(on bool) string {
	if on {
		return On
	}
	return Off
}

// Message is one retained discovery config.
type Message struct {
	Topic   string
	Payload []byte
}

type device struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"manufacturer"`
	Model        string   `json:"model"`
	SWVersion    string   `json:"sw_version"`
}

type origin struct {
	Name       string `json:"name"`
	SWVersion  string `json:"sw_version"`
	SupportURL string `json:"support_url"`
}

// entity is the union of the discovery keys igor uses. Entity ids come from
// the device name plus Name: "UniPi" + "Relay 1" = switch.unipi_relay_1.
type entity struct {
	Name              string   `json:"name"`
	UniqueID          string   `json:"unique_id"`
	StateTopic        string   `json:"state_topic"`
	CommandTopic      string   `json:"command_topic,omitempty"`
	AttributesTopic   string   `json:"json_attributes_topic,omitempty"`
	AvailabilityTopic string   `json:"availability_topic"`
	DeviceClass       string   `json:"device_class,omitempty"`
	Unit              string   `json:"unit_of_measurement,omitempty"`
	StateClass        string   `json:"state_class,omitempty"`
	Precision         *int     `json:"suggested_display_precision,omitempty"`
	ExpireAfter       int      `json:"expire_after,omitempty"`
	Min               *float64 `json:"min,omitempty"`
	Max               *float64 `json:"max,omitempty"`
	Step              *float64 `json:"step,omitempty"`
	Mode              string   `json:"mode,omitempty"`
	Device            device   `json:"device"`
	Origin            origin   `json:"origin"`
}

// Discovery builds the configs igor announces.
type Discovery struct {
	Topics  Topics
	Version string
}

func (d Discovery) message(component, id string, e entity) Message {
	e.UniqueID = "igor_" + id
	e.AvailabilityTopic = d.Topics.Status()
	e.Device = device{Identifiers: []string{"igor_unipi"}, Name: "UniPi", Manufacturer: "UniPi", Model: "UniPi 1.1", SWVersion: d.Version}
	e.Origin = origin{Name: "igor", SWVersion: d.Version, SupportURL: "https://github.com/thaum-xyz/lancre"}
	payload, err := json.Marshal(e)
	if err != nil {
		panic(err) // entity has only marshalable fields
	}
	return Message{Topic: fmt.Sprintf("%s/%s/igor/%s/config", d.Topics.Discovery, component, id), Payload: payload}
}

func (d Discovery) Relay(n int) Message {
	return d.message("switch", fmt.Sprintf("relay_%d", n), entity{
		Name: fmt.Sprintf("Relay %d", n), StateTopic: d.Topics.RelayState(n),
		CommandTopic: d.Topics.RelaySet(n), AttributesTopic: d.Topics.RelayAttrs(n)})
}

func (d Discovery) Input(n int) Message {
	return d.message("binary_sensor", fmt.Sprintf("di_%d", n), entity{
		Name: fmt.Sprintf("DI %d", n), StateTopic: d.Topics.Input(n)})
}

// Analog and Temp expire in Home Assistant when igor stops publishing a
// bad reading; interval is how often igor publishes.
func (d Discovery) Analog(n int, interval time.Duration) Message {
	p := 3
	return d.message("sensor", fmt.Sprintf("ai_%d", n), entity{
		Name: fmt.Sprintf("AI %d", n), StateTopic: d.Topics.Analog(n), DeviceClass: "voltage",
		Unit: "V", StateClass: "measurement", Precision: &p, ExpireAfter: expiry(interval, 60)})
}

// AnalogOut is a number entity, 0-10 V.
func (d Discovery) AnalogOut(n int) Message {
	lo, hi, step := 0.0, 10.0, 0.1
	return d.message("number", fmt.Sprintf("ao_%d", n), entity{
		Name: fmt.Sprintf("AO %d", n), StateTopic: d.Topics.AnalogOutState(n), CommandTopic: d.Topics.AnalogOutSet(n),
		DeviceClass: "voltage", Unit: "V", Min: &lo, Max: &hi, Step: &step, Mode: "box"})
}

func (d Discovery) Temp(addr string, interval time.Duration) Message {
	p := 1
	return d.message("sensor", "temp_"+strings.ToLower(addr), entity{
		Name: "Temp " + addr, StateTopic: d.Topics.Temp(addr), DeviceClass: "temperature",
		Unit: "°C", StateClass: "measurement", Precision: &p, ExpireAfter: expiry(interval, 120)})
}

// expiry allows three missed readings, with a floor in seconds.
func expiry(interval time.Duration, floor int) int {
	return max(floor, int((4 * interval).Seconds()))
}

// RelayAttributes is the JSON on a relay's attributes topic.
func RelayAttributes(locked bool, lastFailsafe string) []byte {
	var last *string
	if lastFailsafe != "" {
		last = &lastFailsafe
	}
	b, _ := json.Marshal(struct {
		Locked       bool    `json:"locked"`
		LastFailsafe *string `json:"last_failsafe"`
	}{locked, last})
	return b
}
