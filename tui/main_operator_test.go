package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func tmuxHub(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/observers":
			w.Write([]byte(`{"token":"observer-token-xyz"}`))
		case "/v1/operators":
			if r.Header.Get("Authorization") != "Bearer "+secret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"token":"operator-token-xyz"}`))
		case "/v1/operator/send":
			if r.Header.Get("Authorization") != "Bearer operator-token-xyz" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			json.NewEncoder(w).Encode(map[string]any{"id": "m-1", "seq": 4, "recipients": []string{body["to"]}})
		default:
			w.Write([]byte(`[{"name":"alpha","harness":"claude","host":"mac","connected":true,"label":"lead","tmux_socket":"default","tmux_pane":"%3"},{"name":"bravo","harness":"codex","host":"mac"}]`))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestAgentsJSONCarriesTheTmuxSocketAndPaneOfARowAndNothingForARowWithNone(t *testing.T) {
	server := tmuxHub(t)
	code, out, errs := invoke(t, server.URL, "agents", "--json")
	if code != 0 || errs != "" {
		t.Fatalf("code %d stderr %q", code, errs)
	}
	var view struct{ Agents []map[string]any }
	if err := json.Unmarshal([]byte(out), &view); err != nil || len(view.Agents) != 2 {
		t.Fatalf("%v: %s", err, out)
	}
	if view.Agents[0]["handle"] != "alpha" || view.Agents[0]["tmux_socket"] != "default" || view.Agents[0]["tmux_pane"] != "%3" {
		t.Fatalf("row %v", view.Agents[0])
	}
	for _, key := range []string{"tmux_socket", "tmux_pane"} {
		if _, present := view.Agents[1][key]; present {
			t.Errorf("%s is present in %v", key, view.Agents[1])
		}
	}
}

func TestAgentsTextAddsTheTmuxItemToTheAttributeField(t *testing.T) {
	server := tmuxHub(t)
	code, out, errs := invoke(t, server.URL, "agents")
	if code != 0 || errs != "" {
		t.Fatalf("code %d stderr %q", code, errs)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("%q", out)
	}
	alpha := strings.Split(lines[0], "\t")
	if len(alpha) != 5 || alpha[0] != "alpha" || !strings.Contains(alpha[4], "label: lead") || !strings.Contains(alpha[4], "tmux: default:%3") {
		t.Fatalf("alpha: %q", lines[0])
	}
	if strings.Contains(lines[1], "tmux") {
		t.Fatalf("bravo: %q", lines[1])
	}
}

func TestOperatorSendThroughTheLandlineBinaryRegistersAnOperatorSendsAndPrintsTheResult(t *testing.T) {
	server := tmuxHub(t)
	code, out, errs := invoke(t, server.URL, "operator", "send", "--to", "alpha", "--body", "hello")
	if code != 0 || errs != "" || out != "sent m-1 (seq 4) to alpha\n" {
		t.Fatalf("code %d out %q err %q", code, out, errs)
	}
	for _, leaked := range []string{secret, "operator-token"} {
		if strings.Contains(out+errs, leaked) {
			t.Fatalf("%q leaked", leaked)
		}
	}
}
