// Package supervisor asks the Home Assistant Supervisor for the MQTT broker
// it gave the app (`services: mqtt:need` in haos/igor/config.yaml).
package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// URL is where Supervisor answers apps; it sets SUPERVISOR_TOKEN for them.
const URL = "http://supervisor"

type Broker struct {
	URL, Username, Password string
}

// MQTT returns the broker of the app that provides the mqtt service.
func MQTT(ctx context.Context, base, token string) (Broker, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/services/mqtt", nil)
	if err != nil {
		return Broker{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Broker{}, err
	}
	defer resp.Body.Close()
	var body struct {
		Result  string `json:"result"`
		Message string `json:"message"`
		Data    struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			SSL      bool   `json:"ssl"`
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Broker{}, fmt.Errorf("services/mqtt: HTTP %d: %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || body.Result != "ok" {
		return Broker{}, fmt.Errorf("services/mqtt: HTTP %d: %s", resp.StatusCode, body.Message)
	}
	d := body.Data
	scheme := "tcp"
	if d.SSL {
		scheme = "ssl"
	}
	return Broker{
		URL:      scheme + "://" + d.Host + ":" + strconv.Itoa(d.Port),
		Username: d.Username,
		Password: d.Password,
	}, nil
}
