# Runs on the HAOS host (ssh -p 22222 root@<pi> 'sh -s' < spare-probe.sh). Tests TCP connects
# from the host network namespace (where Core runs) and from the app network
# (hassio_cli), both with the CLI image's busybox nc. Prints FAIL on any leak.
echo "probe $(date -u +%FT%TZ)"
CLI=$(docker inspect -f '{{.Config.Image}}' hassio_cli)
hostnc() { docker run --rm --network host --entrypoint nc "$CLI" "$@"; }
T="192.168.20.1:53 192.168.110.1:443 192.168.110.2:80 192.168.130.2:80 192.168.50.137:5432 100.98.38.80:22 100.98.38.80:1883 192.168.20.4:1883 192.168.20.4:8123 192.168.40.10:2049"
for t in $T; do
  h=${t%:*}; p=${t#*:}
  if hostnc -z -w 4 "$h" "$p" 2>/dev/null; then echo "FAIL host open $t"; else echo "ok   host blocked $t"; fi
  if docker exec hassio_cli nc -z -w 4 "$h" "$p" 2>/dev/null; then echo "FAIL app  open $t"; else echo "ok   app  blocked $t"; fi
done
# Positive controls: the probes must be able to succeed at all.
hostnc -z -w 4 127.0.0.1 22222 2>/dev/null && echo "ctl  host probe works (127.0.0.1:22222)" || echo "FAIL host probe cannot connect anywhere"
if hostnc -z -w 5 ghcr.io 443 2>/dev/null; then echo "info internet reachable (ghcr.io:443)"; else echo "info no internet"; fi
