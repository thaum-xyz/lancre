package rules

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// The live rule set (lancre#7): relay 1 valve, 4 circulation, 6/7 saunas.
func liveConfig(active bool) Config {
	c := Config{Active: active, HeartbeatTimeout: 10 * time.Minute, Grace: 2 * time.Minute}
	c.Rules[0] = Rule{OffOnSilent: true}
	c.Rules[3] = Rule{MaxOn: 2 * time.Minute}
	c.Rules[5] = Rule{OffOnSilent: true, MaxOn: 3 * time.Hour, OffOnStart: true}
	c.Rules[6] = c.Rules[5]
	return c
}

// house drives a Controller the way the bridge does: heartbeats every
// minute while HA is healthy, a tick every 10 s.
type house struct {
	t         *testing.T
	c         *Controller
	now       time.Time
	heartbeat bool
	effects   []Effect
}

func newHouse(t *testing.T, active bool, chip [N]bool) *house {
	h := &house{t: t, now: t0, heartbeat: true}
	var e []Effect
	h.c, e = New(liveConfig(active), chip, h.now)
	h.effects = append(e, h.c.Connected(h.now)...)
	return h
}

func (h *house) wait(d time.Duration) {
	for end := h.now.Add(d); h.now.Before(end); {
		h.now = h.now.Add(10 * time.Second)
		h.effects = append(h.effects, h.c.Tick(h.now)...)
		if h.heartbeat && h.now.Sub(t0)%time.Minute == 0 {
			h.effects = append(h.effects, h.c.Heartbeat(h.now)...)
		}
	}
}

func (h *house) cmd(n int, on bool) []Effect {
	e := h.c.Command(n, on, false, h.now)
	h.effects = append(h.effects, e...)
	return e
}

func (h *house) want(states string) {
	h.t.Helper()
	got := ""
	for n := 1; n <= N; n++ {
		switch r := h.c.Relay(n); {
		case r.On:
			got += "1"
		case r.Locked:
			got += "L"
		default:
			got += "-"
		}
	}
	if got != states {
		h.t.Fatalf("relays = %s, want %s", got, states)
	}
}

func count(effects []Effect, k Kind, reason string) int {
	n := 0
	for _, e := range effects {
		if e.Kind == k && (reason == "" || e.Reason == reason) {
			n++
		}
	}
	return n
}

func TestForgottenSaunaTripsAndLocks(t *testing.T) {
	h := newHouse(t, true, [N]bool{})
	h.cmd(7, true)
	h.wait(3*time.Hour - 10*time.Second)
	h.want("------1-")
	h.wait(10 * time.Second)
	h.want("------L-")
	if h.c.Relay(7).LastFailsafe != ReasonMaxOn {
		t.Fatalf("last failsafe = %q", h.c.Relay(7).LastFailsafe)
	}
	if e := h.cmd(7, true); count(e, Refused, RefusedLocked) != 1 || count(e, Write, "") != 0 {
		t.Fatalf("on while locked: %+v", e)
	}
	h.cmd(7, false)
	h.cmd(7, true)
	h.want("------1-")
}

func TestRepeatedOnDoesNotExtendMaxOn(t *testing.T) {
	h := newHouse(t, true, [N]bool{})
	h.cmd(4, true)
	h.wait(time.Minute)
	h.cmd(4, true)
	h.wait(time.Minute)
	h.want("---L----")
}

func TestHADiesValveAndSaunaDropAfterGrace(t *testing.T) {
	h := newHouse(t, true, [N]bool{})
	h.cmd(1, true)
	h.cmd(5, true)
	h.cmd(7, true)
	h.heartbeat = false
	h.effects = append(h.effects, h.c.HAStatus(false, h.now)...)
	h.wait(time.Minute + 50*time.Second)
	h.want("1---1-1-")
	h.wait(10 * time.Second)
	h.want("----1---")
	if !h.c.Silent() || h.c.Relay(7).Locked {
		t.Fatal("want silent, sauna not locked")
	}
	h.heartbeat = true
	e := h.c.HAStatus(true, h.now)
	if count(e, Announce, "") != 1 || h.c.Silent() {
		t.Fatalf("HA back: %+v silent=%v", e, h.c.Silent())
	}
	h.cmd(1, true)
	h.want("1---1---")
}

func TestShortBrokerOutageMovesNothing(t *testing.T) {
	h := newHouse(t, true, [N]bool{})
	h.cmd(7, true)
	h.effects = nil
	h.c.Disconnected(h.now)
	h.heartbeat = false
	h.wait(30 * time.Second)
	h.heartbeat = true
	h.effects = append(h.effects, h.c.Connected(h.now)...)
	h.want("------1-")
	if count(h.effects, Write, "") != 0 || count(h.effects, Announce, "") != 1 {
		t.Fatalf("effects: %+v", h.effects)
	}
}

func TestBrokenHeartbeatAutomation(t *testing.T) {
	h := newHouse(t, true, [N]bool{})
	h.cmd(1, true)
	h.cmd(7, true)
	h.heartbeat = false
	h.wait(10 * time.Minute)
	h.want("--------")
	if e := h.cmd(7, true); count(e, Refused, RefusedSilent) != 1 {
		t.Fatalf("sauna on while silent: %+v", e)
	}
	h.cmd(4, true) // a hold relay still obeys
	h.want("---1----")
	h.heartbeat = true
	h.wait(time.Minute)
	if h.c.Silent() {
		t.Fatal("still silent after a heartbeat")
	}
	h.cmd(7, true)
	h.want("---1--1-")
}

func TestStartKeepsRelaysExceptOffOnStart(t *testing.T) {
	h := newHouse(t, true, [N]bool{3: true, 4: true, 6: true})
	h.want("---11---")
	if count(h.effects, Failsafe, ReasonStart) != 1 {
		t.Fatalf("effects: %+v", h.effects)
	}
	h.wait(2 * time.Minute) // the pump's allowance starts at igor's start
	h.want("---L1---")
}

func TestObserveNeverWrites(t *testing.T) {
	h := newHouse(t, false, [N]bool{6: true})
	h.want("------1-") // off_on_start is not applied
	h.effects = append(h.effects, h.c.Observed([N]bool{4: true, 6: true}, h.now)...)
	h.want("----1-1-")
	if e := h.cmd(5, false); count(e, Refused, RefusedObserve) != 1 {
		t.Fatalf("command in observe: %+v", e)
	}
	h.heartbeat = false
	h.wait(4 * time.Hour)
	h.want("----1-1-")
	if !h.c.Silent() || count(h.effects, Write, "") != 0 {
		t.Fatalf("observe wrote to the chip or missed silence: %+v", h.effects)
	}
}

func TestRetainedCommandIsIgnored(t *testing.T) {
	h := newHouse(t, true, [N]bool{})
	e := h.c.Command(7, true, true, h.now)
	h.want("--------")
	if count(e, Refused, RefusedRetained) != 1 {
		t.Fatalf("effects: %+v", e)
	}
}

func TestActiveReassertsWhenChipDrifts(t *testing.T) {
	h := newHouse(t, true, [N]bool{})
	h.cmd(5, true)
	e := h.c.Observed([N]bool{}, h.now) // the write was lost
	if len(e) != 2 || e[0].Kind != Drift || e[1] != (Effect{Kind: Write, Relay: 5, On: true}) {
		t.Fatalf("effects: %+v", e)
	}
	h.want("----1---")
}

func TestOffClearsLockEvenWhenAlreadyOff(t *testing.T) {
	h := newHouse(t, true, [N]bool{})
	h.cmd(4, true)
	h.wait(2 * time.Minute)
	h.want("---L----")
	if e := h.cmd(4, false); count(e, Write, "") != 1 {
		t.Fatalf("off must still be written: %+v", e)
	}
	h.want("--------")
}

func TestHeartbeatOverridesStaleOffline(t *testing.T) {
	h := newHouse(t, true, [N]bool{})
	h.cmd(1, true)
	h.c.HAStatus(false, h.now)
	h.wait(time.Minute) // heartbeats keep coming: HA is clearly alive
	h.wait(5 * time.Minute)
	h.want("1-------")
}
