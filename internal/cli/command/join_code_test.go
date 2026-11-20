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
		t.Fatalf("files changed: key same %v, config same %v", string(keyAfter) == string(keyBefore), string(configAfter) == string(configBefore))
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

func TestAJoinWithASecretRefusesWhileAMachineKeyIsSavedAndChangesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := joinEnv(dir, &out, &errOut)
	env.Getenv = func(key string) string { return map[string]string{"LANDLINE_SECRET": longSecret}[key] }
	if err := SaveConfig(env, Config{Host: "old.example", Port: 9, Machine: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := writeMachineKey(env, mustKey(t)); err != nil {
		t.Fatal(err)
	}
	configBefore, _ := os.ReadFile(configPath(env))
	if code := Run([]string{"join", "other.example:8787"}, env); code != 1 || !strings.Contains(errOut.String(), "machine key") {
		t.Fatalf("%d %s", code, errOut.String())
	}
	if configAfter, _ := os.ReadFile(configPath(env)); string(configAfter) != string(configBefore) {
		t.Fatal("config.toml changed")
	}
}

func TestAJoinWithAnEmptyCodeIsRefusedAndDoesNotAskForASecret(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"join", "host.example:8787", "--code="}, {"join", "host.example:8787", "--code", ""}} {
		var out, errOut strings.Builder
		env := joinEnv(t.TempDir(), &out, &errOut)
		if code := Run(args, env); code != 1 || !strings.Contains(errOut.String(), "invite code is empty") || strings.Contains(errOut.String(), "Switchboard secret:") {
			t.Fatalf("%v: %d %s", args, code, errOut.String())
		}
	}
}

func TestInvitePrintsTheAddressItContactedWhenLandlineURLNamesAnotherLocalPort(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		raw, _ := json.Marshal(map[string]string{"name": "alpha", "code": "ABCD-EFGH"})
		w.Write(raw)
	}))
	t.Cleanup(server.Close)
	var out, errOut strings.Builder
	env := joinEnv(t.TempDir(), &out, &errOut)
	env.Getenv = func(key string) string {
		return map[string]string{"LANDLINE_URL": server.URL, "LANDLINE_SECRET": longSecret}[key]
	}
	if code := Run([]string{"server", "invite", "--name", "alpha"}, env); code != 0 {
		t.Fatalf("%d %s", code, errOut.String())
	}
	want := "landline server join " + strings.TrimPrefix(server.URL, "http://") + " --code ABCD-EFGH"
	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

func TestAdministrationCommandsDoNotStopOnAMalformedMachineKeyWhenTheSharedSecretIsSaved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := joinEnv(dir, &out, &errOut)
	if err := SaveConfig(env, Config{Host: "127.0.0.1", Port: 1, Secret: longSecret}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(machineKeyPath(env), []byte("not a key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	Run([]string{"server", "machines"}, env)
	if strings.Contains(errOut.String(), "machine.key") {
		t.Fatalf("the malformed key stopped the command: %s", errOut.String())
	}
}
