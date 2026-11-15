# HAOS hardware access for `igor` on a Pi 4 (I2C, GPIO lines, PWM)

Researched 2026-11-15 for [#5](https://github.com/thaum-xyz/lancre/issues/5). Sources pinned to:

| Source | Version read |
|---|---|
| HAOS (`home-assistant/operating-system`) | tag [`18.5`](https://github.com/home-assistant/operating-system/tree/18.5) (released 2026-11-06, latest) |
| HAOS rpi4-64 kernel | `raspberrypi/linux` [`9b488bc`](https://github.com/raspberrypi/linux/tree/9b488bc7a1e9b61713082f3e6c4361988d62f77b) = 6.18.59 ([kernel.md](https://github.com/home-assistant/operating-system/blob/18.5/Documentation/kernel.md)) |
| RPi firmware + overlays | `raspberrypi/firmware` [`f2491b7`](https://github.com/raspberrypi/firmware/tree/f2491b71ed49880cc8285e41e03f81b163c6cd59) (pinned in [rpi-firmware.mk L8](https://github.com/home-assistant/buildroot/blob/5c344b3789e3f6d620cf11ffab60e522a875cc79/package/rpi-firmware/rpi-firmware.mk#L8)) |
| Supervisor | `main` [`98a2322`](https://github.com/home-assistant/supervisor/tree/98a2322374ae3d0c248079d11f7435ad759935fa) (2026-11-12) |
| App developer docs | `developers.home-assistant` [`50b5188`](https://github.com/home-assistant/developers.home-assistant/tree/50b518898989b67bb76b69827f227aa6f0acd32a) (2026-10-16) |

Abbreviations below: `OS:` = `https://github.com/home-assistant/operating-system/blob/18.5/`, `SUP:` = `https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/`.

## Answer in one table

| Need | Host side (boot partition) | App `config.yaml` | Confidence |
|---|---|---|---|
| `/dev/i2c-1` | `dtparam=i2c_arm=on` in `config.txt`; `CONFIG/modules/rpi-i2c.conf` containing `i2c-dev`; boot twice | `devices: [/dev/i2c-1]` | Source-confirmed |
| `/dev/gpiochip0` lines | nothing (GPIO chardev is built in) | `devices: [/dev/gpiochip0]` (narrowest) or `gpio: true` | Source-confirmed |
| sysfs PWM on GPIO18 | `dtoverlay=pwm,pin=18,func=2` (consider `dtparam=audio=off`) | `gpio: true` **and** `apparmor: false` or a custom `apparmor.txt` | Inferred from source, not tested on hardware |
| PoE HAT fan curve | `dtoverlay=rpi-poe` / `rpi-poe-plus` + `dtparam=poe_fan_temp*` | none | Overlay and drivers present; HAT-EEPROM interaction unconfirmed |

Not needed: `full_access`, `privileged` capabilities, `kernel_modules`, `udev`.

## 1. Enabling I2C on HAOS

### Documented procedure

The home-assistant.io "Enable I2C" section was **removed on 2026-07-06** ([home-assistant.io#46698](https://github.com/home-assistant/home-assistant.io/pull/46698)) because no Core integration uses I2C any more. The PR says the procedure itself still works on current HAOS. Last published text: [enable_i2c.md @ 074d5e0](https://github.com/home-assistant/home-assistant.io/blob/074d5e0f45/source/_includes/common-tasks/enable_i2c.md).

| Step | File on `hassos-boot` | Content | Source |
|---|---|---|---|
| 1 | `CONFIG/modules/rpi-i2c.conf` | `i2c-dev` | [enable_i2c.md](https://github.com/home-assistant/home-assistant.io/blob/074d5e0f45/source/_includes/common-tasks/enable_i2c.md) |
| 2 | `config.txt` (append) | `dtparam=i2c_vc=on` and `dtparam=i2c_arm=on` | same |
| 3 | boot; "another reboot might be necessary" | — | same |

`i2c_arm` alone gives `i2c-1` (GPIO2/3); `i2c_vc` enables the bus "usually reserved for the VideoCore processor" ([overlays README L306, L341](https://github.com/raspberrypi/firmware/blob/f2491b71ed49880cc8285e41e03f81b163c6cd59/boot/overlays/README#L306)). The UniPi 1.1 needs only `i2c-1`, so `i2c_vc` can be left out (inference; the doc does not explain why it lists both).

### Why it works, from source

| Mechanism | Evidence |
|---|---|
| `haos-config.service` imports `/mnt/boot/CONFIG` when no USB stick labelled `CONFIG` is present, copies `modules/*` into `/etc/modules-load.d/` (replacing what was there), then deletes `/mnt/boot/CONFIG` | [OS:…/usr/sbin/haos-config L3, L47-51, L117](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/rootfs-overlay/usr/sbin/haos-config#L47-L51) |
| `/etc/modules-load.d` is a bind mount of `/mnt/overlay/etc/modules-load.d`, so it persists across OS updates | [OS:…/etc-modules\x2dload.d.mount](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/rootfs-overlay/usr/lib/systemd/system/etc-modules%5Cx2dload.d.mount) |
| `haos-config.service` runs `After=haos-bind.target network.target` in `multi-user.target`, so after `systemd-modules-load` on first boot. This is why a second boot is needed | [OS:…/haos-config.service](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/rootfs-overlay/usr/lib/systemd/system/haos-config.service); maintainer log analysis in [operating-system#4752](https://github.com/home-assistant/operating-system/issues/4752) |
| `i2c-dev` is a module (`CONFIG_I2C_CHARDEV=m`) and has no device-tree alias, so it must be listed explicitly; `i2c-bcm2835` (`=m`) loads from the device tree | [bcm2711_defconfig L768, L773](https://github.com/raspberrypi/linux/blob/9b488bc7a1e9b61713082f3e6c4361988d62f77b/arch/arm64/configs/bcm2711_defconfig#L768) |
| U-Boot boots the kernel with the firmware-built FDT (`fdt_addr`), so `config.txt` `dtparam`/`dtoverlay` lines take effect | [OS:…/uboot-boot64.ush L32-34, L99](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/board/raspberrypi/uboot-boot64.ush#L32-L34) |
| Stock `config.txt` ships `#dtparam=i2c_arm=on` commented, `dtparam=audio=on` active, and ends with `[all]`, so appended lines apply to all models | [OS:…/raspberrypi/config.txt L56, L72, L90](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/board/raspberrypi/config.txt#L56) |

### Pre-first-boot on a Mac

All of it can be written to the freshly flashed `hassos-boot` FAT partition before first boot: `haos-config` only checks `[ -d /mnt/boot/CONFIG ]`, with no first-boot condition ([haos-config L23-25](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/rootfs-overlay/usr/sbin/haos-config#L23-L25)). Expect `/dev/i2c-1` on the **second** boot.

Unconfirmed precaution: macOS may write AppleDouble `._rpi-i2c.conf` files to FAT volumes. `haos-config` copies `modules/*` wholesale, and `._rpi-i2c.conf` also ends in `.conf`. Delete `._*` files (`dot_clean`) before ejecting. Not verified on HAOS.

### Do HAOS updates keep `config.txt` edits?

Yes on rpi4-64. The RAUC boot-slot hook backs up `/mnt/boot/*.txt`, copies the new boot image over, then restores the `*.txt` files. It also **wipes `/mnt/boot/overlays/*`** first, so stock overlays come back but any custom `.dtbo` you drop there is lost ([OS:…/ota/rauc-hook L26-45](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/ota/rauc-hook#L26-L45)). Only rpi5-64 takes a different path. One consequence: `config.txt` never picks up upstream changes to the stock file after first flash (inference from the same lines).

## 2. App `config.yaml` keys

Reference table: [developers.home-assistant.io/docs/apps/configuration](https://developers.home-assistant.io/docs/apps/configuration) ([source L145-205](https://github.com/home-assistant/developers.home-assistant/blob/50b518898989b67bb76b69827f227aa6f0acd32a/docs/apps/configuration.md#L145-L205)). What the Supervisor actually does:

| Key | Docs say | Supervisor does (source) | Security rating effect |
|---|---|---|---|
| (always) | — | Bind-mounts host `/dev` read-only into every app; access is gated by device cgroup rules ([SUP:supervisor/docker/const.py L176-182](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/const.py#L176-L182), [app.py L468](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/app.py#L468)) | — |
| `devices` | "Device list to map into the app" | Adds cgroup rule `c <major>:<minor> rwm` per listed path; a missing path is skipped at start, and a later udev event for it adds the rule live via OS Agent ([app.py L252-283](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/app.py#L252-L283), [L1063-1109](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/app.py#L1063-L1109)) | none |
| `gpio` | maps `/sys/class/gpio`; may need AppArmor off | Cgroup rules for every major in udev subsystems `gpio` and `gpiomem`, plus **read-write** bind mounts of `/sys/class/gpio` and `/sys/devices/platform/soc` ([app.py L289-291, L619-629](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/app.py#L619-L629), [hardware/helper.py L24-27](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/hardware/helper.py#L24-L27), [hardware/policy.py](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/hardware/policy.py)) | none |
| `privileged` | list of capabilities | Docker `cap_add` only, never Docker `--privileged` ([app.py L384-399](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/app.py#L384-L399)) | −1 for `SYS_RAWIO`, `SYS_ADMIN`, `SYS_MODULE`, `NET_ADMIN`, and others ([apps/utils.py L54-73](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/apps/utils.py#L54-L73)) |
| `full_access` | "like the privileged mode in Docker" | Only replaces cgroup rules with `a *:* rwm` (all devices), and only when protection mode is off. No Docker `--privileged`, no `/sys` remount ([app.py L301-303](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/app.py#L301-L303), [policy.py `get_full_access`](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/hardware/policy.py)) | rating forced to 1 ([utils.py L93-94](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/apps/utils.py#L93-L94)) |
| `apparmor` | bool or profile name | `false` → `apparmor=unconfined`; `apparmor.txt` in the app folder → profile named after the slug; otherwise Docker's default profile ([app.py L323-336](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/app.py#L323-L336), [apps/model.py L448-454, L728-730](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/apps/model.py#L448-L454)) | −1 disabled, +1 custom profile |
| `kernel_modules` | host modules read-only + `SYS_MODULE` | `/lib/modules` read-only mount + `SYS_MODULE` ([app.py L389-390, L647-655](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/app.py#L647-L655)) | −1 |
| `udev` | host udev DB read-only | `MOUNT_UDEV` ([app.py L643-644](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/app.py#L643-L644)) | none |

Seccomp is always `unconfined` for apps ([SUP:supervisor/docker/interface.py L203-207](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/docker/interface.py#L203-L207)).

### Is `/sys` writable without `full_access`?

`full_access` makes no difference to `/sys`. Docker mounts `/sys` read-only, and the Supervisor only re-mounts `/sys/class/gpio` and `/sys/devices/platform/soc` read-write, and only for `gpio: true` (table above). A Supervisor maintainer confirmed both points in [supervisor#4560](https://github.com/home-assistant/supervisor/issues/4560): Docker keeps `/sys` read-only by design, and those GPIO mounts are "an ugly hack" meant to go once software moves to the chardev interface (2023-09-19; the mounts are still in the 2026-11-12 source). [supervisor#5839](https://github.com/home-assistant/supervisor/issues/5839) shows the `Read-only file system` error when that mount is missing.

The second gate is AppArmor. Docker's default profile denies writes under `/sys/[^f]*/**`, which covers `/sys/class/**` and `/sys/devices/**` ([moby/profiles apparmor/template.go L76-77](https://github.com/moby/profiles/blob/f862455628f78f84c937e967e4a03902fb1dee11/apparmor/template.go#L76-L77)). This is why the docs say `gpio` needs AppArmor disabled or a custom profile.

### PWM path on the Pi 4 (inference, test on the device)

PWM0 is `pwm@7e20c000` under the `soc` node ([bcm283x.dtsi L413](https://github.com/raspberrypi/linux/blob/9b488bc7a1e9b61713082f3e6c4361988d62f77b/arch/arm/boot/dts/broadcom/bcm283x.dtsi#L413)), remapped to `0xfe…` on BCM2711 ([bcm2711.dtsi L41](https://github.com/raspberrypi/linux/blob/9b488bc7a1e9b61713082f3e6c4361988d62f77b/arch/arm/boot/dts/broadcom/bcm2711.dtsi#L41)). So `/sys/class/pwm/pwmchip0` should resolve to `/sys/devices/platform/soc/fe20c000.pwm/pwm/pwmchip0`, which is inside the `gpio: true` read-write mount. GPIO18 with `func=2` (Alt5) is PWM0 channel 0 ([overlays README L4440-4456](https://github.com/raspberrypi/firmware/blob/f2491b71ed49880cc8285e41e03f81b163c6cd59/boot/overlays/README#L4440-L4456)). I could not confirm on hardware that `export` and `period` writes succeed through this mount.

### Minimal set for `igor`

| Phase | Keys | Rating cost |
|---|---|---|
| I2C + GPIO lines | `devices: [/dev/i2c-1, /dev/gpiochip0]` | none |
| + sysfs PWM | add `gpio: true` and an `apparmor.txt` that allows writes to `/sys/devices/platform/soc/fe20c000.pwm/**` (or `apparmor: false`) | +1 with a custom profile, −1 with AppArmor off |

Device nodes are `root:root 0600` on the host (`ls -la /dev/i2c*` output in [operating-system#4752](https://github.com/home-assistant/operating-system/issues/4752), maintainer comment 2026-06-10), so `igor` must run as root in its container. Gating is by cgroup and AppArmor, not by file mode.

## 3. HAOS rpi4-64 kernel and overlays

Kernel = RPi `bcm2711_defconfig` + HAOS fragments ([OS:…/configs/rpi4_64_defconfig](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/configs/rpi4_64_defconfig)). None of the HAOS fragments override the options below ([OS:…/kernel/v6.18.y/](https://github.com/home-assistant/operating-system/tree/18.5/buildroot-external/kernel/v6.18.y), [board/raspberrypi/kernel.config](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/board/raspberrypi/kernel.config)).

| Feature | Option | Value | Source |
|---|---|---|---|
| `i2c-dev` | `CONFIG_I2C_CHARDEV` | `m` | [bcm2711_defconfig L768](https://github.com/raspberrypi/linux/blob/9b488bc7a1e9b61713082f3e6c4361988d62f77b/arch/arm64/configs/bcm2711_defconfig#L768); `.ko` present on 17.3 per maintainer ([#4752](https://github.com/home-assistant/operating-system/issues/4752)) |
| I2C controller | `CONFIG_I2C_BCM2835` | `m` | [L773](https://github.com/raspberrypi/linux/blob/9b488bc7a1e9b61713082f3e6c4361988d62f77b/arch/arm64/configs/bcm2711_defconfig#L773) |
| GPIO chardev | `CONFIG_GPIO_CDEV`, `_V1` | `y` (Kconfig default, not overridden) | [drivers/gpio/Kconfig L83-98](https://github.com/raspberrypi/linux/blob/9b488bc7a1e9b61713082f3e6c4361988d62f77b/drivers/gpio/Kconfig#L83-L98) |
| GPIO sysfs | `CONFIG_GPIO_SYSFS` | `y` | [OS:…/device-support.config L168-169](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/kernel/v6.18.y/device-support.config#L168-L169) |
| PWM core / `pwm-bcm2835` | `CONFIG_PWM`, `CONFIG_PWM_BCM2835` | `y`, `m` | [L1611-1612](https://github.com/raspberrypi/linux/blob/9b488bc7a1e9b61713082f3e6c4361988d62f77b/arch/arm64/configs/bcm2711_defconfig#L1611-L1612) |
| PoE HAT fan | `CONFIG_PWM_RASPBERRYPI_POE`, `CONFIG_SENSORS_PWM_FAN`, `CONFIG_RPI_POE_POWER` | `m`, `m`, `m` | [L1617, L842, L827](https://github.com/raspberrypi/linux/blob/9b488bc7a1e9b61713082f3e6c4361988d62f77b/arch/arm64/configs/bcm2711_defconfig#L1617) |
| UniPi EEPROM (0x50) kernel driver, if wanted | `CONFIG_EEPROM_AT24` | `m` | [L486](https://github.com/raspberrypi/linux/blob/9b488bc7a1e9b61713082f3e6c4361988d62f77b/arch/arm64/configs/bcm2711_defconfig#L486) |
| Overlays `rpi-poe`, `rpi-poe-plus`, `pwm`, `pwm-2chan` | `BR2_PACKAGE_RPI_FIRMWARE_INSTALL_DTB_OVERLAYS` (default `y`) copies every firmware `.dtbo` to `/boot/overlays` | present | [Config.in L91-93](https://github.com/home-assistant/buildroot/blob/5c344b3789e3f6d620cf11ffab60e522a875cc79/package/rpi-firmware/Config.in#L91-L93), [rpi-firmware.mk L55-61](https://github.com/home-assistant/buildroot/blob/5c344b3789e3f6d620cf11ffab60e522a875cc79/package/rpi-firmware/rpi-firmware.mk#L55-L61), [haos-hook.sh](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/board/raspberrypi/haos-hook.sh); files exist at [firmware f2491b7/boot/overlays](https://github.com/raspberrypi/firmware/tree/f2491b71ed49880cc8285e41e03f81b163c6cd59/boot/overlays) |

PoE fan curve: the `rpi-poe` overlay takes `poe_fan_temp0..3` and `_hyst` params, plus `i2c` to drive the fan from Linux instead of the firmware ([README L4803-4823](https://github.com/raspberrypi/firmware/blob/f2491b71ed49880cc8285e41e03f81b163c6cd59/boot/overlays/README#L4803-L4823)). The PoE HAT EEPROM auto-loads the overlay; params then go in `dtparam=` lines ([raspberrypi/firmware#1689](https://github.com/raspberrypi/firmware/issues/1689), RPi issue tracker). Whether the HAT is "PoE" or "PoE+" decides `rpi-poe` vs `rpi-poe-plus`.

PWM and audio: the README warns that "the onboard analogue audio output uses both PWM channels", and HAOS ships `dtparam=audio=on` ([README L4449-4450](https://github.com/raspberrypi/firmware/blob/f2491b71ed49880cc8285e41e03f81b163c6cd59/boot/overlays/README#L4449-L4450), [config.txt L72](https://github.com/home-assistant/operating-system/blob/18.5/buildroot-external/board/raspberrypi/config.txt#L72)). Plan for `dtparam=audio=off` if PWM0 misbehaves (not tested).

## 4. Custom app repository and updates

| Piece | Requirement | Source |
|---|---|---|
| Repo root | `repository.yaml` with `name` (required), `url`, `maintainer` | [docs/apps/repository.md](https://github.com/home-assistant/developers.home-assistant/blob/50b518898989b67bb76b69827f227aa6f0acd32a/docs/apps/repository.md) |
| App folder | One folder per app holding `config.yaml` (Supervisor searches recursively; do not reuse that filename elsewhere); optional `apparmor.txt` in the same folder | [configuration.md L129-131](https://github.com/home-assistant/developers.home-assistant/blob/50b518898989b67bb76b69827f227aa6f0acd32a/docs/apps/configuration.md#L129-L131), [apps/model.py L728-730](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/apps/model.py#L728-L730) |
| Required keys | `name`, `version`, `slug`, `description`, `arch` (`aarch64`, `amd64`; older values are deprecated with a warning) | [configuration.md L133-141](https://github.com/home-assistant/developers.home-assistant/blob/50b518898989b67bb76b69827f227aa6f0acd32a/docs/apps/configuration.md#L133-L141), [apps/validate.py L239-243](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/apps/validate.py#L239-L243) |
| Prebuilt image | `image: ghcr.io/<org>/igor` (multi-arch manifest preferred; `{arch}` placeholder still accepted); `version` must equal the image tag | [publishing.md L23-46](https://github.com/home-assistant/developers.home-assistant/blob/50b518898989b67bb76b69827f227aa6f0acd32a/docs/apps/publishing.md#L23-L46), [configuration.md L138, L182](https://github.com/home-assistant/developers.home-assistant/blob/50b518898989b67bb76b69827f227aa6f0acd32a/docs/apps/configuration.md#L182) |
| Store refresh | Supervisor reloads repositories every 3 h (`RUN_RELOAD_APPS = 10800`) | [SUP:supervisor/misc/tasks.py L38](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/misc/tasks.py#L38) |
| Update | Shown when repo `version` > installed. Auto-update is opt-in per app and runs every 16 h. It skips versions under 1 day old, versions that cross `breaking_versions`, and options that fail the new schema | [tasks.py L31, L109-137](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/misc/tasks.py#L109-L137), [apps/app.py L494-496](https://github.com/home-assistant/supervisor/blob/98a2322374ae3d0c248079d11f7435ad759935fa/supervisor/apps/app.py#L494-L496) |

## 5. Known breakages and reports

| Report | Relevance | Status |
|---|---|---|
| [operating-system#4752](https://github.com/home-assistant/operating-system/issues/4752): "i2c-dev absent" on Pi 4, HAOS 17.3 | User error. The maintainer showed `i2c-dev.ko` is in the read-only rootfs, and that the terminal method needs the physical console, not an SSH app. The thread also documents the `haos-config` vs `modules-load` ordering | closed 2026-06-10 |
| [home-assistant.io#46698](https://github.com/home-assistant/home-assistant.io/pull/46698) | I2C how-to removed from the docs; the procedure is now undocumented-but-working | merged 2026-07-06 |
| [operating-system#4846](https://github.com/home-assistant/operating-system/issues/4846): Touch Display 2 I2C probe race on 18.1 | Kernel driver probe ordering, not userspace I2C. Shows RPi kernel bumps can shift I2C behaviour | closed 2026-11-09 |
| [supervisor#4560](https://github.com/home-assistant/supervisor/issues/4560) | `/sys` read-only by design; the `gpio` rw mounts are slated for removal. **Risk for sysfs PWM** | open (2024-02) |
| [supervisor#5839](https://github.com/home-assistant/supervisor/issues/5839) | `/sys/class/gpio` write → `Read-only file system` in an app | closed stale 2025-07 |
| [operating-system#4706](https://github.com/home-assistant/operating-system/issues/4706) | RPi kernel moved to 6.18 in HAOS 18 | closed 2026-06-28 |

I found no reports for GPIO chardev in apps or for the `rpi-poe` overlay on HAOS (searched both repos' issues on 2026-11-15).

## Could not confirm from a primary source

| Claim | Why open |
|---|---|
| sysfs PWM writes succeed with `gpio: true` + custom AppArmor | Inferred from mounts, DT and AppArmor rules; needs an on-device test |
| `dtparam=audio=on` actually conflicts with PWM0 on GPIO18 on the Pi 4 | The README warns in general terms only |
| PoE HAT EEPROM auto-loads `rpi-poe` under HAOS's U-Boot chain, and `dtparam` overrides apply | The firmware applies HAT overlays before U-Boot and U-Boot reuses that FDT; the HAT EEPROM behaviour comes from an RPi issue, not docs |
| `i2c_vc=on` is harmless alongside the PoE HAT | Not researched; recommendation is to omit it |
| macOS `._*` files break `modules-load.d` | Precaution, not tested |
| How long the Supervisor keeps the `gpio` sysfs mounts | Only a 2023 maintainer statement of intent |
