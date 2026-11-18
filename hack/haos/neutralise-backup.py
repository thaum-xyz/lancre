#!/usr/bin/env -S python3 -I
"""Neutralise a lancre HA backup for the spare Pi: neutralise-backup.py IN.tar OUT.tar
Recorder -> SQLite, server_host dropped, integrations that reach the house, phones
or cloud APIs disabled. Prints names only, never values."""
import io, json, sys, tarfile
DISABLE = {"mobile_app", "unifiprotect", "huawei_solar", "shelly", "blebox", "esphome",
           "braviatv", "androidtv_remote", "cast", "dlna_dmr", "dlna_dms", "gne_pv_monitoring",
           "co2signal", "forecast_solar", "speedtestdotnet", "upnp", "ipp", "spotify", "withings",
           "generic"}
src, dst = sys.argv[1], sys.argv[2]
outer = tarfile.open(src)
meta = json.load(outer.extractfile("./backup.json"))
assert not meta["protected"], "encrypted backup"
inner = tarfile.open(fileobj=io.BytesIO(outer.extractfile("homeassistant.tar.gz").read()), mode="r:gz")
def edit(name, data):
    if name == "data/configuration.yaml":
        s = data.decode()
        for old, new in [("  db_url: !secret recorder_db_url\n", "  # db_url: !secret recorder_db_url  # spare: SQLite\n"),
                         ("  server_host: 0.0.0.0\n", "")]:
            assert s.count(old) == 1, old; s = s.replace(old, new)
        print("configuration.yaml: db_url commented, server_host removed", file=sys.stderr)
        return s.encode()
    if name == "data/.storage/core.config_entries":
        d = json.loads(data)
        for e in d["data"]["entries"]:
            if e["domain"] in DISABLE and e.get("disabled_by") is None:
                e["disabled_by"] = "user"; print(f"disabled {e['domain']}: {e['title']}", file=sys.stderr)
        return json.dumps(d, indent=4, ensure_ascii=False).encode()
    return data
buf = io.BytesIO()
with tarfile.open(fileobj=buf, mode="w:gz") as out:
    for m in inner.getmembers():
        if m.isfile():
            data = edit(m.name, inner.extractfile(m).read()); m.size = len(data)
            out.addfile(m, io.BytesIO(data))
        else:
            out.addfile(m)
meta["name"] += "-neutralised"
with tarfile.open(dst, "w") as o:
    for name, data in [("./backup.json", json.dumps(meta).encode()), ("homeassistant.tar.gz", buf.getvalue())]:
        ti = tarfile.TarInfo(name); ti.size = len(data); ti.mtime = outer.getmember(name).mtime; o.addfile(ti, io.BytesIO(data))
print("wrote", dst, file=sys.stderr)
