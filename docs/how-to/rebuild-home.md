# Rebuild `home` on Home Assistant OS

Builds a microSD card for `home` (Pi 4 on the UniPi 1.1) from a backup of the running system, on a spare Pi 4 away from the house, so the card boots ready in `home`. First done 2026-11-18 with the SanDisk Extreme Pro 64 GB; what the hardware showed is in [the build notes](../research/2026-11-18-haos-card-spare-pi.md). Swap day itself: [swap `home` to HAOS](swap-home-to-haos.md).

| Needs | Value |
|---|---|
| Card | 64 GB A1/A2 microSD; HAOS images are per board family, so any Pi 4 runs it |
| Image | `haos_rpi4-64-<ver>.img.xz` from [HAOS releases](https://github.com/home-assistant/operating-system/releases), sha256 from the release |
| Backup | **manual, unencrypted** backup of `home`, Core version = the one the house runs. Automatic backups (and their UNAS copies) are encrypted with the backup key from Settings → System → Backups |
| Spare Pi network | internet only: a VLAN that cannot reach any private range, and that your LAN can reach (for SSH :22222, :22, :8123). On banacha: network `lancre spare`, VLAN 120, DMZ zone, switch port 4 |
| Mac tools | `f3` (Homebrew), `xz`, your SSH public key |

Scripts are in [hack/haos/](../../hack/haos/). `<pi>` is the spare's address.

## Card

1. Find the card; it must be the ~64 GB, USB, External, Removable, `Virtual: No` disk. Disk numbers change between commands (the card moved from `disk6` to `disk4` while a Time Machine image took `disk6`), so check right before every write.

   ```sh
   diskutil list external physical
   ```

2. Write-and-verify the whole card (about 25 min): `f3read` must report `Data LOST: 0.00 Bytes`.

   ```sh
   diskutil eraseDisk ExFAT F3 MBR diskN && f3write /Volumes/F3 && f3read /Volumes/F3
   ```

3. Flash, then read back exactly the image's size and compare hashes (the image is a whole number of 4 KiB blocks).

   ```sh
   diskutil unmountDisk diskN && xz -dc haos_rpi4-64-18.5.img.xz | sudo dd of=/dev/rdiskN bs=4m
   ```

4. Mount the boot partition (an EFI-type FAT, so it needs sudo) and apply the boot edits: I2C, PoE HAT fan curve, heartbeat LED, `i2c-dev`, your key for the host shell on :22222.

   ```sh
   sudo diskutil mount diskNs1 && hack/haos/prep-boot.sh && sudo diskutil eject diskN
   ```

## First boots

5. Card into the spare, cable into the isolated port. Within 2 min it gets an address; check the host shell.

   ```sh
   ssh -p 22222 root@<pi> 'cat /etc/os-release | grep PRETTY; ls /dev/gpiochip0'
   ```

6. Prove the network isolation before anything else: every line `ok … blocked`, internet reachable.

   ```sh
   ssh -p 22222 root@<pi> 'sh -s' < hack/haos/spare-probe.sh
   ```

7. First boot installs the newest Core. Let it finish, then move its config aside and pin the house's Core. Downgrading onto a config the newer Core already wrote crashes Core and Supervisor rolls back.

   ```sh
   ssh -p 22222 root@<pi> 'ha core stop && mv /mnt/data/supervisor/homeassistant /mnt/data/supervisor/homeassistant.blank && mkdir /mnt/data/supervisor/homeassistant && ha core update --version 2025.9.4 && ha core start'
   ```

8. Reboot once; `/dev/i2c-1` appears on the second boot.

   ```sh
   ssh -p 22222 root@<pi> 'ha host reboot'
   ```

## Apps

The `ha` CLI here has no `options` command; options go to the Supervisor API through `hassio_cli` (`docker exec -i hassio_cli sh -c 'curl -s -X POST -H "Authorization: Bearer $SUPERVISOR_TOKEN" -H Content-Type:application/json http://supervisor/addons/<slug>/options -d @-'`).

| App | Slug | Repository | Options |
|---|---|---|---|
| Mosquitto broker | `core_mosquitto` | built in | defaults; boot auto, watchdog |
| Terminal & SSH | `core_ssh` | built in | `authorized_keys`: your key; `network: {"22/tcp": 22}` (unpublished by default); boot auto |
| igor | `eee51721_igor` | `https://github.com/thaum-xyz/lancre` (`ha store add …`) | defaults (= `home`'s config); boot auto, watchdog |
| Tailscale | `a0d7b954_tailscale` | built in | `userspace_networking: true`, `accept_dns: false`, `accept_routes: false`, no routes, no exit node; boot auto |

9. Install and start Mosquitto, SSH and igor; set the options above.

   ```sh
   ssh -p 22222 root@<pi> 'ha apps install core_mosquitto && ha apps install core_ssh && ha store add https://github.com/thaum-xyz/lancre && ha apps install eee51721_igor'
   ```

10. Set the host name; Tailscale names the node after it.

    ```sh
    ssh -p 22222 root@<pi> 'ha host options --hostname home'
    ```

11. Install and start Tailscale; open the `AuthURL` from its log, approve the machine and disable its key expiry in the admin console.

    ```sh
    ssh -p 22222 root@<pi> 'ha apps install a0d7b954_tailscale && ha apps start a0d7b954_tailscale && sleep 15 && ha apps logs a0d7b954_tailscale | grep AuthURL'
    ```

12. From another tailnet machine, ports 22, 22222 and 8123 on the new node answer.

## Restore

13. Neutralise the backup on the Mac: recorder → SQLite, `server_host` dropped, integrations that reach the house, phones or clouds disabled. The Pi never sees the original.

    ```sh
    hack/haos/neutralise-backup.py home-backup.tar home-backup-neutralised.tar
    ```

14. Stop Tailscale, set Core and Tailscale not to start on boot (a reboot drops the firewall), apply the firewall, probe again.

    ```sh
    ssh -p 22222 root@<pi> 'sh -s' < hack/haos/spare-firewall.sh && ssh -p 22222 root@<pi> 'sh -s' < hack/haos/spare-probe.sh
    ```

15. Upload and restore Core only (58 s on the Pi).

    ```sh
    ssh -p 22222 root@<pi> 'cat > /mnt/data/supervisor/backup/restore.tar' < home-backup-neutralised.tar && ssh -p 22222 root@<pi> 'ha backups reload && ha backups restore <slug> --homeassistant'
    ```

16. Repoint MQTT to `core-mosquitto` with Mosquitto's login, drop `server_host`, watchdog on. Copy the script first; piped into `sh -s` it breaks.

    ```sh
    ssh -p 22222 root@<pi> 'cat > /mnt/data/post-restore-fix.sh' < hack/haos/post-restore-fix.sh && ssh -p 22222 root@<pi> 'sh /mnt/data/post-restore-fix.sh'
    ```

17. Check: the Mosquitto log shows `u'homeassistant'` connected; Core log has no errors except unreachable house devices and igor's missing relays; you can log in and the dashboards render.

## Finish

18. Core and Tailscale start on boot again, Mosquitto watchdog on, the restore tar removed (`ha backups remove <slug>`), then shut down.

    ```sh
    ssh -p 22222 root@<pi> 'ha host shutdown'
    ```

19. Label the card with its state (Core version, backup name and date, "neutralised: restore before use").
