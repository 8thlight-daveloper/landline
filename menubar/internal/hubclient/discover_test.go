package hubclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func home(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	if config != "" {
		os.MkdirAll(filepath.Join(dir, ".config", "landline"), 0o700)
		os.WriteFile(filepath.Join(dir, ".config", "landline", "config.json"), []byte(config), 0o600)
	}
	return dir
}

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestDiscoverUsesTheEnvironmentBeforeTheSavedConfig(t *testing.T) {
	dir := home(t, `{"url":"http://saved:1","secret":"saved-secret-0000000"}`)
	address, secret, _ := Discover(env(map[string]string{"LANDLINE_URL": "http://env:2/", "LANDLINE_SECRET": "env-secret"}), dir)
	if address != "http://env:2" || secret != "env-secret" {
		t.Fatalf("got %q %q", address, secret)
	}
}

func TestDiscoverFallsBackToTheSavedConfigPerField(t *testing.T) {
	dir := home(t, `{"url":"http://saved:1","secret":"saved-secret-0000000"}`)
	address, secret, _ := Discover(env(map[string]string{"LANDLINE_URL": "http://env:2"}), dir)
	if address != "http://env:2" || secret != "saved-secret-0000000" {
		t.Fatalf("got %q %q", address, secret)
	}
	address, secret, _ = Discover(env(nil), dir)
	if address != "http://saved:1" || secret != "saved-secret-0000000" {
		t.Fatalf("got %q %q", address, secret)
	}
}

func TestDiscoverDefaultsToLoopbackOnTheDefaultOrConfiguredPort(t *testing.T) {
	dir := home(t, "")
	if address, secret, _ := Discover(env(nil), dir); address != "http://127.0.0.1:8787" || secret != "" {
		t.Fatalf("got %q %q", address, secret)
	}
	if address, _, _ := Discover(env(map[string]string{"LANDLINE_PORT": "9001"}), dir); address != "http://127.0.0.1:9001" {
		t.Fatalf("got %q", address)
	}
	if address, _, _ := Discover(env(map[string]string{"LANDLINE_PORT": "nope"}), dir); address != "http://127.0.0.1:8787" {
		t.Fatalf("got %q", address)
	}
}

func TestDiscoverToleratesABrokenConfigFile(t *testing.T) {
	dir := home(t, `{not json`)
	if address, _, _ := Discover(env(nil), dir); address != "http://127.0.0.1:8787" {
		t.Fatalf("got %q", address)
	}
}

func TestDiscoverRejectsAddressesTheMainModuleRejectsWithoutEchoingThem(t *testing.T) {
	dir := home(t, "")
	for _, bad := range []string{
		"http://user:hunter2@host:1", "http://:hunter2@host:1", "http://host:1?x=hunter2", "http://host:1?",
		"http://host:1#hunter2", "ftp://host:1", "host:1", "http://", "http://host:1/path", "http://host:1/a/..",
		"http://host:1\x1b[2J", "http://host:1\u0085", "http://host:1‮", "http://host\n:1", "http://ho st:1",
	} {
		address, secret, err := Discover(env(map[string]string{"LANDLINE_URL": bad, "LANDLINE_SECRET": "s"}), dir)
		if err == nil || address != "" || secret != "" {
			t.Errorf("%q accepted as %q", bad, address)
			continue
		}
		if strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "user") && strings.Contains(err.Error(), "@") {
			t.Errorf("%q echoed: %v", bad, err)
		}
	}
}

func TestDiscoverRejectsABadSavedAddress(t *testing.T) {
	dir := home(t, `{"url":"http://u:hunter2@host:1","secret":"saved-secret-0000000"}`)
	if _, _, err := Discover(env(nil), dir); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("got %v", err)
	}
}

func TestDiscoverAcceptsHTTPSAndATrailingSlash(t *testing.T) {
	dir := home(t, "")
	for in, want := range map[string]string{"https://hub.example:8443/": "https://hub.example:8443", "http://[::1]:8787": "http://[::1]:8787"} {
		if address, _, err := Discover(env(map[string]string{"LANDLINE_URL": in}), dir); err != nil || address != want {
			t.Errorf("%q: got %q %v", in, address, err)
		}
	}
}
