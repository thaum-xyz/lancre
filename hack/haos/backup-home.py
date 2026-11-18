"""Manual, unencrypted, database-less backup of HA Container on `home` (Raspbian).
Runs inside the homeassistant container; token on stdin, never in argv:
  { cat hass-zmc-token; echo; } | ssh pi@ssh-home.krupa.net.pl "docker exec -i homeassistant python3 -c \
    'import base64,sys;exec(base64.b64decode(sys.argv[1]))' $(base64 < backup-home.py | tr -d '\n') NAME"
Prints the backup id and size when done."""
import asyncio, aiohttp, json, sys, time
tok = sys.stdin.readline().strip()
NAME = sys.argv[-1]
async def main():
    async with aiohttp.ClientSession() as s, s.ws_connect("http://127.0.0.1:8123/api/websocket") as ws:
        await ws.receive_json(); await ws.send_json({"type": "auth", "access_token": tok}); a = await ws.receive_json()
        print("auth", a["type"])
        async def call(i, msg):
            msg["id"] = i; await ws.send_json(msg)
            while True:
                r = await ws.receive_json()
                if r.get("id") == i and r.get("type") == "result": return r
        r = await call(1, {"type": "backup/generate", "agent_ids": ["backup.local"], "include_homeassistant": True,
                           "include_database": False, "name": NAME, "password": None})
        print("generate", r.get("success"), r.get("result") or r.get("error"))
        for i in range(2, 40):
            await asyncio.sleep(5)
            r = await call(i, {"type": "backup/info"})
            for b in r["result"]["backups"]:
                if b["name"] == NAME:
                    print("done", b["backup_id"], b["date"], b.get("database_included"), {k: v.get("protected") for k, v in b["agents"].items()}, {k: v.get("size") for k, v in b["agents"].items()}); return
            st = r["result"].get("state")
            print("waiting", st)
asyncio.run(main())
