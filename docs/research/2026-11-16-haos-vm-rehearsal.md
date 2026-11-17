# HAOS VM rehearsal and upgrade to Core 2026.11.1 (2026-11-16)

The visit's restore ([plan](https://github.com/thaum-xyz/lancre/issues/15), phase 4 step 2) rehearsed in a HAOS VM, then Core taken from 2025.9.4 to 2026.11.1. Everything below was measured on 2026-11-16 in that VM.

| Item | Value |
|---|---|
| VM | QEMU 11.1.3 (Homebrew, `hvf`), 4 vCPU, 4 GiB, on an Apple Silicon Mac |
| Image | `haos_generic-aarch64-18.4.qcow2.xz` (stable channel, sha256 matches the release digest); Supervisor 2026.11.0 |
| Backup | `migracja` (partial, Core 2025.9.4, no database, unencrypted, 2.3 MB), taken on `home` 2026-11-16 15:31 CET, after the Postgres move and the Evok repoint |
| Isolation | restore with QEMU `restrict=on` (no outbound at all); afterwards outbound allowed only through a VM firewall dropping 192.168.0.0/16 and 100.64.0.0/10, proven by probes before and after (Postgres 192.168.50.137:5432, `home` over Tailscale and the UNAS were open before, dropped after) |
| Neutralised before any internet | recorder `db_url` commented out (SQLite); 28 config entries disabled: phones (`mobile_app` ×7), inverter, LAN devices, UniFi Protect, cloud APIs; MQTT pointed at the VM's own Mosquitto |

## Restore

| Step | Result | Visit action |
|---|---|---|
| Upload + restore at onboarding | Works; 4.5 min until Core runs (offline, integrations timing out) | none |
| Restore with no internet | Refused: `do_restore_partial blocked … no supervisor internet connection` | have internet at the restore |
| Core version in backup ≠ installed | Supervisor pulls the backup's Core even if the image is local; offline the pull fails and it starts the newer Core on the old config | expect a 3 GB pull of 2025.9.4 at the restore |
| Users, dashboards, scheduler, custom components | Restored; owner `admin`, Bożena, Andrzej present; dashboard and scheduler card render | none |
| `http: server_host: 0.0.0.0` | Core: `incompatible HTTP option 'server_host'. Watchdog feature disabled`; Supervisor's Core watchdog was `false` after the restore | delete `server_host`, then `ha core options --watchdog` |
| `http: trusted_proxies` | Only `127.0.0.1`, `::1`; HAOS apps reach Core from `172.30.33.0/24` | add `172.30.33.0/24` before Cloudflared (the line is already there, commented) |
| MQTT entry (`127.0.0.1`, no user) | Mosquitto app: `not authorised` | MQTT → Reconfigure: broker `core-mosquitto`, a Mosquitto login (HA user or app login) |
| `huawei_solar` requirement `huawei-solar==2.4.7` | Installs from the wheels index at first start (needs internet) | none |
| Old SQLite rollback DB | Not in the backup (separate mount on `home`) | none |

## igor app

| Step | Result |
|---|---|
| Add repository `https://github.com/thaum-xyz/lancre` | Repository adds; **igor does not appear**: Supervisor rejects `haos/igor/config.yaml`, `data['schema']['digital_inputs']['invert'][0]. Got 'int(1'` |
| Cause | `invert: [int(1,12)]` is a YAML flow list; the comma splits it into `'int(1'` and `'12)'` |
| Fix (this branch) | `invert: ["int(1,12)"]`; loaded as a local app it validates and installs |
| Install | Pulls `ghcr.io/thaum-xyz/igor:2026.47.2`; default options pass the schema |
| MQTT service | `broker=tcp://core-mosquitto:1883` taken from the Mosquitto app; the login itself is not reached (next row) |
| No I2C in the VM | Supervisor starts it without the missing devices; igor exits 1: `open I2C bus 1: i2creg: no bus found; did you forget to call Init()?` |
| Clarity of that failure | Fatal and logged, but the text is periph.io's; it does not say `/dev/i2c-1` is missing. Repair `issue_addon_boot_fail` appears |
| Watchdog | Off after install; the how-to's step 4 turns it on |

## Upgrade 2025.9.4 → 2026.11.1 (one step)

`ha core update --version 2026.11.1`: 39 s, no rollback. Snapshot `pre-upgrade-2025.9.4` taken before.

| Breakage | Effect | Fix | Works on 2025.9.4 too |
|---|---|---|---|
| Legacy `sensor: - platform: template` rejected (repair `platform_config_not_supported`) | `sensor.temperature_inside` gone; `climate.heating` current temperature `None`, so it never calls for heat | `template:` block below; tested: BleBox wins when non-zero, UniPi probe otherwise, thermostat follows | yes (`check_config` 2025.9.4) |
| `http:` YAML migrated into `.storage/http` (repair `yaml_still_present_after_migration`, breaks in 2027.2) | none yet | delete the `http:` block after the upgrade; edit HTTP settings in the UI | n/a, only after the upgrade |
| Recorder schema 50 → 53 | automatic, 50 ms on the VM's small SQLite; will run once on the production Postgres at the real upgrade | none; take a DB copy first | n/a |
| `scheduler-component`, `gne_pv_monitoring` use `CONN_CLASS_*_POLL` | warning; removed in 2027.5 | update both before 2027.5 | n/a |
| Port | restored `http:` keeps 8123; port 80 is only a fresh-install default | none | n/a |

```yaml
template:
  - sensor:
      - name: temperature_inside
        unique_id: temperature_inside
        device_class: temperature
        state_class: measurement
        unit_of_measurement: "°C"
        state: >-
          {% set b = states('sensor.blebox') | float(0) %}
          {{ b if b != 0 else states('sensor.unipi_temp_28ff89db06000034') }}
        availability: >-
          {{ states('sensor.blebox') | float(0) != 0
             or has_value('sensor.unipi_temp_28ff89db06000034') }}
```

Loaded without breakage after the upgrade: 4 `generic_thermostat`, 16 automations, `rest` (the SystemAir resource and the dead Evok ones), `trend`, `scheduler`, `min_max`, MQTT, the storage dashboard (8 views, 0 card errors, 0 dead entity references) with `custom:scheduler-card` v4.0.5.

## Already broken on 2025.9.4 (not the upgrade)

| Item | Fix |
|---|---|
| `map:` (`Integration 'map' not found` on both versions) | delete the line |
| `[Energy Bypass] Turn ON/OFF` trigger: `int got invalid input 'unavailable'` | `\| int(0)` on `sensor.powermeter_total_active_power` |
| `sensor.temperature_inside` errors whenever BleBox is `unavailable` (`float` without default) | the `template:` block above |
| Evok `rest` sensors/switches, `rest_command.thermostat_expected` | retire-Evok step 6 |

## Re-run with backup `post-evok` (2026-11-16 17:55 CET)

Same VM, reverted to the 2025.9.4 snapshot, restored offline with `ha backups restore --homeassistant` (4.5 min), neutralised as above (29 entries disabled, now including the UNAS DLNA server), upgraded with `ha core update --version 2026.11.1` (41 s, no rollback).

| Check | `migracja` (15:31) | `post-evok` (17:55) |
|---|---|---|
| Legacy template sensor | breaks `climate.heating` on 2026.11 | already `template:`; `sensor.temperature_inside` loads on both versions |
| `map:` | invalid on both | removed |
| Evok `rest`, `rest_command`, dead helpers | errors on both | removed |
| `[Energy Bypass]` template errors | on both | gone (replaced by `[energia] Grzałka według eksportu PV`) |
| `CONN_CLASS_*_POLL` deprecations (`scheduler`, `gne_pv_monitoring`) | warned | not logged |
| `http:` YAML (`server_host`, `trusted_proxies`) | present | **still present**: watchdog disabled on 2025.9.4, repair `yaml_still_present_after_migration` on 2026.11 |
| Errors on 2026.11.1 other than unreachable hosts | template platform | none |
| Automations | 16 load | 8 load, 0 unavailable |
| Dashboard | 0 card errors, 0 dead references | same views and cards, 0 dead references |

Behaviour on 2026.11.1, with test states set through the API:

| Input | Result |
|---|---|
| hall probe 18.0 °C, preset `home` (19.5) | `climate.heating` heating, `input_boolean.room_heat_demand` on |
| hall probe 21.0 °C | idle, demand off |
| `input_boolean.away_mode` on / off | heating `away` 16 / back to `home` 19.5; water `away` 35 / back to `comfort` 50 |
| `[piec] Palnik według zapotrzebowania` | stops at its conditions (no igor in the VM); no trace errors |

Left for the upgrade: delete the `http:` block after it migrates, set HTTP options in the UI (`trusted_proxies` + `172.30.33.0/24`), and turn on the Core watchdog.

## Not tested in the VM

| What | Why | Where it gets tested |
|---|---|---|
| igor on the board (I2C, GPIO, MQTT login, failsafes) | no UniPi | visit, issue table tests 1-9 |
| Inverter, Shelly, BleBox, ESPHome, Bravia, Android TV, UniFi Protect data | disabled for isolation | visit, or after the upgrade on the Pi |
| `gne_pv_monitoring`, Electricity Maps, Forecast.Solar | disabled (cloud accounts) | after the upgrade |
| Recorder migration on Postgres 30-day data | VM uses SQLite | the real upgrade, after a DB copy |
| Cloudflared tunnel, Tailscale app, phones' notifications | must not run from a copy | visit step 4 |
| Scheduler card edit dialog | browser pane hidden at the time | dashboard work in the VM |
| Chromium 92 (`hmi`) against 2026.11 frontend | no old browser here (user's call) | after the `hmi` reimage |
