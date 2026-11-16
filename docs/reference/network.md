# Network

Hosts at lancre and what ankhmorpork reaches on them. Addresses and ports as seen on 2026-11-16 (sources: lancre map issue and ankhmorpork `master`).

## Hosts

| Host | Hardware | Runs | LAN | Tailscale | Reached by |
|---|---|---|---|---|---|
| `home` | Pi 4, 4 GB, on the UniPi 1.1 | Home Assistant Container 2025.9.4, igor, Mosquitto, cloudflared (Raspbian 10) | 192.168.20.4 | 100.98.38.80 | SSH over Tailscale (`ssh pi@home`) |
| `hmi` | Pi 3B+ with touchscreen | Chromium kiosk showing Home Assistant, cloudflared | 192.168.20.3 | 100.110.160.44 | Tailscale |

Home Assistant is public at `https://zmc.krupa.net.pl/` through the Cloudflare tunnel; ankhmorpork's blackbox exporter probes it (`cluster="lancre"`).

## Ports scraped from ankhmorpork

Scrape jobs are `scrapeConfig/lancre/<name>`, defined in ankhmorpork [`k8s/apps/lancre/`](https://github.com/thaum-xyz/ankhmorpork/tree/master/k8s/apps/lancre); targets use the LAN addresses.

| Host:port | Job | Serves |
|---|---|---|
| `home:9101` | `igor` | igor `/metrics` ([metric list](../../igor/README.md#metrics)) |
| `home:8123` | `homeassistant` | Home Assistant `/api/prometheus`, bearer token from an ExternalSecret |
| `home:9100`, `hmi:9100` | `node-exporter` | node exporter, plus the backup-copy textfile metrics on `home` |
| `home:8099`, `hmi:8099` | `cloudflared` | cloudflared metrics |
| `home:7979` | `evok` | Evok exporter; Evok is kept as rollback until it is removed ([Retire Evok](https://github.com/thaum-xyz/lancre/issues/14)) |
| `home:7003` | `circulation` | the old circulation container; relay 4 and an HA automation do its job |

Mosquitto listens on `home`'s loopback only (port 1883), without authentication until HAOS runs.

## Off-site services

| Service | Address | Used by | How |
|---|---|---|---|
| Recorder database | `postgres-lb.lancre.ankhmorpork.thaum.xyz:5432`, database `homeassistant` | Home Assistant recorder, 30 days | CNPG cluster `postgres` in ankhmorpork namespace `lancre`; [how it was moved](../how-to/move-recorder-to-postgres.md) |
| Backup share | UNAS 192.168.40.10, NFS share `hasszmc`, directory `backups/` | `/usr/local/bin/hass-backup-copy` on `home`, cron 06:30 | NFSv3, mounted for the copy only, newest 30 kept ([Copy HA backups to UNAS nightly](https://github.com/thaum-xyz/lancre/issues/11)) |
