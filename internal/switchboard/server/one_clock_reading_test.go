package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestARedeemWhoseCodeExpiresWhileItsBodyIsArrivingIsRefused(t *testing.T) {
	clock := &settableClock{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	h := startWith(t, func(cfg *Config) {
		cfg.Now = clock.read
		cfg.InviteLifetime = 5 * time.Minute
	})
	invited := clock.read()
	status, raw := h.call("POST", "/v1/machines", secret, map[string]string{"name": "linux-box"})
	if status != http.StatusOK {
		t.Fatalf("invite: %d %s", status, raw)
	}
	var made struct{ Code string }
	json.Unmarshal(raw, &made)
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	body, _ := json.Marshal(map[string]string{"code": made.Code, "public_key": base64.StdEncoding.EncodeToString(public)})

	clock.set(invited.Add(5*time.Minute - time.Second))
	slowBody, feed := io.Pipe()
	redeem, _ := http.NewRequest("PATCH", h.srv.URL+"/v1/machines", slowBody)
	answered := make(chan int, 1)
	go func() {
		resp, err := http.DefaultClient.Do(redeem)
		if err != nil {
			answered <- -1
			return
		}
		resp.Body.Close()
		answered <- resp.StatusCode
	}()
	feed.Write(body[:1])
	time.Sleep(300 * time.Millisecond)
	clock.set(invited.Add(5*time.Minute + time.Second))
	feed.Write(body[1:])
	feed.Close()
	if got := <-answered; got != http.StatusUnauthorized {
		t.Errorf("a redeem whose body finished arriving after its code expired got %d, want 401", got)
	}
}

func TestASignedRegistrationAndARedeemEachReadTheClockOnce(t *testing.T) {
	var reads atomic.Int32
	clock := &settableClock{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	h := startWith(t, func(cfg *Config) {
		cfg.Now = func() time.Time { reads.Add(1); return clock.read() }
	})
	key := h.joinMachine("linux-box")
	clock.set(clock.read().Add(pastTheStartPeriod))

	status, raw := h.call("POST", "/v1/machines", secret, map[string]string{"name": "bravo-box"})
	if status != http.StatusOK {
		t.Fatalf("invite: %d %s", status, raw)
	}
	var made struct{ Code string }
	json.Unmarshal(raw, &made)
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	before := reads.Load()
	if status, raw := h.call("PATCH", "/v1/machines", "", map[string]string{"code": made.Code, "public_key": base64.StdEncoding.EncodeToString(public)}); status != http.StatusOK {
		t.Fatalf("redeem: %d %s", status, raw)
	}
	if got := reads.Load() - before; got != 1 {
		t.Errorf("a redeem read the clock %d times, want 1", got)
	}

	before = reads.Load()
	register := []byte(`{"id":"alpha","harness":"claude-code","host":"mac"}`)
	authorization := machineAuthorization(key, "linux-box", clock.read().Unix(), "POST", "/v1/register", register)
	if status, raw := h.sendWithAuthorization("POST", "/v1/register", authorization, register); status != http.StatusOK {
		t.Fatalf("signed registration: %d %s", status, raw)
	}
	if got := reads.Load() - before; got != 1 {
		t.Errorf("a signed registration read the clock %d times, want 1", got)
	}
}
