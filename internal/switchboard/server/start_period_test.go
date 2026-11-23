package server

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestTheSwitchboardRefusesEverySignedRequestTimedUpToFiveSecondsAfterItsStartSecondWhateverTheWindow(t *testing.T) {
	for _, window := range []time.Duration{time.Second, 5 * time.Second} {
		t.Run(window.String(), func(t *testing.T) {
			h := startWith(t, func(cfg *Config) { cfg.SignatureWindow = window })
			startSecond := h.now.Unix()
			key := h.joinMachine("linux-box")
			body := []byte(`{"kind":"tui"}`)
			h.advance(300 * time.Millisecond)
			for offset := int64(0); offset <= 5; offset++ {
				authorization := machineAuthorization(key, "linux-box", startSecond+offset, "POST", "/v1/observers", body)
				status, raw := h.sendWithAuthorization("POST", "/v1/observers", authorization, body)
				if status != http.StatusUnauthorized || textField(refusalFields(t, raw), "reason") != "refused" {
					t.Errorf("a time %d seconds after the start second, arriving with it: %d %s, want 401 and reason refused", offset, status, raw)
				}
				h.advance(time.Second)
			}
			late := machineAuthorization(key, "linux-box", startSecond+5, "POST", "/v1/observers", body)
			if status, raw := h.sendWithAuthorization("POST", "/v1/observers", late, body); status != http.StatusUnauthorized {
				t.Errorf("a time 5 seconds after the start second, arriving one second after the start period: %d %s, want 401", status, raw)
			}
			after := machineAuthorization(key, "linux-box", startSecond+6, "POST", "/v1/observers", body)
			if status, raw := h.sendWithAuthorization("POST", "/v1/observers", after, body); status != http.StatusOK {
				t.Errorf("a time 6 seconds after the start second: %d %s, want 200", status, raw)
			}
		})
	}
}

func TestACopyOfARequestFromAMachineWhoseClockRunsAheadIsRefusedByTheProcessAfterARestart(t *testing.T) {
	machinesPath := filepath.Join(t.TempDir(), "machines.json")
	first := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	clock := &settableClock{now: first}
	onClock := func(cfg *Config) {
		cfg.Now = clock.read
		cfg.MachinesPath = machinesPath
		cfg.SignatureWindow = 5 * time.Second
	}
	old := startWith(t, onClock)
	key := old.joinMachine("linux-box")
	body := []byte(`{"id":"alpha","harness":"claude-code","host":"mac"}`)
	clock.set(first.Add(10500 * time.Millisecond))
	aheadByFour := first.Unix() + 14
	authorization := machineAuthorization(key, "linux-box", aheadByFour, "POST", "/v1/register", body)
	if status, raw := old.sendWithAuthorization("POST", "/v1/register", authorization, body); status != http.StatusOK {
		t.Fatalf("the process before the restart refused a clock 4 seconds ahead inside the 5 second window: %d %s", status, raw)
	}
	old.srv.Close()
	clock.set(first.Add(11200 * time.Millisecond))
	restarted := startWith(t, onClock)
	clock.set(first.Add(11500 * time.Millisecond))
	if status, raw := restarted.sendWithAuthorization("POST", "/v1/register", authorization, body); status != http.StatusUnauthorized {
		t.Errorf("a copy of a request that the process before the restart accepted, timed 3 seconds after the new start second, reached a process with an empty memory: %d %s, want 401", status, raw)
	}
	clock.set(first.Add(17300 * time.Millisecond))
	fresh := machineAuthorization(key, "linux-box", first.Unix()+17, "POST", "/v1/register", body)
	if status, raw := restarted.sendWithAuthorization("POST", "/v1/register", fresh, body); status != http.StatusOK {
		t.Errorf("a new request timed after the start period: %d %s, want 200", status, raw)
	}
}
