package bridge

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/thaum-xyz/lancre/igor/internal/config"
	"github.com/thaum-xyz/lancre/igor/internal/unipi"
)

type board struct {
	relays   [unipi.NumRelays]bool
	writes   int
	failSet  bool
	inputs   [unipi.NumInputs]unipi.Input
	analog   float64
	analogOK bool
	temps    map[string]float64
	tempErr  error
}

func (b *board) ReadRelays() ([unipi.NumRelays]bool, error) { return b.relays, nil }
func (b *board) SetRelay(n int, on bool) error {
	b.writes++
	if b.failSet {
		return errors.New("i2c: remote I/O error")
	}
	b.relays[n-1] = on
	return nil
}
func (b *board) ReadInputs() [unipi.NumInputs]unipi.Input { return b.inputs }
func (b *board) ReadAnalog(int, float64) (float64, error) {
	if !b.analogOK {
		return 0, errors.New("not ready")
	}
	return b.analog, nil
}
func (b *board) ReadTemps() (map[string]float64, error) { return b.temps, b.tempErr }

type wire struct {
	sent   []string          // "topic payload" in order
	latest map[string]string // last payload per topic
	down   bool
}

func (w *wire) Connected() bool { return !w.down }

func (w *wire) Publish(topic string, payload []byte, _ bool) {
	w.sent = append(w.sent, topic+" "+string(payload))
	w.latest[topic] = string(payload)
}

type rig struct {
	t     *testing.T
	b     *Bridge
	board *board
	wire  *wire
	now   time.Time
	m     *Metrics
}

func newRig(t *testing.T, yaml string, hw *board) *rig {
	t.Helper()
	cfg, err := config.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, board: hw, wire: &wire{latest: map[string]string{}}, now: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)}
	r.m = NewMetrics(prometheus.NewRegistry(), "test", cfg.Mode)
	r.b, err = newBridge(cfg, hw, r.wire, r.m, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", func() time.Time { return r.now })
	if err != nil {
		t.Fatal(err)
	}
	return r
}

const live = `
mode: active
relays:
  1: {on_silent: "off"}
  4: {max_on: 2m}
  6: {on_silent: "off", max_on: 3h, off_on_start: true}
  7: {on_silent: "off", max_on: 3h, off_on_start: true}
`

func (r *rig) expect(topic, payload string) {
	r.t.Helper()
	if got := r.wire.latest[topic]; got != payload {
		r.t.Fatalf("%s = %q, want %q", topic, got, payload)
	}
}

func TestCommandSwitchesTheChipAndReportsBack(t *testing.T) {
	r := newRig(t, live, &board{})
	r.b.OnConnect()
	r.b.OnMessage("igor/relay/5/set", []byte("ON"), false)
	if !r.board.relays[4] {
		t.Fatal("relay 5 not switched")
	}
	r.expect("igor/relay/5/state", "ON")
	r.expect("igor/relay/5/attributes", `{"locked":false,"last_failsafe":null}`)
	if testutil.ToFloat64(r.m.relay.WithLabelValues("5")) != 1 {
		t.Fatal("metric not set")
	}
}

func TestStartSwitchesSaunaOffAndKeepsTheRest(t *testing.T) {
	hw := &board{relays: [unipi.NumRelays]bool{4: true, 6: true}}
	r := newRig(t, live, hw)
	if want := ([unipi.NumRelays]bool{4: true}); hw.relays != want {
		t.Fatalf("relays = %v", hw.relays)
	}
	r.b.OnConnect()
	r.expect("igor/relay/7/attributes", `{"locked":false,"last_failsafe":"start"}`)
	r.expect("igor/relay/5/state", "ON")
}

func TestConnectAnnouncesStatusDiscoveryAndStates(t *testing.T) {
	hw := &board{analog: 1.5, analogOK: true, temps: map[string]float64{"28FF4C30041503A7": 58.75}}
	hw.inputs[4] = unipi.Input{On: true, Available: true}
	r := newRig(t, live, hw)
	r.b.pollInputs()
	r.b.pollAnalogs()
	r.b.pollTemps()
	r.wire.sent, r.wire.latest = nil, map[string]string{}
	r.b.OnConnect()

	if r.wire.sent[0] != "igor/status online" {
		t.Fatalf("first message: %s", r.wire.sent[0])
	}
	r.expect("igor/di/5/state", "ON")
	r.expect("igor/ai/1/state", "1.500")
	r.expect("igor/temp/28FF4C30041503A7/state", "58.75")
	var relay struct {
		Name, UniqueID, CommandTopic, AvailabilityTopic string
		Device                                          struct{ Name string }
	}
	raw := r.wire.latest["homeassistant/switch/igor/relay_1/config"]
	if err := json.Unmarshal([]byte(strings.NewReplacer("unique_id", "UniqueID", "command_topic", "CommandTopic", "availability_topic", "AvailabilityTopic").Replace(raw)), &relay); err != nil {
		t.Fatal(err)
	}
	if relay.Name != "Relay 1" || relay.UniqueID != "igor_relay_1" || relay.CommandTopic != "igor/relay/1/set" ||
		relay.AvailabilityTopic != "igor/status" || relay.Device.Name != "UniPi" {
		t.Fatalf("relay config: %s", raw)
	}
	for _, topic := range []string{
		"homeassistant/binary_sensor/igor/di_5/config",
		"homeassistant/sensor/igor/ai_1/config",
		"homeassistant/sensor/igor/temp_28ff4c30041503a7/config",
	} {
		if r.wire.latest[topic] == "" {
			t.Errorf("no discovery on %s", topic)
		}
	}
	// DI 1 is unavailable on this board (GPIO4 taken): no entity for it.
	if _, ok := r.wire.latest["homeassistant/binary_sensor/igor/di_1/config"]; ok {
		t.Error("discovery published for an unavailable input")
	}
}

func TestFailedWriteIsRetriedByTheNextPoll(t *testing.T) {
	hw := &board{failSet: true}
	r := newRig(t, live, hw)
	r.b.OnMessage("igor/relay/5/set", []byte("ON"), false)
	if hw.relays[4] {
		t.Fatal("write should have failed")
	}
	hw.failSet = false
	r.b.pollRelays()
	if !hw.relays[4] {
		t.Fatal("relay 5 not re-asserted")
	}
	if testutil.ToFloat64(r.m.drift.WithLabelValues("5")) != 1 || testutil.ToFloat64(r.m.busErrors.WithLabelValues("i2c")) != 1 {
		t.Fatal("drift or bus error not counted")
	}
}

func TestSilenceDropsValveViaPolling(t *testing.T) {
	r := newRig(t, live, &board{})
	r.b.OnConnect()
	r.b.OnMessage("igor/relay/1/set", []byte("ON"), false)
	r.b.OnMessage("igor/relay/5/set", []byte("ON"), false)
	r.b.OnMessage("homeassistant/status", []byte("offline"), false)
	r.now = r.now.Add(2 * time.Minute)
	r.b.pollRelays()
	if want := ([unipi.NumRelays]bool{4: true}); r.board.relays != want {
		t.Fatalf("relays = %v", r.board.relays)
	}
	r.expect("igor/relay/1/state", "OFF")
	if testutil.ToFloat64(r.m.silent) != 1 || testutil.ToFloat64(r.m.failsafes.WithLabelValues("1", "silent")) != 1 {
		t.Fatal("silence not reported")
	}
	r.b.OnMessage("igor/ha/heartbeat", []byte("1"), false)
	if testutil.ToFloat64(r.m.silent) != 0 {
		t.Fatal("still silent after a heartbeat")
	}
}

func TestObserveRefusesAndFollowsTheChip(t *testing.T) {
	hw := &board{relays: [unipi.NumRelays]bool{6: true}}
	r := newRig(t, "mode: observe\nrelays:\n  7: {off_on_start: true, max_on: 1m}\n", hw)
	r.b.OnConnect()
	r.b.OnMessage("igor/relay/7/set", []byte("OFF"), false)
	hw.relays[4] = true // Evok switches heat demand on
	r.now = r.now.Add(time.Hour)
	r.b.pollRelays()
	if hw.writes != 0 {
		t.Fatalf("observe wrote to the chip %d times", hw.writes)
	}
	r.expect("igor/relay/5/state", "ON")
	r.expect("igor/relay/7/state", "ON")
	if testutil.ToFloat64(r.m.refused.WithLabelValues("7", "observe")) != 1 {
		t.Fatal("refusal not counted")
	}
}

func TestInputDebounceAndInvert(t *testing.T) {
	hw := &board{}
	for i := range hw.inputs {
		hw.inputs[i].Available = true
	}
	r := newRig(t, "digital_inputs: {debounce: 50ms, invert: [6]}\n", hw)
	r.b.pollInputs()
	r.expect("igor/di/5/state", "OFF")
	r.expect("igor/di/6/state", "ON") // inverted

	hw.inputs[4].On = true
	r.now = r.now.Add(10 * time.Millisecond)
	r.b.pollInputs()
	hw.inputs[4].On = false // a 10 ms glitch
	r.now = r.now.Add(10 * time.Millisecond)
	r.b.pollInputs()
	r.now = r.now.Add(100 * time.Millisecond)
	r.b.pollInputs()
	r.expect("igor/di/5/state", "OFF")

	hw.inputs[4].On = true
	r.b.pollInputs()
	r.now = r.now.Add(50 * time.Millisecond)
	r.b.pollInputs()
	r.expect("igor/di/5/state", "ON")
}

func TestBadSensorReadingsAreNotPublished(t *testing.T) {
	hw := &board{temps: map[string]float64{"28AA": 20, "28BB": 30}, analog: 2, analogOK: true}
	r := newRig(t, "mode: active\n", hw)
	r.b.pollTemps()
	r.b.pollAnalogs()
	r.wire.sent = nil

	hw.temps, hw.tempErr = map[string]float64{"28AA": 21}, errors.New("28BB: bad scratchpad CRC")
	hw.analogOK = false
	r.b.pollTemps()
	r.b.pollAnalogs()
	if len(r.wire.sent) != 1 || r.wire.sent[0] != "igor/temp/28AA/state 21.00" {
		t.Fatalf("sent: %v", r.wire.sent)
	}
	if testutil.CollectAndCount(r.m.temp) != 1 || testutil.CollectAndCount(r.m.analog) != 0 {
		t.Fatal("stale readings still exported")
	}
}

func TestLostConnectionIsNoticedWithoutTheCallback(t *testing.T) {
	r := newRig(t, live, &board{})
	r.b.OnConnect()
	r.b.OnMessage("igor/relay/1/set", []byte("ON"), false)
	r.wire.down = true // no OnDisconnect: the callback never came
	r.b.pollRelays()
	r.now = r.now.Add(2 * time.Minute)
	r.b.pollRelays()
	if r.board.relays[0] {
		t.Fatal("valve still on after the broker was gone for the grace period")
	}
	r.b.OnConnect() // a stale connect callback while still down
	if testutil.ToFloat64(r.m.connected) != 0 {
		t.Fatal("stale OnConnect counted as connected")
	}
}

func TestRetainedHeartbeatIsNotProofOfLife(t *testing.T) {
	r := newRig(t, live, &board{})
	r.b.OnConnect()
	r.now = r.now.Add(10 * time.Minute)
	r.b.OnMessage("igor/ha/heartbeat", []byte("1"), true)
	r.b.OnMessage("homeassistant/status", []byte("online"), true)
	r.b.pollRelays()
	if testutil.ToFloat64(r.m.silent) != 1 {
		t.Fatal("a retained heartbeat kept igor from going silent")
	}
}

func TestRetainedAndGarbageCommandsDoNothing(t *testing.T) {
	hw := &board{}
	r := newRig(t, live, hw)
	r.b.OnMessage("igor/relay/7/set", []byte("ON"), true)
	r.b.OnMessage("igor/relay/7/set", []byte("on"), false)
	r.b.OnMessage("igor/relay/9/set", []byte("ON"), false)
	if hw.writes != 0 {
		t.Fatalf("%d writes", hw.writes)
	}
}
