package server

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func TestTheStartPeriodBeginsWhenTheSwitchboardListensAndNotWhenItIsMade(t *testing.T) {
	made := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	clock := &settableClock{now: made}
	h := startNotListening(t, func(cfg *Config) {
		cfg.Now = clock.read
		cfg.SignatureWindow = 5 * time.Second
	})
	key := h.joinMachine("linux-box")
	listened := made.Add(20 * time.Second)
	clock.set(listened)
	h.hub.Listening()
	body := []byte(`{"kind":"tui"}`)
	clock.set(listened.Add(1200 * time.Millisecond))
	early := machineAuthorization(key, "linux-box", listened.Unix()+1, "POST", "/v1/observers", body)
	if status, raw := h.sendWithAuthorization("POST", "/v1/observers", early, body); status != http.StatusUnauthorized {
		t.Errorf("a request timed 1 second after the switchboard began to listen, and 21 seconds after it was made: %d %s, want 401", status, raw)
	}
	clock.set(listened.Add(6200 * time.Millisecond))
	after := machineAuthorization(key, "linux-box", listened.Unix()+6, "POST", "/v1/observers", body)
	if status, raw := h.sendWithAuthorization("POST", "/v1/observers", after, body); status != http.StatusOK {
		t.Errorf("a request timed 6 seconds after the switchboard began to listen: %d %s, want 200", status, raw)
	}
}

func TestACopyWhoseBodyArrivesAfterALaterRequestForgotItsSignatureIsRefused(t *testing.T) {
	clock := &settableClock{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	h := startWith(t, func(cfg *Config) {
		cfg.Now = clock.read
		cfg.SignatureWindow = 5 * time.Second
	})
	key := h.joinMachine("linux-box")
	signed := clock.read().Add(pastTheStartPeriod).Truncate(time.Second)
	clock.set(signed)
	body := []byte(`{"kind":"tui"}`)
	original := machineAuthorization(key, "linux-box", signed.Unix(), "POST", "/v1/observers", body)
	if status, raw := h.sendWithAuthorization("POST", "/v1/observers", original, body); status != http.StatusOK {
		t.Fatalf("the original request: %d %s", status, raw)
	}

	slowBody, feed := io.Pipe()
	copyRequest, _ := http.NewRequest("POST", h.srv.URL+"/v1/observers", slowBody)
	copyRequest.Header.Set("Authorization", original)
	answered := make(chan int, 1)
	go func() {
		resp, err := http.DefaultClient.Do(copyRequest)
		if err != nil {
			answered <- -1
			return
		}
		resp.Body.Close()
		answered <- resp.StatusCode
	}()
	feed.Write(body[:1])
	time.Sleep(300 * time.Millisecond)

	clock.set(signed.Add(8 * time.Second))
	later := machineAuthorization(key, "linux-box", signed.Unix()+8, "POST", "/v1/observers", body)
	if status, raw := h.sendWithAuthorization("POST", "/v1/observers", later, body); status != http.StatusOK {
		t.Fatalf("the later request: %d %s", status, raw)
	}

	feed.Write(body[1:])
	feed.Close()
	if status := <-answered; status != http.StatusUnauthorized {
		t.Errorf("a copy of an accepted request whose body arrived after a later request forgot its signature got %d, want 401", status)
	}
}
