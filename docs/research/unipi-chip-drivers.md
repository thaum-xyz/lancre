# UniPi 1.1 chip drivers for igor (Go)

Researched 2026-11-15 for [#4](https://github.com/thaum-xyz/lancre/issues/4). Versions and dates come from `proxy.golang.org/<module>/@latest` and the GitHub API, read 2026-11-15.

Short links used below:

| Alias | URL |
|---|---|
| `DEV` | https://github.com/periph/devices/blob/v3.7.4 |
| `CONN` | https://github.com/periph/conn/blob/v3.7.3 |
| `HOST` | https://github.com/periph/host/blob/v3.8.5 |
| `EVOK` | https://github.com/UniPiTechnology/evok/blob/2.4.34 |
| MCP3422 datasheet | https://ww1.microchip.com/downloads/en/DeviceDoc/22088c.pdf (DS22088C) |

## 1. Modules

| Module | Latest tag | Tag date | Default branch HEAD | Source |
|---|---|---|---|---|
| `periph.io/x/conn/v3` | v3.7.3 | 2026-04-07 | 2026-11-09, untagged | https://proxy.golang.org/periph.io/x/conn/v3/@latest |
| `periph.io/x/devices/v3` | v3.7.4 | 2025-03-16 | 2026-11-09, untagged | https://proxy.golang.org/periph.io/x/devices/v3/@latest |
| `periph.io/x/host/v3` | v3.8.5 | 2025-04-02 | 2026-11-09, untagged | https://proxy.golang.org/periph.io/x/host/v3/@latest |

None of the three repos is archived, and all three got commits on 2026-11-09 (https://api.github.com/repos/periph/devices). devices is 20 months past its last tag, so igor pins v3.7.4.

## 2. Coverage per chip

| Chip / function | Go package | Status | Source |
|---|---|---|---|
| MCP23008 @0x20 (relays) | `periph.io/x/devices/v3/mcp23xxx`, variant `mcp23xxx.MCP23008` | Exists. I2C addresses 0x20–0x27 only. Each pin implements `gpio.PinIO`, and `Dev.Group()` writes all 8 pins at once | `DEV/mcp23xxx/mcp23xxx.go#L61-L71`, `#L179-L201`, `DEV/mcp23xxx/group.go#L25` |
| MCP3422 @0x68 (AI1/AI2) | none | **No driver.** periph devices v3.7.4 and its 2026-11-09 HEAD have no `mcp342x`; their only ADC is `ads1x15`. pkg.go.dev search for `mcp3422` and `mcp342x` returns 0 packages. TinyGo drivers have no MCP342x either. It must be written against `i2c.Dev` (section 3) | https://github.com/periph/devices/tree/v3.7.4, https://pkg.go.dev/search?q=mcp3422&m=package, https://github.com/tinygo-org/drivers |
| 24C02 EEPROM @0x50 (AI coefficients) | none needed | No periph driver. Evok reads it through the kernel `at24` driver's sysfs file (section 4) | `EVOK/evok/config.py#L29-L58` |
| DS2482-100 @0x18 (1-Wire master) | `periph.io/x/devices/v3/ds248x` | Exists. Covers DS2482-100, DS2482-800 and DS2483. Implements `onewire.Bus`, including search | `DEV/ds248x/ds248x.go#L62-L80` |
| DS18B20 probes | `periph.io/x/devices/v3/ds18b20` | Exists: `New`, `ConvertAll`, `StartAll`, `LastTemp`, `Sense`. `SenseContinuous` is not implemented | `DEV/ds18b20/ds18b20.go#L42`, `#L71` |
| 12 DIs on GPIO 4,17,27,23,22,24,11,7,8,9,25,10 | `periph.io/x/host/v3/gpioioctl` (Linux GPIO character device, uAPI v2) | Exists. `host.Init()` registers it on Linux. Per-line `WaitForEdge` and `LineSet` reads of several lines at once. No debounce attribute (Evok runs `debounce = 0` anyway) | `HOST/gpioioctl/doc.go`, `HOST/host_linux.go#L9`, `HOST/gpioioctl/gpio.go#L224` |
| AO: PWM on GPIO18 | `periph.io/x/host/v3/bcm283x` `Pin.PWM` (possible, not recommended) | GPIO18 goes through the **DMA** path and needs `/dev/mem` (root). The source says "PWM0 alt5: disabled". periph has no `/sys/class/pwm` driver (`sysfs/` has gpio, i2c, led, spi and thermal only, also at HEAD). The likelier route is `dtoverlay=pwm` (default pin 18, Alt5) plus plain file writes to `/sys/class/pwm` | `HOST/bcm283x/gpio.go#L601-L630`, https://github.com/periph/host/tree/v3.8.5/sysfs, https://github.com/raspberrypi/linux/blob/rpi-6.12.y/arch/arm/boot/dts/overlays/README (section `pwm`) |

### Behaviour that matters for igor

| Package | Behaviour | Source |
|---|---|---|
| mcp23xxx | `NewI2C` only reads IODIR. It writes nothing, so relays keep their state across an igor restart. `Out()` first clears the IODIR bit, then read-modify-writes OLAT through a cache. Evok instead writes `IODIR=0x00` (all outputs) at startup | `DEV/mcp23xxx/mcp23xxx.go#L94-L124`, `DEV/mcp23xxx/pins.go#L153-L159`, `EVOK/evok/unipig.py#L80-L95` |
| mcp23xxx | IODIR and OLAT are cached after the first read. A second writer on 0x20 (for example Evok still running during cutover) makes the cache stale. `Read()` reads the GPIO register uncached | `DEV/mcp23xxx/registers.go#L80-L107`, `DEV/mcp23xxx/pins.go#L122-L128` |
| mcp23xxx | `WaitForEdge` is a TODO that always returns false. Not needed for relays | `DEV/mcp23xxx/pins.go#L130-L133` |
| ds248x | Every bus transaction polls the status register's 1WB bit: it sleeps `delay`, then polls every `delay/10`, with a **3 ms overall timeout**. A timeout is a *persistent* error, and the `Dev` must be re-created | `DEV/ds248x/ds248x.go#L272-L305`, `#L84-L93` |
| ds248x | Init sends Device Reset (0xF0) and expects status `0x18` (RST and LL). It writes config `0xE1` (APU on; the upper nibble is the complement of the lower one). It tells the variants apart by which read-pointer codes are ACKed | `DEV/ds248x/ds248x.go#L307-L372`; the Linux driver uses the same codes: https://github.com/torvalds/linux/blob/master/drivers/w1/masters/ds2482.c#L48-L95 |
| ds248x | Strong pull-up (SPU) is set before the last byte when `onewire.StrongPullup` is used. `ds18b20` uses it for Convert T | `DEV/ds248x/ds248x.go#L128-L162` |
| ds18b20 | Conversion wait is `94 << (bits-9)` ms, so **752 ms at 12 bit**. The Linux `w1_therm` uses 750 ms. `ConvertAll` (Skip ROM + 0x44) and then one `LastTemp` per probe costs one wait for all probes. `Sense()` costs one wait per probe | `DEV/ds18b20/ds18b20.go#L209`, https://github.com/torvalds/linux/blob/master/drivers/w1/slaves/w1_therm.c#L518-L551 |
| ds18b20 | `New(..., bits)` with a resolution that differs from the probe's writes the scratchpad with **TH=TL=0** and then Copy Scratchpad (0x48), an EEPROM write. Pass 12 (the factory default) to avoid the write | `DEV/ds18b20/ds18b20.go#L71-L104` |
| ds18b20 / onewire | The 9-byte scratchpad CRC is checked on every read. ROM CRC is checked during search. A reading of exactly 85 °C (the power-on value) is rejected as "no conversion" | `DEV/ds18b20/ds18b20.go#L158-L175`, `#L223`, `CONN/onewire/search.go#L105` |
| host/sysfs I2C | `Tx` uses `I2C_RDWR` (one ioctl, repeated start between write and read). It never calls `I2C_SLAVE`, so the kernel's `EBUSY` check for addresses bound to a kernel driver (such as `at24` on 0x50) does not apply | `HOST/sysfs/i2c.go#L92-L127`, https://github.com/torvalds/linux/blob/master/drivers/i2c/i2c-dev.c#L405-L416 |

## 3. MCP3422: register sequence over raw I2C

Datasheet facts (DS22088C):

| Item | Value | Page |
|---|---|---|
| Config byte | `RDY C1 C0 O/C S1 S0 G1 G0`. RDY write=1 starts a one-shot conversion; RDY read=0 means a new result. C=00 ch1, 01 ch2. O/C=0 one-shot. S=11 18 bit / 3.75 SPS. G=00 ×1 | p.18, Register 5-1 |
| 18-bit read | 3 data bytes `MMMMMMD17D16, D15..D8, D7..D0`, then the config byte (M = repeated sign) | p.22 |
| Code format | two's complement, range −131072..131071 | p.15, Tables 4-2/4-3 |
| LSB | `2·VREF / 2^N` = 15.625 µV at 18 bit, VREF = 2.048 V | p.15, Eq. 4-3, Table 4-1 |
| I2C address | `1101 A2 A1 A0` (0x68 with A2..A0=000) | p.20 |

Sequence that matches Evok's one-shot mode (`EVOK/evok/unipig.py#L279-L300`, `#L365-L399`):

| Step | I2C op (addr 0x68) | AI1 (ch0) | AI2 (ch1) |
|---|---|---|---|
| 1. Start one-shot | write 1 byte | `0x8C` | `0xAC` |
| 2. Wait | — | ≥ 1/3.75 s = 266.7 ms | same |
| 3. Read | read 4 bytes `b0 b1 b2 cfg` | — | — |
| 4. Check | `cfg & 0x80 == 0` (ready) and `cfg & 0x7F == mode & 0x7F` (`0x0C` / `0x2C`) | else retry | |
| 5. Decode | `code = sign-extend((b0<<16 \| b1<<8 \| b2) & 0x3FFFF, 18)` | | |

In Go: `d := &i2c.Dev{Bus: bus, Addr: 0x68}; d.Tx([]byte{0x8C}, nil); time.Sleep(267*time.Millisecond); d.Tx(nil, buf[:4])`. A write and a read must be separate `Tx` calls, because one combined `Tx` would put a repeated start between them.

## 4. Evok 2.x: MCP3422 sample to volts

Source version: tag `2.4.34` (commit `b64ac03`, 2023-08-16), the last 2.4.x release with the native UniPi 1.1 I2C code (`evok/unipig.py`). From tag `2.5.0` (2023-11-16) on, `unipig.py` is gone and UniPi 1.1 AIs are read as Modbus registers (`etc/hw_definitions/UNIPI11.yaml`, https://github.com/UniPiTechnology/evok/blob/2.5.0/etc/hw_definitions/UNIPI11.yaml). The conversion for that path is not in this repo and was not researched.

**Raw to ADC volts** (`EVOK/evok/unipig.py#L241-L246`, `#L391-L398`):

```python
value = 0
for i in range(readlen - 1): value = (value << 8) | data[i]
bits = (status & 0x0c) >> 2
sign = BIT_MASKS[bits][1] & value      # 18 bit: 0b100000000000000000
value &= BIT_MASKS[bits][0]            # 18 bit: 0b11111111111111111 = 131071
if sign: value = -value
self.lastmeasure._set_voltage(2.048 / BIT_MASKS[bits][0] * value)
```

**Coefficient and output** (`EVOK/evok/unipig.py#L423`, `#L455-L457`):

```python
self.koef = self.correction / self.gain
...
if raw_value < 0: raw_value = 0
self.value = raw_value * self.koef
```

**Formula** (18 bit, gain 1):

```
V = max(0, 2.048 · (code & 0x1FFFF) / 131071) · correction / gain
```

| Detail | Effect | Source |
|---|---|---|
| Divides by 131071 (max code) where the datasheet LSB divides by 131072 | +7.6 ppm vs datasheet; reproduce it to match Evok exactly | `EVOK/evok/unipig.py#L398`; DS22088C p.15 |
| Negative codes are treated as sign-magnitude, not two's complement | Irrelevant: negatives are clamped to 0 V | `EVOK/evok/unipig.py#L395-L397`, `#L456` |
| `correction` ≈ 5.56 is effectively the input divider ratio (10 V → ~1.8 V at the ADC) | Default when the EEPROM is unusable: `5.564920867` | `EVOK/evok/config.py#L17-L24`, `#L346` |

**Where `correction` comes from.** This is the path the shipped config actually takes, which is not the `corr_rom` path:

1. `read_eprom_config()` reads `/sys/bus/i2c/devices/1-0050/eeprom` (256 bytes, exposed by the `at24` kernel driver that `modprobe unipi_eprom` binds as a 24c02 at 0x50). See `EVOK/evok/config.py#L29-L58` and `EVOK/etc/modprobe.d/unipi.conf#L4-L6`.
2. In the shipped `evok-unipi1.1.conf`, `[EPROM_1]` is commented out ("eprom should be handled by kernel module"). `Devices.by_int(EE, corr_rom)` therefore raises, and the `except:` branch builds the AI **without** `rom`. `corr_rom`/`corr_addr` are dead config. See `EVOK/etc/evok-unipi1.1.conf#L34`, `EVOK/evok/config.py#L337-L358`.
3. `correction` = an explicit `correction =` in the config if set, else `up_globals['devices']['ai'][circuit]` from step 1, else the default `5.564920867`.

**EEPROM layout** (`EVOK/evok/config.py#L36-L55`):

| Offset | Bytes | Meaning | Encoding |
|---|---|---|---|
| 0xE0–0xE1 | 2 | magic `FA 55`; layout valid only if present | raw |
| 0xE2–0xE3 | 2 | board version: `01 01` = UniPi 1.1, `0B 01` = UniPi Lite 1.1, else 1.0 | raw |
| 0xE4–0xE7 | 4 | serial | `struct 'i'` = native int32 (little-endian on the Pi) |
| 0xF0–0xF3 | 4 | AI1 correction | `struct '!f'` = **big-endian IEEE-754 float32** |
| 0xF4–0xF7 | 4 | AI2 correction | big-endian float32 |

Coefficients are only read for "UniPi 1.1" and "UniPi 1.0"; Lite boards get 0. The unused `corr_rom` path (`EVOK/evok/unipig.py#L425-L441`) also decodes `!f` and falls back on `0` or NaN. The sysfs path used in practice has **no** 0/NaN guard. Evok's writer `set_adc_coef.py` packs the float as a uint32 and writes its hex digits MSB first, so it is big-endian too (`EVOK/set_adc_coef.py#L28-L31`).

For igor: read the sysfs file like Evok (`os.ReadFile("/sys/bus/i2c/devices/1-0050/eeprom")`, `math.Float32frombits(binary.BigEndian.Uint32(b[0xF0:0xF4]))`), and check the magic plus `!= 0 && !NaN` before use. Raw I2C also works (`Tx([]byte{0xF0}, r[:8])`, 1-byte word address on a 24C02), but it bypasses the bound `at24` driver (section 2, host/sysfs I2C), so it competes with the driver for the chip.

## 5. Test fakes

| Fake | Package | What it does | Limits | Source |
|---|---|---|---|---|
| I2C | `periph.io/x/conn/v3/i2c/i2ctest` | `Playback{Ops []IO}`. Each `Tx` must match the next `IO{Addr, W, R}` exactly (addr, write bytes, read length), then copies `R`. `Close()` fails if ops remain. `Record` wraps a real bus to capture sequences on the Pi | Strictly ordered script with no register model; a retry loop or an extra read breaks the script. Panics unless `DontPanic: true` | `CONN/i2c/i2ctest/i2ctest.go#L97-L142` |
| 1-Wire | `periph.io/x/conn/v3/onewire/onewiretest` | Same `Record`/`Playback` model for `onewire.Bus` (`Tx(w, r, pull)`), so `ds18b20` can be tested without faking the DS2482 | Same as above | `CONN/onewire/onewiretest/onewiretest.go#L88-L115` |
| GPIO | `periph.io/x/conn/v3/gpio/gpiotest` | `Pin{N, Num, L, EdgesChan, Clock}`. Set `L` to fake a level and push on `EdgesChan` to fake an edge. Takes an injectable `clockwork.Clock` | Models one pin, not a `gpioioctl.LineSet` | `CONN/gpio/gpiotest/gpiotest.go#L24-L131` |

Verdict: `i2ctest.Playback` is enough for igor's own MCP3422 and EEPROM code and for wire-level checks of the periph drivers. Tested 2026-11-15 against v3.7.4: a Playback script of IODIR read → `{0x00,0x7F}` → OLAT read → `{0x0A,0x80}` passed for "relay 1 on" (MCP23008 pin 7). Logic above the bus (debounce, scheduling) tests better behind igor's own small interfaces. Two catches:

- `ds18b20.ConvertAll` and `ds248x.waitIdle` use a package-private `sleep = time.Sleep`, so tests that go through them **really sleep** (752 ms per 12-bit conversion) (`DEV/ds18b20/ds18b20.go#L209`).
- `ds248x` takes a real `time.Now()` 3 ms deadline.

## 6. Build: cgo-free, armv7 and arm64

Verified 2026-11-15 with go1.26.4, building a program that imports `host`, `i2creg`, `mcp23xxx`, `ds248x` and `ds18b20` at the versions above:

| Target | Command | Result |
|---|---|---|
| arm64 (64-bit Pi OS) | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build` | ok |
| armv7 (32-bit Pi OS) | `CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build` | ok |

`host` v3.8.5 has no `import "C"`. The cgo-based FTDI driver is commented out of `host.go` (`HOST/host.go#L11`). `host_arm.go` and `host_arm64.go` register different SoC drivers, and `bcm283x` is in both (`HOST/host_arm.go`, `HOST/host_arm64.go`).

## Open points

| Point | Why uncertain |
|---|---|
| Which Evok igor actually replaces (2.4.x native I2C vs ≥2.5.0 Modbus) | Not recorded in this repo (the removed `docs/evok.md` did not say). Check `/opt/evok` or `version.txt` on the Pi before trusting section 4 |
| `bcm283x` DMA PWM on BCM2711 (Pi 4) | Not tested. It needs root and `/dev/mem`. The kernel `pwm` overlay avoids it |
| Evok's AO frequency | The 2.4.34 config has `frequency = 100` (`EVOK/etc/evok-unipi1.1.conf#L103-L105`); the removed `docs/evok.md` said 400 Hz. Evok drives it with pigpio `set_PWM_*` (pigpio PWM, range 1000, duty = V×100 on 1.1, inverted on 1.0) (`EVOK/evok/unipig.py#L671-L700`) |
| DS2482-100 and DS18B20 datasheets | analog.com timed out on 2026-11-15. Command codes and timings above are cross-checked against the periph and Linux kernel drivers, not the PDFs |
| DI polarity | Evok sets `PUD_UP` and reports the raw level (`EVOK/evok/unipig.py#L756`, `EVOK/evok/apigpio.py#L165-L188`). Whether 1 means "input active" depends on the board's optocoupler stage, which was not checked |
