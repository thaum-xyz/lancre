// Package rules decides what each relay does: it applies commands from Home
// Assistant and the per-relay failsafes. It is pure: callers pass the time
// in and carry out the returned effects.
package rules

import "time"

// N is the number of relays on the board. Relays are numbered 1..N.
const N = 8

// Failsafe reasons and command refusal reasons, used as metric labels.
const (
	ReasonMaxOn  = "max_on"
	ReasonSilent = "silent"
	ReasonStart  = "start"

	RefusedObserve  = "observe"
	RefusedLocked   = "locked"
	RefusedSilent   = "silent"
	RefusedRetained = "retained"
)

// Silent is what a relay does while Home Assistant is silent.
type Silent int

const (
	Hold      Silent = iota // keep the state, keep obeying commands
	SilentOff               // switch off, refuse "on"
	SilentOn                // switch on, refuse "off"
)

// Rule is the failsafe configuration of one relay.
type Rule struct {
	OnSilent   Silent
	MaxOn      time.Duration // longest continuous on-time, 0 = unlimited
	OffOnStart bool          // switch off when igor starts
}

type Config struct {
	Active           bool // false = observe: never write, refuse every command
	HeartbeatTimeout time.Duration
	Grace            time.Duration // broker loss or HA offline shorter than this is not silence
	Rules            [N]Rule
}

type Relay struct {
	On           bool
	OnSince      time.Time
	Locked       bool   // tripped max_on; "on" is refused until an explicit "off"
	LastFailsafe string // "" or a Reason* constant
}

type Kind int

const (
	Write    Kind = iota // set relay Relay on the chip to On
	Publish              // publish state and attributes of relay Relay
	Announce             // publish availability, discovery and every state
	Failsafe             // a failsafe switched Relay to On for Reason
	Refused              // a command for Relay was refused for Reason
	Drift                // the chip disagreed with us about Relay in active mode
	Silence              // silence began (On) or ended (!On)
)

type Effect struct {
	Kind   Kind
	Relay  int
	On     bool
	Reason string
}

type Controller struct {
	cfg           Config
	relays        [N]Relay
	connected     bool
	haOffline     bool
	linkBad       bool
	linkBadSince  time.Time
	lastHeartbeat time.Time
	silent        bool
}

// New takes the relay states read from the chip at start.
func New(cfg Config, chip [N]bool, now time.Time) (*Controller, []Effect) {
	c := &Controller{cfg: cfg, linkBad: true, linkBadSince: now, lastHeartbeat: now}
	var out []Effect
	for i, on := range chip {
		if !on {
			continue
		}
		c.relays[i] = Relay{On: true, OnSince: now}
		if cfg.Active && cfg.Rules[i].OffOnStart {
			out = c.failsafe(i, ReasonStart, out)
		}
	}
	return c, out
}

func (c *Controller) Relay(n int) Relay { return c.relays[n-1] }
func (c *Controller) Silent() bool      { return c.silent }
func (c *Controller) Active() bool      { return c.cfg.Active }

// LastHeartbeat is the start time until the first heartbeat arrives.
func (c *Controller) LastHeartbeat() time.Time { return c.lastHeartbeat }

func (c *Controller) Connected(now time.Time) []Effect {
	c.connected = true
	c.link(now)
	return c.evaluate(now, []Effect{{Kind: Announce}})
}

func (c *Controller) Disconnected(now time.Time) []Effect {
	c.connected = false
	c.link(now)
	return c.evaluate(now, nil)
}

// HAStatus handles Home Assistant's birth and last-will messages.
func (c *Controller) HAStatus(online bool, now time.Time) []Effect {
	c.haOffline = !online
	c.link(now)
	var out []Effect
	if online {
		out = append(out, Effect{Kind: Announce})
	}
	return c.evaluate(now, out)
}

func (c *Controller) Heartbeat(now time.Time) []Effect {
	c.lastHeartbeat = now
	c.haOffline = false // a heartbeat proves HA is up even if its birth message was missed
	c.link(now)
	return c.evaluate(now, nil)
}

func (c *Controller) Tick(now time.Time) []Effect { return c.evaluate(now, nil) }

// Command handles "on"/"off" for relay n (1..N) from Home Assistant.
func (c *Controller) Command(n int, on, retained bool, now time.Time) []Effect {
	i := n - 1
	r := &c.relays[i]
	refuse := func(why string) []Effect {
		return c.evaluate(now, []Effect{{Kind: Refused, Relay: n, On: on, Reason: why}, {Kind: Publish, Relay: n}})
	}
	out := c.evaluate(now, nil) // settle silence and trips first so the command sees current state
	switch {
	case retained:
		return append(out, refuse(RefusedRetained)...)
	case !c.cfg.Active:
		return append(out, refuse(RefusedObserve)...)
	case on && r.Locked:
		return append(out, refuse(RefusedLocked)...)
	case c.silent && on && c.cfg.Rules[i].OnSilent == SilentOff,
		c.silent && !on && c.cfg.Rules[i].OnSilent == SilentOn:
		return append(out, refuse(RefusedSilent)...)
	}
	if !on {
		r.Locked = false
	}
	if r.On != on {
		r.On = on
		r.OnSince = time.Time{}
		if on {
			r.OnSince = now
		}
	}
	// Write even when unchanged: it is idempotent and repairs a failed earlier write.
	out = append(out, Effect{Kind: Write, Relay: n, On: on}, Effect{Kind: Publish, Relay: n})
	return out
}

// Observed takes a periodic readback of the chip and adopts what it sees.
// The exception keeps igor from ever leaving on a relay it believes off.
func (c *Controller) Observed(chip [N]bool, now time.Time) []Effect {
	var out []Effect
	for i, on := range chip {
		r := &c.relays[i]
		if r.On == on {
			continue
		}
		if c.cfg.Active {
			out = append(out, Effect{Kind: Drift, Relay: i + 1})
			if on {
				out = append(out, Effect{Kind: Write, Relay: i + 1, On: false})
				continue
			}
		}
		r.On = on
		r.OnSince = time.Time{}
		if on {
			r.OnSince = now
		}
		out = append(out, Effect{Kind: Publish, Relay: i + 1})
	}
	return c.evaluate(now, out)
}

func (c *Controller) link(now time.Time) {
	bad := !c.connected || c.haOffline
	if bad && !c.linkBad {
		c.linkBadSince = now
	}
	c.linkBad = bad
}

func (c *Controller) evaluate(now time.Time, out []Effect) []Effect {
	silent := (c.linkBad && now.Sub(c.linkBadSince) >= c.cfg.Grace) ||
		now.Sub(c.lastHeartbeat) >= c.cfg.HeartbeatTimeout
	if silent != c.silent {
		c.silent = silent
		out = append(out, Effect{Kind: Silence, On: silent})
	}
	if !c.cfg.Active {
		return out
	}
	for i := range c.relays {
		r, rule := &c.relays[i], c.cfg.Rules[i]
		switch {
		case !r.On:
			// A max_on trip during silence keeps the relay off: the lock wins.
			if silent && rule.OnSilent == SilentOn && !r.Locked {
				r.On, r.OnSince, r.LastFailsafe = true, now, ReasonSilent
				out = append(out,
					Effect{Kind: Failsafe, Relay: i + 1, On: true, Reason: ReasonSilent},
					Effect{Kind: Write, Relay: i + 1, On: true},
					Effect{Kind: Publish, Relay: i + 1})
			}
		case silent && rule.OnSilent == SilentOff:
			out = c.failsafe(i, ReasonSilent, out)
		case rule.MaxOn > 0 && now.Sub(r.OnSince) >= rule.MaxOn:
			out = c.failsafe(i, ReasonMaxOn, out)
		}
	}
	return out
}

func (c *Controller) failsafe(i int, reason string, out []Effect) []Effect {
	r := &c.relays[i]
	r.On, r.OnSince, r.LastFailsafe = false, time.Time{}, reason
	if reason == ReasonMaxOn {
		r.Locked = true
	}
	return append(out,
		Effect{Kind: Failsafe, Relay: i + 1, Reason: reason},
		Effect{Kind: Write, Relay: i + 1, On: false},
		Effect{Kind: Publish, Relay: i + 1})
}
