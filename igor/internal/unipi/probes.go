package unipi

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/onewire"
	"periph.io/x/devices/v3/ds18b20"
	"periph.io/x/devices/v3/ds248x"
)

// rescanEvery is how many reads pass between 1-Wire bus searches.
const rescanEvery = 10

const familyDS18B20 = 0x28

// Probes reads every DS18B20 on the 1-Wire bus behind the DS2482. It only
// ever converts and reads: a probe's EEPROM is never written.
type Probes struct {
	open  func() (onewire.BusSearcher, error)
	bus   onewire.BusSearcher
	known []onewire.Address
	reads int
}

func NewProbes(bus i2c.Bus) *Probes {
	return newProbes(func() (onewire.BusSearcher, error) { return ds248x.New(bus, addrOneWir, nil) })
}

func newProbes(open func() (onewire.BusSearcher, error)) *Probes {
	return &Probes{open: open}
}

// Read converts all probes at once (about 0.75 s) and returns °C by probe
// address. A probe with a failed read is left out. The error is non-nil
// when the bus itself failed or any probe did.
func (p *Probes) Read() (map[string]float64, error) {
	if p.bus == nil {
		bus, err := p.open()
		if err != nil {
			return nil, fmt.Errorf("ds2482: %w", err)
		}
		p.bus, p.reads = bus, 0
	}
	if p.reads%rescanEvery == 0 {
		if err := p.scan(); err != nil {
			return nil, p.fail(err)
		}
	}
	p.reads++
	if len(p.known) == 0 {
		return map[string]float64{}, nil
	}
	if err := ds18b20.ConvertAll(p.bus, 12); err != nil {
		return nil, p.fail(fmt.Errorf("ds18b20 convert: %w", err))
	}
	out := make(map[string]float64, len(p.known))
	var bad []string
	for _, addr := range p.known {
		t, err := p.temperature(addr)
		if err != nil {
			bad = append(bad, AddressString(addr)+": "+err.Error())
			continue
		}
		out[AddressString(addr)] = t
	}
	if bad != nil {
		return out, errors.New(strings.Join(bad, "; "))
	}
	return out, nil
}

// scan adds newly found probes. A probe that stops answering stays known,
// so it keeps its entity and simply reports nothing.
func (p *Probes) scan() error {
	addrs, err := onewire.Search(p.bus, false)
	if err != nil {
		return fmt.Errorf("1-wire search: %w", err)
	}
	for _, a := range addrs {
		if byte(a) != familyDS18B20 {
			continue
		}
		i := sort.Search(len(p.known), func(i int) bool { return p.known[i] >= a })
		if i < len(p.known) && p.known[i] == a {
			continue
		}
		p.known = append(p.known, 0)
		copy(p.known[i+1:], p.known[i:])
		p.known[i] = a
	}
	return nil
}

func (p *Probes) temperature(addr onewire.Address) (float64, error) {
	var spad [9]byte
	dev := onewire.Dev{Bus: p.bus, Addr: addr}
	if err := dev.Tx([]byte{0xBE}, spad[:]); err != nil {
		return 0, err
	}
	// Nine zero bytes (a bus held low) carry a valid CRC, so reject them too.
	if !onewire.CheckCRC(spad[:]) || spad == [9]byte{} {
		return 0, errors.New("bad scratchpad")
	}
	raw := int16(spad[1])<<8 | int16(spad[0])
	// Below 12 bit the low bits are undefined (datasheet p.6).
	raw &^= 1<<(3-uint(spad[4]>>5&3)) - 1
	if raw == 85*16 {
		return 0, errors.New("power-on value, no conversion happened")
	}
	if c := float64(raw) / 16; c >= -55 && c <= 125 {
		return c, nil
	}
	return 0, errors.New("reading outside the sensor's range")
}

// fail drops the DS2482 handle unless the error came from the 1-Wire side:
// the driver refuses all further use after a chip or I2C error.
func (p *Probes) fail(err error) error {
	var wire onewire.BusError
	if !errors.As(err, &wire) {
		p.bus = nil
	}
	return err
}

// AddressString formats a probe address the way Evok does: ROM bytes in
// wire order, family code first, e.g. 28FF4C30041503A7.
func AddressString(a onewire.Address) string {
	var b strings.Builder
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&b, "%02X", byte(a>>(8*i)))
	}
	return b.String()
}
