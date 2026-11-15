# Move the recorder from SQLite to PostgreSQL

Keeps long-term statistics (energy, hourly means); short-term history starts fresh. Rehearsed on 2026-11-15 against HA 2025.9.4 (schema 50) and PostgreSQL 17.11: 3 067 810 + 191 974 + 170 rows loaded in 71 s, HA started twice with no warning, migration or schema repair. Why this procedure: [research](../research/recorder-sqlite-to-postgres.md).

Not rehearsed: sums of real `sum` sensors continuing after the switch. Check the energy dashboard the hour after.

| Needs | Value |
|---|---|
| Target | `postgres-lb.lancre.ankhmorpork.thaum.xyz:5432`, database and user `homeassistant`, **empty** |
| Password | Doppler `LANCRE_DB_PASS`, in a file `./lancre-db-pass` on the workstation |
| Workstation | Docker, the HA image of **the same version as the running HA** |

## Steps

1. Take a consistent copy on `home` while HA runs (30 s for 600 MB). `sqlite3` is not installed there; Python is.

   ```sh
   ssh pi@home 'sudo python3 -c "
   import sqlite3
   s = sqlite3.connect(\"file:/srv/homeassistant/db/home-assistant_v2.db?mode=ro\", uri=True)
   d = sqlite3.connect(\"/home/pi/ha-copy.db\"); s.backup(d, pages=-1); d.close()
   print(sqlite3.connect(\"/home/pi/ha-copy.db\").execute(\"pragma quick_check\").fetchone())"'
   ```

2. Fetch it and remove it from the Pi. `scp` fails against Raspbian 10; stream it.

   ```sh
   mkdir -p work && ssh pi@home 'sudo cat /home/pi/ha-copy.db && sudo rm /home/pi/ha-copy.db' > work/ha-copy.db
   ```

3. Create the schema, load the three statistics tables, fix the identity sequences and verify. The script refuses a non-empty target or a schema version that differs from the image. Every table line must end in `OK`.

   ```sh
   PGPASSWORD=$(cat lancre-db-pass) docker run --rm --entrypoint python3 \
     -e PGPASSWORD -e PGHOST=postgres-lb.lancre.ankhmorpork.thaum.xyz \
     -e PGDATABASE=homeassistant -e PGUSER=homeassistant -e SQLITE=/data/ha-copy.db \
     -v "$PWD/work:/data:ro" -v "$PWD/hack:/scripts:ro" \
     ghcr.io/home-assistant/home-assistant:2025.9.4 /scripts/recorder_move.py
   ```

4. Point HA at it in `configuration.yaml` (password URL-encoded, from `secrets.yaml`) and restart HA once.

   ```yaml
   recorder:
     db_url: !secret recorder_db_url  # postgresql://homeassistant:…@postgres-lb.lancre.ankhmorpork.thaum.xyz/homeassistant?sslmode=require&connect_timeout=10
     purge_keep_days: 30
   ```

5. Read the log. Expect `Data migration … not needed, database created with version 50`. None of: `does not support double precision`, `about to correct DB schema errors`, `about to upgrade`, `UniqueViolation`.

6. Delete `work/`.

## Start over

Drops everything in the target; only before step 4.

```sh
PGPASSWORD=$(cat lancre-db-pass) psql "host=postgres-lb.lancre.ankhmorpork.thaum.xyz dbname=homeassistant user=homeassistant sslmode=require" -c "DO \$\$ DECLARE r record; BEGIN FOR r IN SELECT tablename FROM pg_tables WHERE schemaname='public' LOOP EXECUTE format('DROP TABLE %I CASCADE', r.tablename); END LOOP; END \$\$"
```

## Differences from the research note

| Research | Rehearsal |
|---|---|
| pgloader | Python in the HA image: pgloader has no arm64 image |
| `sqlite3 .backup` CLI | Python `Connection.backup(pages=-1)`: one read transaction, no restarts |
| CNPG encoding unconfirmed | `UTF8` |
