"""Move Home Assistant long-term statistics from a SQLite copy to an empty PostgreSQL.

Run inside the Home Assistant image of the same version as the source database.
Env: SQLITE (path), PGHOST, PGDATABASE, PGUSER, PGPASSWORD.
"""
import csv
import io
import os
import sqlite3
import sys
import time
from urllib.parse import quote

from sqlalchemy import Boolean, create_engine, text

from homeassistant.components.recorder.db_schema import SCHEMA_VERSION, Base

TABLES = ["statistics_meta", "statistics", "statistics_short_term"]
BATCH = 50_000

url = "postgresql://{}:{}@{}/{}?sslmode=require&connect_timeout=10".format(
    os.environ["PGUSER"], quote(os.environ["PGPASSWORD"], safe=""),
    os.environ["PGHOST"], os.environ["PGDATABASE"])
engine = create_engine(url)
src = sqlite3.connect("file:{}?mode=ro".format(os.environ["SQLITE"]), uri=True)

ver = src.execute(
    "select schema_version from schema_changes order by change_id desc limit 1").fetchone()[0]
if ver != SCHEMA_VERSION:
    sys.exit(f"source schema {ver} != image schema {SCHEMA_VERSION}")

with engine.connect() as c:
    n = c.execute(text(
        "select count(*) from information_schema.tables where table_schema='public'")).scalar()
if n:
    sys.exit(f"target is not empty: {n} tables")

t0 = time.time()
Base.metadata.create_all(engine)
print(f"schema v{SCHEMA_VERSION} created in {time.time() - t0:.1f}s")

raw = engine.raw_connection()
cur = raw.cursor()
cur.execute("set timezone to 'UTC'")
for name in TABLES:
    table = Base.metadata.tables[name]
    src_cols = [r[1] for r in src.execute(f"pragma table_info({name})")]
    pg_cols = [c.name for c in table.columns]
    if set(src_cols) != set(pg_cols):
        print(f"{name}: column mismatch sqlite-only={set(src_cols) - set(pg_cols)} "
              f"pg-only={set(pg_cols) - set(src_cols)}")
    cols = [c for c in pg_cols if c in src_cols]
    bools = [i for i, c in enumerate(cols) if isinstance(table.columns[c].type, Boolean)]
    t0, rows = time.time(), 0
    rd = src.execute(f"select {','.join(cols)} from {name} order by id")
    while batch := rd.fetchmany(BATCH):
        buf = io.StringIO()
        w = csv.writer(buf)
        for row in batch:
            row = list(row)
            for i in bools:
                if row[i] is not None:
                    row[i] = "t" if row[i] else "f"
            w.writerow(["" if v is None else repr(v) if isinstance(v, float) else v for v in row])
        buf.seek(0)
        cur.copy_expert(f"copy {name} ({','.join(cols)}) from stdin with (format csv)", buf)
        rows += len(batch)
    raw.commit()
    print(f"{name}: {rows} rows in {time.time() - t0:.1f}s")

# Identity sequences stay at 1 after explicit-id inserts.
cur.execute("""
DO $$ DECLARE r record; BEGIN
  FOR r IN SELECT table_name, column_name FROM information_schema.columns
           WHERE table_schema = 'public' AND is_identity = 'YES' LOOP
    EXECUTE format('SELECT setval(pg_get_serial_sequence(%L, %L), COALESCE((SELECT max(%I) FROM %I), 0) + 1, false)',
                   r.table_name, r.column_name, r.column_name, r.table_name);
  END LOOP; END $$""")
raw.commit()

ok = True
for name in TABLES:
    q = f"select count(*), max(id), max(start_ts) from {name}" if name != "statistics_meta" \
        else f"select count(*), max(id), null from {name}"
    a = tuple(src.execute(q).fetchone())
    cur.execute(q)
    b = tuple(cur.fetchone())
    cur.execute(f"select nextval(pg_get_serial_sequence('{name}', 'id'))")
    seq = cur.fetchone()[0]
    good = a == b and seq > a[1]
    ok &= good
    print(f"{name}: sqlite={a} pg={b} next_id={seq} {'OK' if good else 'MISMATCH'}")

q = ("select metadata_id, sum, state, mean, min, max from statistics_short_term "
     "where start_ts = (select max(start_ts) from statistics_short_term) order by metadata_id")
a = src.execute(q).fetchall()
cur.execute(q)
b = cur.fetchall()
good = a == b
ok &= good
print(f"last short-term period: {len(a)} rows {'identical' if good else 'DIFFER'}")
cur.execute("select count(*) from schema_changes")
print("schema_changes rows (must be 0 so HA treats the DB as new):", cur.fetchone()[0])
cur.execute("select pg_size_pretty(pg_database_size(current_database()))")
print("database size:", cur.fetchone()[0])
raw.commit()
sys.exit(0 if ok else 1)
