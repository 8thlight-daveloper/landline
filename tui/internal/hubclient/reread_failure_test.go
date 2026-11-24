package hubclient

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestWhenTheCredentialCannotBeReadAgainTheErrorNamesTheKeyFileAndTheHeldSecretIsNotSent(t *testing.T) {
	var registrations atomic.Int32
	var expired atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/observers":
			registrations.Add(1)
			w.Write([]byte(`{"token":"observer-token"}`))
		case expired.Load():
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unknown token"}`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(server.Close)
	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	home := t.TempDir()
	directory := filepath.Join(home, ".config", "landline")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	config := "secret = \"alpha-secret-0123456789abcdef\"\nhost = \"" + host + "\"\nport = " + port + "\nmachine = \"linux-box\"\n"
	if err := os.WriteFile(filepath.Join(directory, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(string) string { return "" }
	address, credential, err := DiscoverCredential(getenv, home)
	if err != nil {
		t.Fatal(err)
	}
	client := NewWith(address, credential)
	client.RereadCredentialWith(func() (string, Credential, error) { return DiscoverCredential(getenv, home) })
	if _, err := client.Agents(); err != nil || registrations.Load() != 1 {
		t.Fatalf("with the secret: %v, %d registrations", err, registrations.Load())
	}

	keyPath := filepath.Join(directory, "machine.key")
	if err := os.WriteFile(keyPath, []byte("not a key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	expired.Store(true)
	_, err = client.Agents()
	if err == nil || !strings.Contains(err.Error(), keyPath) {
		t.Errorf("after a malformed machine.key appeared: %v, want an error that names %s", err, keyPath)
	}
	if err != nil && strings.Contains(err.Error(), "there is no switchboard credential") {
		t.Errorf("a malformed key is reported as no credential: %v", err)
	}
	if got := registrations.Load(); got != 1 {
		t.Errorf("the client registered %d more times with the credential it held before the key file broke", got-1)
	}
}
