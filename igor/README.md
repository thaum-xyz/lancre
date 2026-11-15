# igor

Connects the UniPi 1.1 board on `home` to Home Assistant over MQTT. It replaces Evok.

igor is a connector. It holds no control logic and does not know what a relay drives; the only decisions it makes are the per-relay failsafes in its config file. Terms are in the repo's `GLOSSARY.md`.

| Decision | Where it was made |
|----------|-------------------|
| Chips and drivers | [docs/research/unipi-chip-drivers.md](../docs/research/unipi-chip-drivers.md) |
| Failsafe rules | [lancre#7](https://github.com/thaum-xyz/lancre/issues/7) |
| Config file and MQTT contract | [lancre#9](https://github.com/thaum-xyz/lancre/issues/9) |

## Layout

| Path | Holds |
|------|-------|
| `internal/rules` | Relay decisions: commands, `on_silent`, `max_on`, `off_on_start`. Pure, no I/O. |
| `internal/unipi` | Chip drivers and the board's fixed wiring; a fake board for laptops. |
| `internal/bridge` | Polls the board, carries out the rule engine's decisions, publishes, exports metrics. |
| `internal/hass` | Topic names and discovery payloads. |
| `internal/config` | Config file loading. Reference: [config.example.yaml](config.example.yaml). |
| `docker-compose.yaml`, `dev/` | Laptop stack: fake board, open broker, throwaway Home Assistant. |
| `deploy/home/` | Stack for `home`: broker with passwords, igor in `observe` mode. |

## Modes

| Mode | Relay chip | Commands | Failsafes |
|------|-----------|----------|-----------|
| `observe` | read only, follows what Evok does; input pull-ups left as found | refused and counted | not applied |
| `active` | owned by igor; a differing chip is written again | obeyed | applied |

The relay chip latches. Relays keep their state while igor is stopped or restarting, and igor reads them back at start.

## Entities in Home Assistant

Entity ids follow the terminal; set the friendly name in Home Assistant.

| Terminal | Entity id | Notes |
|----------|-----------|-------|
| Relay 1-8 | `switch.unipi_relay_N` | attributes `locked`, `last_failsafe` |
| DI 1-12 | `binary_sensor.unipi_di_N` | on = terminal driven; a GPIO the kernel refuses gets no entity |
| AI 1-2 | `sensor.unipi_ai_N` | volts; unavailable 60 s after the last good reading |
| DS18B20 | `sensor.unipi_temp_<address>` | found by bus search; unavailable 120 s after the last good reading |

## Topics

Prefix `igor`, QoS 1.

| Topic | Direction | Payload | Retained |
|-------|-----------|---------|----------|
| `igor/status` | igor → HA | `online` / `offline` (last will) | yes |
| `igor/relay/N/state` | igor → HA | `ON` / `OFF` | yes |
| `igor/relay/N/attributes` | igor → HA | `{"locked":bool,"last_failsafe":null\|"max_on"\|"silent"\|"start"}` | yes |
| `igor/relay/N/set` | HA → igor | `ON` / `OFF`; a retained message is ignored | never |
| `igor/di/N/state` | igor → HA | `ON` / `OFF` | yes |
| `igor/ai/N/state` | igor → HA | volts, 3 decimals | no |
| `igor/temp/ADDRESS/state` | igor → HA | °C, 2 decimals | no |
| `igor/ha/heartbeat` | HA → igor | anything, once a minute | no |
| `homeassistant/status` | HA → igor | `online` / `offline`; on `online` igor repeats discovery and every state | no |

## Metrics

`:9101/metrics`. The first four match the Evok exporter's shape (`scrapeConfig/lancre/evok`).

| Metric | Labels |
|--------|--------|
| `igor_relay_state_boolean` | `circuit` |
| `igor_digital_input_state_boolean` | `circuit` |
| `igor_analog_input_volts` | `circuit` |
| `igor_temperature_celsius` | `address` |
| `igor_ha_silent`, `igor_mqtt_connected`, `igor_ha_last_heartbeat_timestamp_seconds` | - |
| `igor_relay_locked` | `circuit` |
| `igor_relay_failsafe_total` | `circuit`, `reason` (`max_on`, `silent`, `start`) |
| `igor_relay_commands_refused_total` | `circuit`, `reason` (`observe`, `locked`, `silent`, `retained`) |
| `igor_relay_drift_total` | `circuit` |
| `igor_bus_errors_total` | `bus` (`i2c`, `onewire`) |
| `igor_build_info` | `version`, `mode` |

Comparison with Evok during observe (both must stay empty):

```promql
igor_relay_state_boolean != on(circuit) evok_relay_state_boolean
```

```promql
igor_digital_input_state_boolean != on(circuit) evok_digital_input_state_boolean
```

## Try it on a laptop

1. Start the stack.

   ```bash
   docker compose up -d --build
   ```

2. Open http://127.0.0.1:8123, create a user, then add the MQTT integration with broker `mosquitto`, port 1883. The device "UniPi" appears with all entities.
3. Send a heartbeat by hand (the dev config goes silent after 3 minutes without one).

   ```bash
   docker compose exec mosquitto mosquitto_pub -t igor/ha/heartbeat -m 1
   ```

4. Remove everything.

   ```bash
   docker compose down -v
   ```

The fake board latches relays in memory, pulses DI 5 for 5 s every minute and drifts AI 1 and two probes.

## Tests

```bash
go test ./...
```

Drivers run against periph's I2C and 1-Wire playback fakes; nothing needs hardware.

## Deploy on `home`

Checked on 2026-11-15: Docker 26.1.4 with Compose v2.27.1, ports 1883 and 9101 free. The steps below have not been run on `home` yet.

1. Copy `deploy/home/` to `/srv/igor` on `home`.
2. Create `.env` from `.env.example`: an image tag and a new password for igor.
3. Create the broker users `igor` and `homeassistant`, each prompting for its password.

   ```bash
   docker compose run --rm mosquitto mosquitto_passwd -c /mosquitto/config/passwd igor
   ```

   ```bash
   docker compose run --rm mosquitto mosquitto_passwd /mosquitto/config/passwd homeassistant
   ```

4. Hand the password file to the broker's user; a root-owned file stops Mosquitto 2.1 from starting.

   ```bash
   docker compose run --rm mosquitto chown mosquitto:mosquitto /mosquitto/config/passwd
   ```

5. Start both containers.

   ```bash
   docker compose up -d
   ```

6. In Home Assistant add the MQTT integration: broker `127.0.0.1`, port 1883, user `homeassistant`. This needs no restart.
7. Check `curl -s 127.0.0.1:9101/metrics | grep igor_relay_state` against Evok.

`ghcr.io/thaum-xyz/igor` is built by `.github/workflows/igor.yaml` on pushes to `master` that touch `igor/`. The package is private until its visibility is changed, so `home` needs `docker login ghcr.io` or a public package.

### While Evok still runs

| Limit | Reason |
|-------|--------|
| Analog inputs and probes are off (the default in `observe`) | Both chips need multi-step transactions that Evok would interleave with. Relay and input reads are single transactions. |
| DI 1 has no entity | The `w1-gpio` overlay holds GPIO4. It goes away with the HAOS `config.txt`. |

### Going active

Home Assistant must send the heartbeat before igor is switched to `active`, or igor turns silent after 10 minutes.

```yaml
automation:
  - alias: igor heartbeat
    trigger:
      - platform: time_pattern
        minutes: "/1"
    action:
      - service: mqtt.publish
        data:
          topic: igor/ha/heartbeat
          payload: "{{ now().timestamp() | int }}"
```

Then, with Evok stopped, set `mode: active`, remove the two `enabled: false` lines and `docker compose up -d`.

## Known limits

| Limit | Effect |
|-------|--------|
| `max_on` counts from igor's start for a relay found on | A restart gives that relay a fresh allowance; set `off_on_start` where that matters. |
| igor stopped or crashed | Relays stay as they were; no failsafe runs. Alert on the scrape target being down. |

## Not built yet

| Item | Where it is tracked |
|------|---------------------|
| HAOS app manifest and `repository.yaml` | [lancre#15](https://github.com/thaum-xyz/lancre/issues/15) |
| Analog output (GPIO18 PWM) | out of the first release, [lancre#5](https://github.com/thaum-xyz/lancre/issues/5) |
