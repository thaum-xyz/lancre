# Install igor as a HAOS app

The app runs the published image `ghcr.io/thaum-xyz/igor` at the tag in [haos/igor/config.yaml](../../haos/igor/config.yaml) `version:`, which CI sets on every igor release. Its default options are the config `home` runs after Evok's retirement: `active`, the [lancre#7](https://github.com/thaum-xyz/lancre/issues/7) relay rules, the board's AI corrections, no analog output.

Menu names are those of HA Core 2025.9 ("Add-ons"); later Cores call them "Apps".

| Needs | Value |
|---|---|
| HAOS on `home` | `/dev/i2c-1` present: [I2C on HAOS](../research/haos-hardware-access.md#1-enabling-i2c-on-haos), appears on the second boot |
| Mosquitto app | installed and started; igor takes its broker address and user from it |
| `home` | port 9101 free; ankhmorpork Prometheus scrapes it there |

## Steps

1. In Home Assistant open Settings → Add-ons → Add-on store, ⋮ → Repositories, add `https://github.com/thaum-xyz/lancre` and close the dialog.
2. "igor" appears under "lancre apps"; open it and Install. Supervisor pulls the image.
3. Configuration tab: compare the options with [igor.example.yaml](../../igor/igor.example.yaml), which explains every key. Save.
4. Info tab: turn on Start on boot and Watchdog (restarts igor when its Docker health check fails), then Start.
5. Log tab: expect these two lines and no error.

   ```text
   level=INFO msg="starting igor" version=<version> mode=active board=unipi1.1 broker=tcp://core-mosquitto:1883
   level=INFO msg="connected to broker"
   ```

6. Check the board and the broker from any machine on the LAN: `igor_mqtt_connected 1`, `igor_bus_errors_total` 0 for `i2c`, a `igor_relay_state_boolean` line per relay.

   ```sh
   curl -s http://home:9101/metrics | grep -E '^igor_(mqtt_connected|bus_errors_total|relay_state_boolean)'
   ```

7. In Settings → Devices & services the MQTT integration lists the device "UniPi" with `switch.unipi_relay_1`…`8`, `binary_sensor.unipi_di_1`…`12` and the `sensor.unipi_temp_*` probes. DI 1 has an entity now that `w1-gpio` is gone.
8. In ankhmorpork Prometheus, `up{job=~"scrapeConfig/lancre/.*"}` is 1 for igor.

## Update

1. After an igor release, Add-on store → ⋮ → Check for updates (Supervisor also reloads every 3 h); the igor page offers Update to the new `version:`. Relays keep their state through the restart.
