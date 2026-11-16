# Glossary

| Term | Meaning | Avoid |
|------|---------|-------|
| **Terminal** | One physical I/O point on the UniPi board: a relay, digital input, analog input, analog output or 1-Wire probe. Identified by kind and number (or probe address), never by what is wired to it. | channel, circuit, port |
| **Heartbeat** | The message Home Assistant sends once a minute to show it is alive and deciding. | ping, keepalive |
| **Silent** | Home Assistant is silent when igor has had no broker connection for longer than the grace, or Home Assistant has reported itself offline for longer than the grace, or no heartbeat arrived within the timeout. | down, offline, lost |
| **Failsafe** | What igor does to a relay on its own, without a command from Home Assistant. | watchdog, fallback |
| **Heat demand** | Relay 5's signal asking the boiler to heat. The boiler keeps its own limiter and safety chain. | flame, burner |
