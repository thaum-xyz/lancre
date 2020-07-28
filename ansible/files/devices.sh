#!/bin/bash

METRICS_FILE="/var/lib/node_exporter/devices.prom"

TOUCHSCREEN=0
if [ -f '/sys/devices/platform/soc/soc:firmware/soc:firmware:touchscreen/input/input0/power/control' ]; then
        TOUCHSCREEN=1
fi

cat <<EOF > "$METRICS_FILE"
# HELP devices_input_touchscreen_up Check if touchscreen was detected
# TYPE devices_input_touchscreen_up gauge
devices_input_touchscreen_up $TOUCHSCREEN
EOF
