// igor connects a UniPi 1.1 board to Home Assistant over MQTT.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/thaum-xyz/lancre/igor/internal/bridge"
	"github.com/thaum-xyz/lancre/igor/internal/config"
	"github.com/thaum-xyz/lancre/igor/internal/hass"
	"github.com/thaum-xyz/lancre/igor/internal/supervisor"
	"github.com/thaum-xyz/lancre/igor/internal/unipi"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	def := "/etc/igor/config.yaml"
	if p := os.Getenv("IGOR_CONFIG"); p != "" {
		def = p // the HAOS app points this at /data/options.json
	}
	path := flag.String("config", def, "path to the config file (YAML or JSON)")
	health := flag.String("healthcheck", "", "GET this URL, exit 0 on HTTP 200 (the image's HEALTHCHECK; it has no curl)")
	flag.Parse()
	if *health != "" {
		os.Exit(healthcheck(*health))
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(*path, log); err != nil {
		log.Error("igor stopped", "err", err)
		os.Exit(1)
	}
}

func run(path string, log *slog.Logger) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	// As a HAOS app without a configured user, take the Mosquitto app's broker.
	if tok := os.Getenv("SUPERVISOR_TOKEN"); tok != "" && cfg.MQTT.Username == "" {
		b, err := supervisor.MQTT(context.Background(), supervisor.URL, tok)
		if err != nil {
			return fmt.Errorf("mqtt broker from Supervisor: %w", err)
		}
		cfg.MQTT.URL, cfg.MQTT.Username, cfg.MQTT.Password = b.URL, b.Username, b.Password
	}
	log.Info("starting igor", "version", version, "mode", cfg.Mode, "board", cfg.Board, "broker", cfg.MQTT.URL)
	for _, w := range cfg.Warnings() {
		log.Warn(w)
	}

	var board bridge.Board
	if cfg.Board == "fake" {
		board = unipi.NewFake()
	} else {
		real, err := unipi.Open(log, cfg.Mode == "active")
		if err != nil {
			return err
		}
		if cfg.Mode == "active" {
			if err := real.MakeOutputs(); err != nil {
				return err
			}
		}
		board = real
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	pub := newPublisher(log)
	br, err := bridge.New(cfg, board, pub, bridge.NewMetrics(reg, version, cfg.Mode), log, version)
	if err != nil {
		return err
	}

	topics := hass.Topics{Prefix: cfg.MQTT.TopicPrefix, Discovery: cfg.MQTT.DiscoveryPrefix}
	onMessage := func(_ mqtt.Client, m mqtt.Message) { br.OnMessage(m.Topic(), m.Payload(), m.Retained()) }
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTT.URL).
		SetClientID(cfg.MQTT.ClientID).
		SetUsername(cfg.MQTT.Username).
		SetPassword(cfg.MQTT.Password).
		SetCleanSession(true).
		SetKeepAlive(30*time.Second).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5*time.Second).
		SetMaxReconnectInterval(30*time.Second).
		SetWriteTimeout(5*time.Second).
		SetWill(topics.Status(), hass.Offline, 1, true).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) { br.OnDisconnect(err) }).
		SetOnConnectHandler(func(c mqtt.Client) {
			subs := map[string]byte{topics.RelaySetAll(): 1, topics.Heartbeat(): 1, topics.HAStatus(): 1, topics.AnalogOutSetAll(): 1}
			if t := c.SubscribeMultiple(subs, onMessage); t.Wait() && t.Error() != nil {
				// Deaf to commands and heartbeats: stay "disconnected" so the
				// grace period runs, and force a fresh connection.
				log.Error("subscribe failed, reconnecting", "err", t.Error())
				go func() { c.Disconnect(0); c.Connect() }()
				return
			}
			br.OnConnect()
		})
	client := mqtt.NewClient(opts)
	pub.start(client)
	client.Connect() // retries in the background until the broker answers

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	srv := &http.Server{Addr: cfg.HTTP.Listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	httpErr := make(chan error, 1)
	go func() { httpErr <- srv.ListenAndServe() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() { br.Run(ctx); close(done) }()

	select {
	case <-ctx.Done():
	case err := <-httpErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http: %w", err)
		}
	}
	stop()
	<-done
	// Relays keep their state: the chip latches them and the next start
	// applies off_on_start.
	log.Info("shutting down; relays keep their state")
	client.Publish(topics.Status(), 1, true, hass.Offline).WaitTimeout(2 * time.Second)
	client.Disconnect(250)
	return srv.Close()
}

// publisher adapts paho to bridge.Publisher. Publish only queues, so the
// bridge never waits on the network while it holds its lock. A message that
// cannot be queued or sent is dropped: everything is re-announced on connect.
type publisher struct {
	client atomic.Pointer[mqtt.Client]
	queue  chan message
	log    *slog.Logger
}

type message struct {
	topic   string
	payload []byte
	retain  bool
}

func newPublisher(log *slog.Logger) *publisher {
	return &publisher{queue: make(chan message, 1024), log: log}
}

func (p *publisher) start(c mqtt.Client) {
	p.client.Store(&c)
	go func() {
		for m := range p.queue {
			if !c.IsConnectionOpen() {
				continue
			}
			t := c.Publish(m.topic, 1, m.retain, m.payload)
			go func() {
				if t.WaitTimeout(10*time.Second) && t.Error() != nil {
					p.log.Warn("publish failed", "topic", m.topic, "err", t.Error())
				}
			}()
		}
	}()
}

func (p *publisher) Connected() bool {
	c := p.client.Load()
	return c != nil && (*c).IsConnectionOpen()
}

func (p *publisher) Publish(topic string, payload []byte, retain bool) {
	select {
	case p.queue <- message{topic, payload, retain}:
	default:
		p.log.Warn("publish queue full, message dropped", "topic", topic)
	}
}

func healthcheck(url string) int {
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, resp.Status)
		return 1
	}
	return 0
}
