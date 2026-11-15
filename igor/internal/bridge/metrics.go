package bridge

import "github.com/prometheus/client_golang/prometheus"

// Metrics mirror the Evok exporter's names and labels (job
// scrapeConfig/lancre/evok) so both can be compared with a plain join.
type Metrics struct {
	relay, input, analog, temp *prometheus.GaugeVec
	analogOut                  *prometheus.GaugeVec
	locked                     *prometheus.GaugeVec
	failsafes, refused, drift  *prometheus.CounterVec
	busErrors                  *prometheus.CounterVec
	connected, silent          prometheus.Gauge
	lastHeartbeat              prometheus.Gauge
}

func NewMetrics(reg prometheus.Registerer, version, mode string) *Metrics {
	gauge := func(name, help string, labels ...string) *prometheus.GaugeVec {
		g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
		reg.MustRegister(g)
		return g
	}
	counter := func(name, help string, labels ...string) *prometheus.CounterVec {
		c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
		reg.MustRegister(c)
		return c
	}
	m := &Metrics{
		relay:         gauge("igor_relay_state_boolean", "Relay latch state read back from the chip.", "circuit"),
		input:         gauge("igor_digital_input_state_boolean", "Digital input state after debounce.", "circuit"),
		analog:        gauge("igor_analog_input_volts", "Analog input voltage.", "circuit"),
		temp:          gauge("igor_temperature_celsius", "DS18B20 temperature.", "address"),
		analogOut:     gauge("igor_analog_output_volts", "Analog output voltage being driven.", "circuit"),
		locked:        gauge("igor_relay_locked", "1 after a max_on trip until an explicit off.", "circuit"),
		failsafes:     counter("igor_relay_failsafe_total", "Relays switched by igor itself.", "circuit", "reason"),
		refused:       counter("igor_relay_commands_refused_total", "Commands igor did not carry out.", "circuit", "reason"),
		drift:         counter("igor_relay_drift_total", "Times the chip disagreed with igor in active mode.", "circuit"),
		busErrors:     counter("igor_bus_errors_total", "Failed hardware reads and writes.", "bus"),
		connected:     gauge("igor_mqtt_connected", "1 while connected to the broker.").WithLabelValues(),
		silent:        gauge("igor_ha_silent", "1 while Home Assistant is silent.").WithLabelValues(),
		lastHeartbeat: gauge("igor_ha_last_heartbeat_timestamp_seconds", "Last heartbeat; igor's start time before the first one.").WithLabelValues(),
	}
	gauge("igor_build_info", "Version and mode.", "version", "mode").WithLabelValues(version, mode).Set(1)
	for _, bus := range []string{"i2c", "onewire", "pwm"} {
		m.busErrors.WithLabelValues(bus)
	}
	return m
}
