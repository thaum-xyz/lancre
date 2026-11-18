# Roll `home` back to Raspbian

Puts the old system back after a [swap to HAOS](swap-home-to-haos.md): the old SD card (Raspbian 10, boots first) and the USB stick (`/srv`: Home Assistant config, igor, Mosquitto) go back in together. They were not changed after the swap, so `home` comes back as it was at step 3 of the swap. The recorder stays on PostgreSQL throughout; the old Core is the same version (2025.9.4), so history written by HAOS in between stays readable.

1. **H**: unplug the network cable from `home` (HAOS cannot be shut down without access; a power cut is safe for the rollback).
2. **H**: take out the HAOS card, insert the old SD card, plug the USB stick back in.
3. **H**: plug the network cable back in. Containers start in about 3 min.
4. **R**: SSH answers through the old tunnel and Tailscale (`home-lancre`).

   ```sh
   ssh pi@ssh-home.krupa.net.pl 'docker ps --format "{{.Names}} {{.Status}}"'
   ```

5. **R**: igor connected and Home Assistant up.

   ```sh
   ssh pi@ssh-home.krupa.net.pl 'docker logs --tail 3 igor; curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8123/'
   ```

6. **R**: if any public hostname was moved to a new Cloudflare tunnel, move `zmc` and `ssh-home` back to tunnel `home` (`e7561110-…`) in Zero Trust → Networks → Tunnels.
7. **R**: un-silence the ankhmorpork `node-exporter` and `cloudflared` targets for `home`.

The HAOS card keeps its state; it can go back in after the cause is fixed, and the swap restarts at step 1 with a fresh backup.
