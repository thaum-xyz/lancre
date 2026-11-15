#!/bin/sh
# Prints "version=YYYY.WW.PATCH" for the next igor release: ISO year and
# week of today (UTC), PATCH one above the highest igor-v tag of that week.
# NOW overrides the date for tests, as seconds since the epoch.
set -eu
week=$(date -u ${NOW:+-d "@$NOW"} +%G.%V)
last=$(git tag -l "igor-v$week.*" | sed "s/^igor-v$week\.//" | grep -E '^[0-9]+$' | sort -n | tail -1 || true)
echo "version=$week.$(( ${last:--1} + 1 ))"
