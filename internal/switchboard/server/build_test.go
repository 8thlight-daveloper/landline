package server

import (
	"encoding/json"
	"testing"
)

func TestBuildAnswersTheConfiguredValuesToTheSecretAndAnObserverAndRefusesTheRest(t *testing.T) {
	want := BuildResponse{Version: "0.2.0", Commit: "abc", Date: "2026-11-12T16:34:00Z"}
	h := startWith(t, func(cfg *Config) { cfg.Build = want })
	agent := h.register("alpha")
	observer := h.registerObserver("tui")
	for name, token := range map[string]string{"secret": secret, "observer": observer} {
		status, raw := h.call("GET", "/v1/build", token, nil)
		var got BuildResponse
		if err := json.Unmarshal(raw, &got); status != 200 || err != nil || got != want {
			t.Errorf("%s: %d %s", name, status, raw)
		}
	}
	for token, status := range map[string]int{agent: 403, "": 401, "wrong": 401} {
		if got, raw := h.call("GET", "/v1/build", token, nil); got != status {
			t.Errorf("token %q: %d %s, want %d", token, got, raw, status)
		}
	}
	if status, _ := h.call("POST", "/v1/build", secret, map[string]string{}); status == 200 {
		t.Error("POST answered 200")
	}
}
