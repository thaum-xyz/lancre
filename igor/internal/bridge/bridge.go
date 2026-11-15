// Package bridge connects the board to MQTT: it feeds the rule engine with
// events, carries out its effects, and publishes inputs and sensors.
package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/thaum-xyz/lancre/igor/internal/config"
	"github.com/thaum-xyz/lancre/igor/internal/hass"
	"github.com/thaum-xyz/lancre/igor/internal/rules"
	"github.com/thaum-xyz/lancre/igor/internal/unipi"
)

type Board interface {
	ReadRelays() ([unipi.NumRelays]bool, error)
	SetRelay(n int, on bool) error
	ReadInputs() [unipi.NumInputs]unipi.Input
	ReadAnalog(ch int, correction float64) (float64, error)
	ReadTemps() (map[string]float64, error)
}

// Publisher sends one MQTT message at QoS 1 without blocking. Messages sent
// while disconnected may be dropped: everything is re-announced on connect.
type Publisher interface {
	Publish(topic string, payload []byte, retain bool)
	// Connected reports whether the broker connection is up right now.
	Connected() bool
}

const (
	relayPoll = time.Second
	inputPoll = 10 * time.Millisecond
)

type Bridge struct {
	cfg     config.Config
	board   Board
	pub     Publisher
	metrics *Metrics
	log     *slog.Logger
	topics  hass.Topics
	disc    hass.Discovery
	now     func() time.Time

	mu      sync.Mutex // guards everything below and serialises relay writes
	ctl     *rules.Controller
	linked  bool // what the rule engine was last told about the broker connection
	inputs  [unipi.NumInputs]debounced
	analogs [unipi.NumAnalogs]*float64 // nil = no valid reading
	temps   map[string]*float64        // every probe ever seen; nil = no valid reading
}

type debounced struct {
	known     bool // a stable value has been published
	available bool
	on        bool
	candidate bool
	since     time.Time
}

// New reads the relay chip and applies the start rules. It fails when the
// chip cannot be read: igor must not guess relay states.
func New(cfg config.Config, board Board, pub Publisher, metrics *Metrics, log *slog.Logger, version string) (*Bridge, error) {
	return newBridge(cfg, board, pub, metrics, log, version, time.Now)
}

func newBridge(cfg config.Config, board Board, pub Publisher, metrics *Metrics, log *slog.Logger, version string, now func() time.Time) (*Bridge, error) {
	rc, err := cfg.Rules()
	if err != nil {
		return nil, err
	}
	chip, err := board.ReadRelays()
	if err != nil {
		return nil, fmt.Errorf("read relays at start: %w", err)
	}
	topics := hass.Topics{Prefix: cfg.MQTT.TopicPrefix, Discovery: cfg.MQTT.DiscoveryPrefix}
	b := &Bridge{
		cfg: cfg, board: board, pub: pub, metrics: metrics, log: log, now: now,
		topics: topics, disc: hass.Discovery{Topics: topics, Version: version},
		temps: map[string]*float64{},
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var effects []rules.Effect
	b.ctl, effects = rules.New(rc, chip, b.now())
	b.apply(effects)
	b.relayMetrics()
	return b, nil
}

// Run polls the board until ctx is done.
func (b *Bridge) Run(ctx context.Context) {
	var wg sync.WaitGroup
	loop := func(every time.Duration, f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(every)
			defer t.Stop()
			for {
				f()
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}()
	}
	loop(relayPoll, b.pollRelays)
	loop(inputPoll, b.pollInputs)
	if b.cfg.AnalogEnabled() {
		loop(b.cfg.AnalogInputs.Interval, b.pollAnalogs)
	}
	if b.cfg.OneWireEnabled() {
		loop(b.cfg.OneWire.Interval, b.pollTemps)
	}
	wg.Wait()
}

// OnConnect and OnDisconnect follow the broker connection.
func (b *Bridge) OnConnect() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.pub.Connected() {
		return // the link dropped again before the handler ran
	}
	b.linked = true
	b.metrics.connected.Set(1)
	b.log.Info("connected to broker")
	b.apply(b.ctl.Connected(b.now()))
}

func (b *Bridge) OnDisconnect(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.log.Warn("lost broker connection", "err", err)
	b.unlink()
}

// unlink tells the rule engine the broker is gone. Callers hold b.mu.
func (b *Bridge) unlink() {
	b.linked = false
	b.metrics.connected.Set(0)
	b.apply(b.ctl.Disconnected(b.now()))
}

// OnMessage handles everything igor subscribes to.
func (b *Bridge) OnMessage(topic string, payload []byte, retained bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	// A retained heartbeat or status is a replay, not proof of life.
	if retained && (topic == b.topics.Heartbeat() || topic == b.topics.HAStatus()) {
		b.log.Warn("ignored retained message", "topic", topic)
		return
	}
	switch {
	case topic == b.topics.Heartbeat():
		b.apply(b.ctl.Heartbeat(now))
	case topic == b.topics.HAStatus():
		switch string(payload) {
		case hass.Online:
			b.log.Info("Home Assistant is online")
			b.apply(b.ctl.HAStatus(true, now))
		case hass.Offline:
			b.log.Warn("Home Assistant is offline")
			b.apply(b.ctl.HAStatus(false, now))
		}
	default:
		n, ok := b.topics.RelayFromSet(topic)
		p := string(payload)
		if !ok || n < 1 || n > rules.N || (p != hass.On && p != hass.Off) {
			b.log.Warn("ignored message", "topic", topic, "payload", p)
			return
		}
		b.apply(b.ctl.Command(n, p == hass.On, retained, now))
	}
}

// apply carries out the rule engine's decisions. Callers hold b.mu.
func (b *Bridge) apply(effects []rules.Effect) {
	for _, e := range effects {
		circuit := strconv.Itoa(e.Relay)
		switch e.Kind {
		case rules.Write:
			if err := b.board.SetRelay(e.Relay, e.On); err != nil {
				// The next relay poll sees the mismatch and writes again.
				b.metrics.busErrors.WithLabelValues("i2c").Inc()
				b.log.Error("relay write failed", "relay", e.Relay, "on", e.On, "err", err)
			}
		case rules.Publish:
			b.publishRelay(e.Relay)
		case rules.Announce:
			b.announce()
		case rules.Failsafe:
			b.metrics.failsafes.WithLabelValues(circuit, e.Reason).Inc()
			b.log.Warn("failsafe switched relay off", "relay", e.Relay, "reason", e.Reason)
		case rules.Refused:
			b.metrics.refused.WithLabelValues(circuit, e.Reason).Inc()
			b.log.Warn("command refused", "relay", e.Relay, "on", e.On, "reason", e.Reason)
		case rules.Drift:
			b.metrics.drift.WithLabelValues(circuit).Inc()
			b.log.Warn("relay differs from the chip, writing again", "relay", e.Relay)
		case rules.Silence:
			if e.On {
				b.log.Warn("Home Assistant is silent")
			} else {
				b.log.Info("Home Assistant is no longer silent")
			}
		}
	}
	b.metrics.silent.Set(flag(b.ctl.Silent()))
	b.metrics.lastHeartbeat.Set(float64(b.ctl.LastHeartbeat().Unix()))
}

func (b *Bridge) publishRelay(n int) {
	r := b.ctl.Relay(n)
	b.pub.Publish(b.topics.RelayState(n), []byte(hass.OnOff(r.On)), true)
	b.pub.Publish(b.topics.RelayAttrs(n), hass.RelayAttributes(r.Locked, r.LastFailsafe), true)
	b.relayMetrics()
}

func (b *Bridge) relayMetrics() {
	for n := 1; n <= rules.N; n++ {
		r := b.ctl.Relay(n)
		b.metrics.relay.WithLabelValues(strconv.Itoa(n)).Set(flag(r.On))
		b.metrics.locked.WithLabelValues(strconv.Itoa(n)).Set(flag(r.Locked))
	}
}

// announce republishes discovery and every known state.
func (b *Bridge) announce() {
	b.pub.Publish(b.topics.Status(), []byte(hass.Online), true)
	config := func(m hass.Message) { b.pub.Publish(m.Topic, m.Payload, true) }
	for n := 1; n <= rules.N; n++ {
		config(b.disc.Relay(n))
		b.publishRelay(n)
	}
	for i, in := range b.inputs {
		if in.known && in.available {
			config(b.disc.Input(i + 1))
			b.publishInput(i + 1)
		}
	}
	if b.cfg.AnalogEnabled() {
		for i, v := range b.analogs {
			config(b.disc.Analog(i+1, b.cfg.AnalogInputs.Interval))
			if v != nil {
				b.publishAnalog(i+1, *v)
			}
		}
	}
	for addr, v := range b.temps {
		config(b.disc.Temp(addr, b.cfg.OneWire.Interval))
		if v != nil {
			b.publishTemp(addr, *v)
		}
	}
}

func (b *Bridge) pollRelays() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if b.linked && !b.pub.Connected() {
		// The connection-lost callback can be late or lost in a reconnect race.
		b.log.Warn("broker connection is down")
		b.unlink()
	}
	chip, err := b.board.ReadRelays()
	if err != nil {
		b.metrics.busErrors.WithLabelValues("i2c").Inc()
		b.log.Error("relay readback failed", "err", err)
		b.apply(b.ctl.Tick(now)) // failsafes still run on the last known state
		return
	}
	b.apply(b.ctl.Observed(chip, now))
	b.relayMetrics()
}

func (b *Bridge) pollInputs() {
	levels := b.board.ReadInputs()
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	for i, l := range levels {
		in := &b.inputs[i]
		n := i + 1
		if !l.Available {
			continue
		}
		on := l.On != b.cfg.Inverted(n)
		if !in.known {
			// First reading: publish at once, there is nothing to debounce against.
			*in = debounced{known: true, available: true, on: on, candidate: on, since: now}
			m := b.disc.Input(n)
			b.pub.Publish(m.Topic, m.Payload, true)
			b.publishInput(n)
			continue
		}
		if on != in.candidate {
			in.candidate, in.since = on, now
		}
		if in.candidate != in.on && now.Sub(in.since) >= b.cfg.DigitalInputs.Debounce {
			in.on = in.candidate
			b.publishInput(n)
		}
	}
}

func (b *Bridge) publishInput(n int) {
	on := b.inputs[n-1].on
	b.pub.Publish(b.topics.Input(n), []byte(hass.OnOff(on)), true)
	b.metrics.input.WithLabelValues(strconv.Itoa(n)).Set(flag(on))
}

func (b *Bridge) pollAnalogs() {
	for ch := 1; ch <= unipi.NumAnalogs; ch++ {
		v, err := b.board.ReadAnalog(ch, b.cfg.Correction(ch)) // ~0.3 s, outside the lock
		b.mu.Lock()
		first := b.analogs[ch-1] == nil
		if err != nil {
			b.metrics.busErrors.WithLabelValues("i2c").Inc()
			b.metrics.analog.DeleteLabelValues(strconv.Itoa(ch))
			b.log.Error("analog input read failed", "ai", ch, "err", err)
			b.analogs[ch-1] = nil
		} else {
			b.analogs[ch-1] = &v
			if first {
				m := b.disc.Analog(ch, b.cfg.AnalogInputs.Interval)
				b.pub.Publish(m.Topic, m.Payload, true)
			}
			b.publishAnalog(ch, v)
		}
		b.mu.Unlock()
	}
}

func (b *Bridge) publishAnalog(ch int, v float64) {
	b.pub.Publish(b.topics.Analog(ch), []byte(strconv.FormatFloat(v, 'f', 3, 64)), false)
	b.metrics.analog.WithLabelValues(strconv.Itoa(ch)).Set(v)
}

func (b *Bridge) pollTemps() {
	temps, err := b.board.ReadTemps() // ~0.8 s, outside the lock
	b.mu.Lock()
	defer b.mu.Unlock()
	if err != nil {
		b.metrics.busErrors.WithLabelValues("onewire").Inc()
		b.log.Error("temperature read failed", "err", err)
	}
	for addr := range b.temps {
		if _, ok := temps[addr]; !ok {
			b.temps[addr] = nil // no publish: Home Assistant expires the sensor
			b.metrics.temp.DeleteLabelValues(addr)
		}
	}
	for addr, v := range temps {
		if _, seen := b.temps[addr]; !seen {
			b.log.Info("found temperature probe", "address", addr)
			m := b.disc.Temp(addr, b.cfg.OneWire.Interval)
			b.pub.Publish(m.Topic, m.Payload, true)
		}
		b.temps[addr] = &v
		b.publishTemp(addr, v)
	}
}

func (b *Bridge) publishTemp(addr string, v float64) {
	b.pub.Publish(b.topics.Temp(addr), []byte(strconv.FormatFloat(v, 'f', 2, 64)), false)
	b.metrics.temp.WithLabelValues(addr).Set(v)
}

func flag(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
