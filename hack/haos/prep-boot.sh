#!/bin/bash
# Boot-partition edits for `home` before first boot (lancre docs/research/haos-hardware-access.md).
set -euo pipefail
B=/Volumes/hassos-boot
[ -f "$B/config.txt" ] || { echo "ABORT: $B/config.txt missing"; exit 1; }
tail -n 3 "$B/config.txt" | grep -q '^\[all\]' || echo "WARN: config.txt does not end in [all]; check placement"
grep -q 'lancre home' "$B/config.txt" || cat >> "$B/config.txt" <<'CFG'
# lancre home: I2C for igor (UniPi 1.1), PoE HAT fan curve as on Raspbian, heartbeat LED
[all]
dtparam=i2c_arm=on
dtoverlay=rpi-poe
dtparam=poe_fan_temp0=30000,poe_fan_temp0_hyst=1000
dtparam=poe_fan_temp1=50000,poe_fan_temp1_hyst=5000
dtparam=poe_fan_temp2=75000,poe_fan_temp2_hyst=5000
dtparam=poe_fan_temp3=80000,poe_fan_temp3_hyst=5000
dtparam=act_led_trigger=heartbeat
CFG
mkdir -p "$B/CONFIG/modules"
printf 'i2c-dev\n' > "$B/CONFIG/modules/rpi-i2c.conf"
cp "${SSH_PUBKEY:-$HOME/.ssh/id_rsa.pub}" "$B/CONFIG/authorized_keys"
dot_clean -m "$B"; find "$B" -name '._*' -delete
find "$B/CONFIG" -type f; tail -n 12 "$B/config.txt"
