package supervisor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMQTTReadsTheMosquittoAppsBroker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/mqtt" || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Shape of Supervisor's answer for the Mosquitto app.
		w.Write([]byte(`{"result":"ok","data":{"host":"core-mosquitto","port":1883,"ssl":false,"protocol":"3.1.1","username":"addons","password":"pw","addon":"core_mosquitto"}}`))
	}))
	defer srv.Close()

	got, err := MQTT(context.Background(), srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	want := Broker{URL: "tcp://core-mosquitto:1883", Username: "addons", Password: "pw"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMQTTFailsWithoutTheService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"result":"error","message":"Service not enabled"}`))
	}))
	defer srv.Close()

	_, err := MQTT(context.Background(), srv.URL, "tok")
	if err == nil || !strings.Contains(err.Error(), "Service not enabled") {
		t.Fatalf("err = %v", err)
	}
}
