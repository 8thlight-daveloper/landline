package hubclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func homeWith(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		os.MkdirAll(filepath.Join(dir, ".config", "landline"), 0o700)
		os.WriteFile(filepath.Join(dir, ".config", "landline", name), []byte(content), 0o600)
	}
	return dir
}

func home(t *testing.T, toml string) string {
	t.Helper()
	if toml == "" {
		return homeWith(t, nil)
	}
	return homeWith(t, map[string]string{"config.toml": toml})
}

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

const savedToml = "# mine\nsecret = \"saved-secret-0000000\"\nhost = \"saved\"\nport = 1\n"

func TestDiscoverUsesTheEnvironmentBeforeTheSavedConfig(t *testing.T) {
	dir := home(t, savedToml)
	address, secret, _ := Discover(env(map[string]string{"LANDLINE_URL": "http://env:2/", "LANDLINE_SECRET": "env-secret"}), dir)
	if address != "http://env:2" || secret != "env-secret" {
		t.Fatalf("got %q %q", address, secret)
	}
}

func TestDiscoverFallsBackToTheSavedConfigPerField(t *testing.T) {
	dir := home(t, savedToml)
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

func TestDiscoverFillsAMissingHostOrPortFromTheDefaultsAndTheSavedPortBeatsTheEnvironment(t *testing.T) {
	secretOnly := home(t, "secret = \"saved-secret-0000000\"\n")
	if address, secret, err := Discover(env(map[string]string{"LANDLINE_PORT": "9001"}), secretOnly); err != nil || address != "http://127.0.0.1:9001" || secret != "saved-secret-0000000" {
		t.Fatalf("got %q %q %v", address, secret, err)
	}
	hostOnly := home(t, "host = \"tailnet-mac\"\n")
	if address, _, _ := Discover(env(map[string]string{"LANDLINE_PORT": "9001"}), hostOnly); address != "http://tailnet-mac:9001" {
		t.Fatalf("got %q", address)
	}
	if address, _, _ := Discover(env(map[string]string{"LANDLINE_PORT": "9001"}), home(t, savedToml)); address != "http://saved:1" {
		t.Fatalf("a saved port must win: %q", address)
	}
	if address, _, _ := Discover(env(nil), home(t, "host = \"::1\"\nport = 8787\n")); address != "http://[::1]:8787" {
		t.Fatalf("an IPv6 host takes brackets: %q", address)
	}
}

func TestDiscoverReadsAnOldConfigJSONOnlyWhenThereIsNoConfigTomlAndPrefersToml(t *testing.T) {
	old := `{"url":"http://old:3/","secret":"old-secret-0000000"}`
	if address, secret, err := Discover(env(nil), homeWith(t, map[string]string{"config.json": old})); err != nil || address != "http://old:3" || secret != "old-secret-0000000" {
		t.Fatalf("got %q %q %v", address, secret, err)
	}
	if address, _, _ := Discover(env(nil), homeWith(t, map[string]string{"config.json": old, "config.toml": savedToml})); address != "http://saved:1" {
		t.Fatalf("toml must win: %q", address)
	}
}

func TestDiscoverToleratesABrokenOldConfigJSONAndRefusesAnUnusableOneWithoutEchoingIt(t *testing.T) {
	if address, _, err := Discover(env(nil), homeWith(t, map[string]string{"config.json": `{not json`})); err != nil || address != "http://127.0.0.1:8787" {
		t.Fatalf("got %q %v", address, err)
	}
	for _, bad := range []string{"https://host:1", "http://u:hunter2@host:1", "http://host", "http://host:1/path", "ftp://host:1"} {
		_, _, err := Discover(env(nil), homeWith(t, map[string]string{"config.json": `{"url":"` + bad + `","secret":"saved-secret-0000000"}`}))
		if err == nil || strings.Contains(err.Error(), "hunter2") || !strings.Contains(err.Error(), "config.json") {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestDiscoverRefusesALineThatIsNotBlankACommentOrAKeyWithATextOrNumberValueAtItsLineWithoutEchoingIt(t *testing.T) {
	for text, line := range map[string]string{
		"host = h\n": "line 1", "host = 5\n": "line 1", "secret = 12345\n": "line 1", "port = \"8787\"\n": "line 1",
		"port = 0\n": "line 1", "port = 87.87\n": "line 1", "port = 8787 # hunter2\n": "line 1", "host = 'hunter2'\n": "line 1",
		"[a.hunter2]\n": "line 1", "list = [1, hunter2]\n": "line 1", "# ok\n\nthis is hunter2\n": "line 3",
	} {
		_, _, err := Discover(env(nil), home(t, text))
		if err == nil || !strings.Contains(err.Error(), "config.toml line "+line[len("line "):]) || strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "12345") {
			t.Errorf("%q: %v", text, err)
		}
	}
}

func TestDiscoverIgnoresCommentsBlankLinesAndUnknownKeys(t *testing.T) {
	dir := home(t, "# one\n\nextra = \"x\"\nmore = 7\nsecret = \"saved-secret-0000000\"\n\nhost = \"h\"\nport = 2\n")
	if address, secret, err := Discover(env(nil), dir); err != nil || address != "http://h:2" || secret != "saved-secret-0000000" {
		t.Fatalf("got %q %q %v", address, secret, err)
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

func TestDiscoverAcceptsHTTPSAndATrailingSlashInTheEnvironment(t *testing.T) {
	dir := home(t, "")
	for in, want := range map[string]string{"https://hub.example:8443/": "https://hub.example:8443", "http://[::1]:8787": "http://[::1]:8787"} {
		if address, _, err := Discover(env(map[string]string{"LANDLINE_URL": in}), dir); err != nil || address != want {
			t.Errorf("%q: got %q %v", in, address, err)
		}
	}
}
