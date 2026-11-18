# HAOS card for `home`, built on a spare Pi 4 (2026-11-18)

The card for [swap `home` to HAOS](../how-to/swap-home-to-haos.md), built per [rebuild `home`](../how-to/rebuild-home.md) on a spare Pi 4 with a fanless PoE HAT and no UniPi board. Follows the [VM rehearsal](2026-11-16-haos-vm-rehearsal.md). Everything below measured 2026-11-18.

| Item | Value |
|---|---|
| Card | SanDisk Extreme Pro 64 GB; `f3`: 59.45 GB OK, 0 lost; 81.8 MB/s write, 90.5 MB/s read (Mac USB 3 reader) |
| Image | `haos_rpi4-64-18.5.img.xz`, 332 066 384 B, sha256 `779431180f4dbe4d2c33801e6bfe1d6fbff9ff4434f56a15282d0f23d45bdef8`; read-back of the 955 273 216 B raw image matched |
| Card state | HAOS 18.5, Supervisor 2026.11.0, Core 2025.9.4 (boot on, watchdog on), host name `home` |
| Config | backup `haos-card-2026-11-18` (manual, unencrypted, 00:47 CET), neutralised: SQLite recorder, 29 entries disabled; MQTT → `core-mosquitto` |
| Apps | Mosquitto 7.1.3, Terminal & SSH 10.6.0 (port 22), igor 2026.47.3 (watchdog), Tailscale 0.30.7 (node `home`, 100.64.173.70, key expiry off); all start on boot |
| Isolation | banacha VLAN 120 in the DMZ zone + host firewall; probes to Postgres, UNAS, `home` (LAN and tailnet), the house router and banacha's LANs all blocked before and after the restore |

## What the hardware showed that the VM could not

| Item | Result |
|---|---|
| Boot from the card | yes; Supervisor up and first Core installed 6 min after power-on |
| `CONFIG/` import | `i2c-dev` copied to `/etc/modules-load.d/` and `authorized_keys` active on boot 1; `CONFIG/` removed from the boot partition |
| `/dev/i2c-1` | absent on boot 1, present on boot 2 (`i2c_dev`, `i2c_bcm2835` loaded; `i2c@7e804000` `okay`) |
| `/dev/gpiochip0` | present on boot 1 |
| PoE HAT overlay | `pwm_raspberrypi_poe` and `pwm_fan` load from the explicit `dtoverlay=rpi-poe` |
| igor without a board | opens `/dev/i2c-1` under the default AppArmor profile, then exits 1: `mcp23008 read IODIR: sysfs-i2c: remote I/O error`. Also logs `open /dev/gpiochip1: operation not permitted` (its GPIO library opens every chip; only `gpiochip0` is mapped): noise |
| Store | repository `thaum-xyz/lancre` adds and igor appears (the quoted `invert` schema loads on a real Supervisor) |
| Restore timing | 58 s from `Partial-Restore start` to Core running (VM: 4.5 min) |
| First-boot Core | Supervisor installs the newest Core (2026.11.2) before anyone can pin a version |
| Downgrade 2026.11.2 → 2025.9.4 | crashes on the `.storage` files 2026.11.2 wrote (`data["minor_version"]`), Supervisor rolls back, repeatedly; works onto an empty config folder |
| MQTT after the restore | restored entry `127.0.0.1` without login refused (`not authorised`); after `post-restore-fix.sh` Core connects as `homeassistant` |
| `core_ssh` | port 22 not published until `network: {"22/tcp": 22}` |
| `ha` CLI | no `apps options`; options via the Supervisor API in `hassio_cli` |
| Raw disk on macOS | the card renumbered `disk6` → `disk4` between commands, and a Time Machine image took `disk6`; the flash script's size/USB/`Virtual: No` checks stopped the write |

## Only swap day can prove

| Item | Why not here |
|---|---|
| igor reads MCP23008, DS2482, MCP3422, the DIs; relays follow HA | no UniPi on the spare |
| Failsafes (relays 1, 6, 7 off when Core is silent) | same |
| PoE fan curve drives the fan; PoE power budget with the UniPi | fanless HAT, no UniPi |
| Restore of the swap-day backup onto the card at the house, with integrations enabled (inverter over RS485 `/dev/ttyUSB0`, Shelly, BleBox, phones) | all disabled or absent here |
| Recorder on Postgres from HAOS (no schema change: same Core) | blocked here by design |
| `hmi` panel against the new system | panel at the house |
| Tailscale from the house uplink; public `zmc`/`ssh-home` | no Cloudflared app on the card yet |
| ankhmorpork scraping igor and HA at `home` | ankhmorpork cannot reach the spare |
