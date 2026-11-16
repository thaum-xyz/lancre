# lancre

The smart home at lancre: igor, the bridge between the UniPi 1.1 board on `home` and Home Assistant, and the site's docs. Terms are in [GLOSSARY.md](GLOSSARY.md).

| Path | Holds |
|---|---|
| [igor/](igor/README.md) | igor: Go source, MQTT contract, metrics, deploy stack for `home` |
| [haos/igor/](haos/igor/config.yaml) | igor as a Home Assistant OS app; [repository.yaml](repository.yaml) makes this repo an app repository |
| [docs/reference/](docs/reference/) | [Network](docs/reference/network.md): hosts, scrape targets, recorder database, backup share |
| [docs/how-to/](docs/how-to/) | [Install igor as a HAOS app](docs/how-to/install-igor-haos-app.md), [move the recorder to PostgreSQL](docs/how-to/move-recorder-to-postgres.md) |
| [docs/research/](docs/research/) | Findings behind the decisions: [UniPi chip drivers](docs/research/unipi-chip-drivers.md), [HAOS hardware access](docs/research/haos-hardware-access.md), [recorder move](docs/research/recorder-sqlite-to-postgres.md) |
| [hack/](hack/) | Scripts the how-tos run |
| [docs/agents/](docs/agents/) | How agents use the issue tracker and this repo's docs |

Work is tracked in [GitHub issues](https://github.com/thaum-xyz/lancre/issues); the renewal map is [Lancre renewal](https://github.com/thaum-xyz/lancre/issues/3).
