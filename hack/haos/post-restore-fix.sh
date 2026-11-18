#!/bin/sh
# Run on the HAOS host after restoring a `home` backup from Raspbian (copy to /mnt/data and run 'sh /mnt/data/post-restore-fix.sh'; piped into 'sh -s' the embedded Python breaks).
# 1. MQTT entry: 127.0.0.1 without login -> core-mosquitto with the login the Mosquitto app
#    offers Core (Supervisor discovery). Entry id kept, so igor's entities keep their names.
# 2. configuration.yaml: drop `server_host: 0.0.0.0` (disables the Core watchdog on HAOS).
# 3. Core watchdog on. Prints no credentials.
set -e
C=/mnt/data/supervisor/homeassistant
ha core stop --no-progress >/dev/null
cp -p $C/.storage/core.config_entries $C/.storage/core.config_entries.pre-haos
python3 - "$C" <<'PY'
import json, sys
c = sys.argv[1]
disc = [x["config"] for x in json.load(open("/mnt/data/supervisor/discovery.json"))["discovery"]
        if x["service"] == "mqtt" and x.get("app") == "core_mosquitto"]
assert len(disc) == 1, "no Mosquitto discovery: is the Mosquitto app started?"
d = disc[0]; p = f"{c}/.storage/core.config_entries"; s = json.load(open(p))
m = [e for e in s["data"]["entries"] if e["domain"] == "mqtt"]
assert len(m) == 1, f"{len(m)} mqtt entries"
m[0]["data"].update(broker=d["host"], port=d["port"], username=d["username"], password=d["password"])
json.dump(s, open(p, "w"), indent=4, ensure_ascii=False)
print(f"mqtt entry {m[0]['entry_id']}: broker={d['host']}:{d['port']}, login set")
y = f"{c}/configuration.yaml"; t = open(y).read()
if "\n  server_host: 0.0.0.0\n" in t:
    open(y, "w").write(t.replace("\n  server_host: 0.0.0.0\n", "\n")); print("configuration.yaml: server_host removed")
PY
ha core options --watchdog=true --no-progress >/dev/null 2>&1 || true
ha core start --no-progress >/dev/null
echo "core started"
