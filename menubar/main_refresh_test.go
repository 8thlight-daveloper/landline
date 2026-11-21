package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
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

func configDirectory(home string) string { return filepath.Join(home, ".config", "landline") }

func writeSwitchboardConfig(t *testing.T, home, address, secretText string) {
	t.Helper()
	host, port, err := net.SplitHostPort(strings.TrimPrefix(address, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDirectory(home), 0o700); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("secret = %q\nhost = %q\nport = %s\n", secretText, host, port)
	if err := os.WriteFile(filepath.Join(configDirectory(home), "config.toml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func homeOnlyFromConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "")
	t.Setenv("LANDLINE_URL", "")
	t.Setenv("LANDLINE_SECRET", "")
	t.Setenv("LANDLINE_PORT", "")
	return home
}

func newSwitchboardAnswering(t *testing.T, seen *seenAuthorizations, accepts func(*http.Request) bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.add(r)
		switch {
		case r.URL.Path == "/v1/observers" && accepts(r):
			w.Write([]byte(`{"token":"new-observer-token"}`))
		case r.Header.Get("Authorization") != "Bearer new-observer-token":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/v1/agents":
			w.Write([]byte(`[{"id":"bravo","harness":"codex","host":"mac","connected":true}]`))
		case r.URL.Path == "/v1/build":
			w.Write([]byte(`{"version":"1.2.3","commit":"abc1234","date":""}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func oldSwitchboardThatMovesTheConfig(t *testing.T, seen *seenAuthorizations, move func()) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.add(r)
		switch {
		case r.URL.Path == "/v1/observers" && r.Header.Get("Authorization") == "Bearer "+oldSwitchboardSecret:
			w.Write([]byte(`{"token":"old-observer-token"}`))
		case r.URL.Path == "/v1/agents" && r.Header.Get("Authorization") == "Bearer old-observer-token":
			move()
			w.Write([]byte(`[{"id":"alpha","harness":"claude","host":"mac","connected":true}]`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestAfterTheConfigNamesANewSwitchboardTheMenuBarRegistersThereAndTheOldAddressNeverReceivesTheNewSecret(t *testing.T) {
	home := homeOnlyFromConfig(t)
	var newSeen, oldSeen seenAuthorizations
	newer := newSwitchboardAnswering(t, &newSeen, func(r *http.Request) bool {
		return r.Header.Get("Authorization") == "Bearer "+newSwitchboardSecret
	})
	older := oldSwitchboardThatMovesTheConfig(t, &oldSeen, func() { writeSwitchboardConfig(t, home, newer.URL, newSwitchboardSecret) })
	writeSwitchboardConfig(t, home, older.URL, oldSwitchboardSecret)

	var out, errs bytes.Buffer
	run([]string{"menu", "--text"}, &out, &errs)

	if leaked := oldSeen.containing(newSwitchboardSecret); len(leaked) > 0 {
		t.Errorf("the old switchboard received the new switchboard's secret %d times", len(leaked))
	}
	if leaked := oldSeen.containing("new-observer-token"); len(leaked) > 0 {
		t.Errorf("the old switchboard received the new switchboard's observer token %d times", len(leaked))
	}
	if registered := newSeen.containing("/v1/observers Bearer " + newSwitchboardSecret); len(registered) == 0 {
		t.Errorf("the menu bar never registered at the address the config names now")
	}
}

func TestAfterTheConfigNamesANewSwitchboardTheMenuBarSignsOnlyForTheNewAddress(t *testing.T) {
	home := homeOnlyFromConfig(t)
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var newSeen, oldSeen seenAuthorizations
	newer := newSwitchboardAnswering(t, &newSeen, func(r *http.Request) bool {
		return strings.HasPrefix(r.Header.Get("Authorization"), "Landline-Machine name=linux-box,")
	})
	older := oldSwitchboardThatMovesTheConfig(t, &oldSeen, func() {
		host, port, _ := net.SplitHostPort(strings.TrimPrefix(newer.URL, "http://"))
		os.WriteFile(filepath.Join(configDirectory(home), "machine.key"), []byte(base64.StdEncoding.EncodeToString(key.Seed())+"\n"), 0o600)
		os.WriteFile(filepath.Join(configDirectory(home), "config.toml"), []byte(fmt.Sprintf("host = %q\nport = %s\nmachine = \"linux-box\"\n", host, port)), 0o600)
	})
	writeSwitchboardConfig(t, home, older.URL, oldSwitchboardSecret)

	var out, errs bytes.Buffer
	run([]string{"menu", "--text"}, &out, &errs)

	if signed := oldSeen.containing("Landline-Machine"); len(signed) > 0 {
		t.Errorf("the old switchboard received %d requests signed with the key that the config now pairs with another switchboard", len(signed))
	}
	if registered := newSeen.containing("/v1/observers Landline-Machine name=linux-box,"); len(registered) == 0 {
		t.Errorf("the menu bar never registered with its key at the address the config names now")
	}
}
