# Swap `home` to HAOS

One page for swap day: the old SD card and USB stick come out of `home`, the [prepared card](rebuild-home.md) goes in, and the newest configuration is restored onto it. **H** = a household member at the Pi (physical steps only); **R** = remote, over Tailscale from a Mac with the `lancre` repo. Takes about 30 min; heating relays hold their state while nothing runs. Undo: [rollback](rollback.md).

## Before the day

| Item | Why |
|---|---|
| Card labelled "home HAOS 18.5 / Core 2025.9.4", built and checked on the spare | [build notes](../research/2026-11-18-haos-card-spare-pi.md) |
| Decide public access: without a Cloudflared app on the card, `zmc.krupa.net.pl` and `ssh-home.krupa.net.pl` stop at step 4 (they point at the old tunnel `home`); phones away from home lose Home Assistant until it is set up | open |
| Silence `scrapeConfig/lancre/node-exporter` and `/cloudflared` for `home` in ankhmorpork; they have no HAOS equivalent | they go down at step 4 |
| A labelled bag for the old SD card and USB stick | rollback kit |

## Steps

1. **R**: take a manual, unencrypted backup on the old system and copy it to the Mac (automatic backups are encrypted). Run from the `thaum-xyz` folder.

   ```sh
   { cat hass-zmc-token; echo; } | ssh pi@ssh-home.krupa.net.pl "docker exec -i homeassistant python3 -c 'import base64,sys;exec(base64.b64decode(sys.argv[1]))' $(base64 < lancre/hack/haos/backup-home.py | tr -d '\n') swap-$(date +%F)"
   ```

2. **R**: copy it to the Mac.

   ```sh
   ssh pi@ssh-home.krupa.net.pl 'sudo sh -c "cat /srv/homeassistant/config/backups/swap-$(date +%F)_*.tar"' > swap-$(date +%F).tar
   ```

3. **R**: shut the old system down.

   ```sh
   ssh pi@ssh-home.krupa.net.pl 'sudo shutdown -h now'
   ```

4. **H**: when the green LED has stopped blinking, unplug the network cable from `home`. Take out the SD card and the USB stick, put both in the bag.
5. **H**: insert the new card (label up, as the old one), leave the USB port empty, plug the network cable back in. The green LED blinks in a heartbeat within a minute.
6. **R**: the Tailscale node `home` is online within 5 min; the host shell answers.

   ```sh
   ssh -p 22222 root@home 'ha core info | grep ^version; ls /dev/i2c-1; ha apps logs eee51721_igor | tail -3'
   ```

7. **R**: upload the backup and restore Core only (about 1 min; Core version equal, nothing to download).

   ```sh
   ssh -p 22222 root@home 'cat > /mnt/data/supervisor/backup/swap.tar' < swap-YYYY-MM-DD.tar && ssh -p 22222 root@home 'ha backups reload && ha backups list'
   ```

8. **R**: restore it.

   ```sh
   ssh -p 22222 root@home 'ha backups restore <slug> --homeassistant'
   ```

9. **R**: repoint MQTT to the Mosquitto app, drop `server_host`, turn on the Core watchdog (the script is already on the card).

   ```sh
   ssh -p 22222 root@home 'sh /mnt/data/post-restore-fix.sh'
   ```

## Checks

| # | Check | Pass |
|---|---|---|
| 1 | igor log | `connected to broker`; no `remote I/O error` |
| 2 | Relays | `curl -s http://home:9101/metrics \| grep igor_relay_state_boolean` lists 8 relays; spare relay 2 on/off from HA answers |
| 3 | Heating | `climate.heating` heating ⇒ relays 5 and 1 follow within 3 min (`burner_follows_demand`) |
| 4 | igor not silent | `igor_mqtt_connected 1`; heartbeat automation runs every minute without errors |
| 5 | Recorder | Core log has no recorder errors; a `homeassistant` connection in `pg_stat_activity` on ankhmorpork `lancre/postgres` |
| 6 | Panel (**H**) | `hmi` shows the `dom` dashboard (reload the page if it shows a connection error) |
| 7 | ankhmorpork | `up{job=~"scrapeConfig/lancre/(igor\|homeassistant)"}` = 1 |
| 8 | Remote access | Tailscale `home` answers on 22, 22222, 8123; public access per the decision above |
| 9 | Failsafe with Core stopped | relays 1, 6, 7 off after the 2 min grace ([igor tests](https://github.com/thaum-xyz/lancre/issues/15)) |

Any of checks 1-5 failing and not fixable in 30 min: [rollback](rollback.md).

## After

Add the UNAS share `hasszmc` as backup location (Settings → System → Storage; NFS 192.168.40.10, export allows 192.168.20.4 only), update [network](../reference/network.md), drop the old scrape targets in ankhmorpork, keep the rollback kit until the soak ends.
