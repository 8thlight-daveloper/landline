package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	oldSwitchboardSecret = "old-secret-old-secret-0001"
	newSwitchboardSecret = "new-secret-new-secret-0002"
)

type seenAuthorizations struct {
	mu    sync.Mutex
	lines []string
}

func (s *seenAuthorizations) add(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
}

func (s *seenAuthorizations) containing(text string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var found []string
	for _, line := range s.lines {
		if strings.Contains(line, text) {
			found = append(found, line)
		}
	}
	return found
}

func writeSwitchboardConfig(t *testing.T, home, address, secretText string) {
	t.Helper()
	host, port, err := net.SplitHostPort(strings.TrimPrefix(address, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(home, ".config", "landline")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("secret = %q\nhost = %q\nport = %s\n", secretText, host, port)
	if err := os.WriteFile(filepath.Join(directory, "config.toml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func homeOnlyFromConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LANDLINE_URL", "")
	t.Setenv("LANDLINE_SECRET", "")
	t.Setenv("LANDLINE_PORT", "")
	return home
}

func TestAfterTheConfigNamesANewSwitchboardTheScreenRegistersThereAndTheOldAddressNeverReceivesTheNewSecret(t *testing.T) {
	home := homeOnlyFromConfig(t)
	var newSeen seenAuthorizations
	newer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newSeen.add(r)
		switch {
		case r.URL.Path == "/v1/observers" && r.Header.Get("Authorization") == "Bearer "+newSwitchboardSecret:
			w.Write([]byte(`{"token":"new-observer-token"}`))
		case r.URL.Path == "/v1/agents" && r.Header.Get("Authorization") == "Bearer new-observer-token":
			w.Write([]byte(`[{"id":"bravo","harness":"codex","host":"mac","connected":true}]`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(newer.Close)
	var oldSeen seenAuthorizations
	older := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldSeen.add(r)
		switch {
		case r.URL.Path == "/v1/observers" && r.Header.Get("Authorization") == "Bearer "+oldSwitchboardSecret:
			w.Write([]byte(`{"token":"old-observer-token"}`))
		case r.URL.Path == "/v1/agents":
			writeSwitchboardConfig(t, home, newer.URL, newSwitchboardSecret)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(older.Close)
	writeSwitchboardConfig(t, home, older.URL, oldSwitchboardSecret)

	var out, errs bytes.Buffer
	run([]string{"agents", "--json"}, &out, &errs)

	if leaked := oldSeen.containing(newSwitchboardSecret); len(leaked) > 0 {
		t.Errorf("the old switchboard received the new switchboard's secret %d times", len(leaked))
	}
	if leaked := oldSeen.containing("new-observer-token"); len(leaked) > 0 {
		t.Errorf("the old switchboard received the new switchboard's observer token %d times", len(leaked))
	}
	if registered := newSeen.containing("/v1/observers Bearer " + newSwitchboardSecret); len(registered) == 0 {
		t.Errorf("the screen never registered at the address the config names now")
	}
	if !strings.Contains(out.String(), "bravo") {
		t.Errorf("the screen does not list the new switchboard's agents: out %q err %q", out.String(), errs.String())
	}
}

func TestAfterTheConfigNamesANewSwitchboardTheScreenSignsOnlyForTheNewAddress(t *testing.T) {
	home := homeOnlyFromConfig(t)
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var newSeen seenAuthorizations
	newer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newSeen.add(r)
		switch {
		case r.URL.Path == "/v1/observers" && strings.HasPrefix(r.Header.Get("Authorization"), "Landline-Machine name=linux-box,"):
			w.Write([]byte(`{"token":"new-observer-token"}`))
		case r.URL.Path == "/v1/agents" && r.Header.Get("Authorization") == "Bearer new-observer-token":
			w.Write([]byte(`[{"id":"bravo","harness":"codex","host":"mac","connected":true}]`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(newer.Close)
	directory := filepath.Join(home, ".config", "landline")
	var oldSeen seenAuthorizations
	older := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldSeen.add(r)
		switch {
		case r.URL.Path == "/v1/observers" && r.Header.Get("Authorization") == "Bearer "+oldSwitchboardSecret:
			w.Write([]byte(`{"token":"old-observer-token"}`))
		case r.URL.Path == "/v1/agents":
			host, port, _ := net.SplitHostPort(strings.TrimPrefix(newer.URL, "http://"))
			os.WriteFile(filepath.Join(directory, "machine.key"), []byte(base64.StdEncoding.EncodeToString(key.Seed())+"\n"), 0o600)
			os.WriteFile(filepath.Join(directory, "config.toml"), []byte(fmt.Sprintf("host = %q\nport = %s\nmachine = \"linux-box\"\n", host, port)), 0o600)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(older.Close)
	writeSwitchboardConfig(t, home, older.URL, oldSwitchboardSecret)

	var out, errs bytes.Buffer
	run([]string{"agents", "--json"}, &out, &errs)

	if signed := oldSeen.containing("Landline-Machine"); len(signed) > 0 {
		t.Errorf("the old switchboard received %d requests signed with the key that the config now pairs with another switchboard", len(signed))
	}
	if registered := newSeen.containing("/v1/observers Landline-Machine name=linux-box,"); len(registered) == 0 {
		t.Errorf("the screen never registered with its key at the address the config names now")
	}
	var view struct{ Agents []struct{ ID string } }
	if json.Unmarshal(out.Bytes(), &view) != nil || len(view.Agents) != 1 || view.Agents[0].ID != "bravo" {
		t.Errorf("the screen does not list the new switchboard's agents: out %q err %q", out.String(), errs.String())
	}
}
