package command

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func redeemServer(t *testing.T, name string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		out, _ := json.Marshal(map[string]string{"name": name})
		w.Write(out)
	}))
	t.Cleanup(server.Close)
	return strings.TrimPrefix(server.URL, "http://")
}

func joinEnv(dir string, out, errOut *strings.Builder) Env {
	return Env{ConfigDir: dir, Stdout: out, Stderr: errOut, Stdin: strings.NewReader(""), Getenv: func(string) string { return "" }}
}

func TestAJoinThatCannotSaveRestoresTheEarlierKeyAndConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := joinEnv(dir, &out, &errOut)
	if err := SaveConfig(env, Config{Host: "old.example", Port: 9, Machine: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := writeMachineKey(env, mustKey(t)); err != nil {
		t.Fatal(err)
	}
	keyBefore, _ := os.ReadFile(machineKeyPath(env))
	configBefore, _ := os.ReadFile(configPath(env))
	if err := os.MkdirAll(filepath.Join(dir, "config.toml.tmp", "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"join", redeemServer(t, "beta"), "--code", "ABCD-EFGH"}, env); code != 1 {
		t.Fatalf("%d %s", code, out.String())
	}
	keyAfter, _ := os.ReadFile(machineKeyPath(env))
	configAfter, _ := os.ReadFile(configPath(env))
	if string(keyAfter) != string(keyBefore) || string(configAfter) != string(configBefore) {
		t.Fatalf("files changed: key %q config %q", keyAfter, configAfter)
	}
	if !strings.Contains(errOut.String(), "landline server revoke beta") {
		t.Fatalf("%s", errOut.String())
	}
}

func TestAJoinRefusesAMachineNameThatTheSwitchboardRuleDoesNotAllow(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := joinEnv(dir, &out, &errOut)
	hostile := "a\"\nhost = \"evil.example\"\nsecret = \"x"
	if code := Run([]string{"join", redeemServer(t, hostile), "--code", "ABCD-EFGH"}, env); code != 1 {
		t.Fatalf("%d %s", code, out.String())
	}
	for _, name := range []string{"machine.key", "config.toml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Fatalf("%s was written", name)
		}
	}
}

func TestAdminCommandsRefuseWhenLandlineURLNamesAnotherMachine(t *testing.T) {
	t.Parallel()
	for command, extra := range map[string][]string{"invite": {"--name", "x"}, "machines": nil, "revoke": {"x"}} {
		var out, errOut strings.Builder
		env := joinEnv(t.TempDir(), &out, &errOut)
		env.Getenv = func(key string) string {
			return map[string]string{"LANDLINE_URL": "http://203.0.113.9:8787", "LANDLINE_SECRET": longSecret}[key]
		}
		if code := Run(append([]string{"server", command}, extra...), env); code != 1 || !strings.Contains(errOut.String(), "not an address of this machine") {
			t.Fatalf("%s: %d %s", command, code, errOut.String())
		}
	}
}

func mustKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, private, err := newMachineKey()
	if err != nil {
		t.Fatal(err)
	}
	return private
}
