package acceptance

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type redirectingSwitchboard struct {
	address string
	host    string
	port    string
	reached *atomic.Int32
}

func startRedirectingSwitchboard(t *testing.T) redirectingSwitchboard {
	t.Helper()
	reached := &atomic.Int32{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"alpha","token":"alpha-token","name":"alpha","agents":[]}`))
	}))
	t.Cleanup(target.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)
	address := redirector.Listener.Addr().String()
	host, port, _ := net.SplitHostPort(address)
	return redirectingSwitchboard{address: address, host: host, port: port, reached: reached}
}

func (r redirectingSwitchboard) message() string {
	return "the switchboard at " + r.address + " answered with a redirect, which Landline does not follow"
}

func secretHomeFor(t *testing.T, host, port string) (string, []string) {
	t.Helper()
	home := t.TempDir()
	writeConfigFile(t, home, "config.toml", tomlFor(host, port, secret))
	return home, isolatedEnv(home)
}

func keyHomeFor(t *testing.T, host, port string) (string, []string) {
	t.Helper()
	return machineHomeFor(t, &refusingSwitchboard{host: host, port: port})
}

func TestEveryLandlineCommandTreatsARedirectAsAFailureSendsNothingToItsTargetAndExitsOne(t *testing.T) {
	t.Parallel()
	bin := build(t)
	cases := map[string]struct {
		home func(t *testing.T, host, port string) (string, []string)
		args []string
	}{
		"join --code": {func(t *testing.T, _, _ string) (string, []string) {
			home := t.TempDir()
			return home, isolatedEnv(home)
		}, nil},
		"register with a secret": {secretHomeFor, []string{"register", "--id", "alpha", "--harness", "claude-code", "--host", "mac"}},
		"register with a key":    {keyHomeFor, []string{"register", "--id", "alpha", "--harness", "claude-code", "--host", "mac"}},
		"log with a secret":      {secretHomeFor, []string{"log"}},
		"server machines":        {secretHomeFor, []string{"server", "machines"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			redirect := startRedirectingSwitchboard(t)
			home, env := c.home(t, redirect.host, redirect.port)
			args := c.args
			if args == nil {
				args = []string{"server", "join", redirect.address, "--code", "ABCD-EFGH"}
			}
			out, err := runIn(t, bin, env, home, args...)
			if exitCodeOf(err) != 1 || !strings.Contains(out, redirect.message()) {
				t.Errorf("%v: exit %d %q, want 1 and %q", args, exitCodeOf(err), out, redirect.message())
			}
			if got := redirect.reached.Load(); got != 0 {
				t.Errorf("%v: the place the redirect names got %d requests, want none", args, got)
			}
			if name == "join --code" && exists(filepath.Join(configDirOf(home), "machine.key")) {
				t.Errorf("a redirected join saved a machine key")
			}
		})
	}
}

func TestLandlineMCPReportsARedirectAsTheToolsErrorAndSendsNothingToItsTarget(t *testing.T) {
	t.Parallel()
	bin := build(t)
	for name, homeFor := range map[string]func(t *testing.T, host, port string) (string, []string){"a secret": secretHomeFor, "a key": keyHomeFor} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			redirect := startRedirectingSwitchboard(t)
			home, env := homeFor(t, redirect.host, redirect.port)
			cmd := exec.Command(mcpOf(bin))
			cmd.Dir, cmd.Env = home, env
			p := newMCP(t, cmd)
			p.handshake("claude-code", "2.1.296")
			reply := p.request("tools/call", `{"name":"landline_agents","arguments":{}}`)
			raw, _ := json.Marshal(reply)
			result, _ := reply["result"].(map[string]any)
			if result == nil || result["isError"] != true || !strings.Contains(string(raw), "redirect") {
				t.Errorf("landline_agents: %s, want an error result that says the switchboard answered with a redirect", raw)
			}
			if got := redirect.reached.Load(); got != 0 {
				t.Errorf("the place the redirect names got %d requests, want none", got)
			}
		})
	}
}

func TestTheScreenReportsARedirectAndSendsNothingToItsTarget(t *testing.T) {
	t.Parallel()
	bin := build(t)
	for name, homeFor := range map[string]func(t *testing.T, host, port string) (string, []string){"a secret": secretHomeFor, "a key": keyHomeFor} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			redirect := startRedirectingSwitchboard(t)
			_, env := homeFor(t, redirect.host, redirect.port)
			result := runBin(t, bin, env, "agents", "--json")
			if result.code != 1 || !strings.Contains(result.stderr, "redirect") {
				t.Errorf("agents --json: %+v, want exit 1 and the redirect named", result)
			}
			if got := redirect.reached.Load(); got != 0 {
				t.Errorf("the place the redirect names got %d requests, want none", got)
			}
		})
	}
}

func TestTheMenuBarSendsNothingToTheTargetOfARedirect(t *testing.T) {
	t.Parallel()
	redirect := startRedirectingSwitchboard(t)
	rig := newMenubarRig(t, defaultFlags(t), "http://"+redirect.address, landlineNextToBinary)
	result := rig.run(t, "menu", "--text")
	if result.code != 0 || strings.Contains(result.stdout, "Running at") {
		t.Errorf("%+v, want the menu of a switchboard it cannot use", result)
	}
	if got := redirect.reached.Load(); got != 0 {
		t.Errorf("the place the redirect names got %d requests, want none", got)
	}
}

const (
	forgedLine   = "joined the switchboard at alpha-host:8787 as alpha"
	oneLineError = "invalid or expired invite code\n" + forgedLine + "\tnow\r\nregistered bravo"
)

func startOneLineErrorSwitchboard(t *testing.T, status int) (string, string, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": oneLineError})
	}))
	t.Cleanup(server.Close)
	address := server.Listener.Addr().String()
	host, port, _ := net.SplitHostPort(address)
	return address, host, port
}

func TestASwitchboardMessageWithALineFeedOrTabCannotForgeASecondLineOrAColumn(t *testing.T) {
	t.Parallel()
	bin := build(t)
	cases := map[string]struct {
		status int
		home   func(t *testing.T, host, port string) (string, []string)
		args   []string
	}{
		"a refused join --code": {http.StatusUnauthorized, func(t *testing.T, _, _ string) (string, []string) {
			home := t.TempDir()
			return home, isolatedEnv(home)
		}, nil},
		"server machines":        {http.StatusForbidden, secretHomeFor, []string{"server", "machines"}},
		"server invite":          {http.StatusBadRequest, secretHomeFor, []string{"server", "invite", "--name", "bravo"}},
		"register with a secret": {http.StatusBadRequest, secretHomeFor, []string{"register", "--id", "alpha", "--harness", "claude-code", "--host", "mac"}},
		"register with a key":    {http.StatusBadRequest, keyHomeFor, []string{"register", "--id", "alpha", "--harness", "claude-code", "--host", "mac"}},
		"log with a secret":      {http.StatusBadRequest, secretHomeFor, []string{"log"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			address, host, port := startOneLineErrorSwitchboard(t, c.status)
			home, env := c.home(t, host, port)
			args := c.args
			if args == nil {
				args = []string{"server", "join", address, "--code", "ABCD-EFGH"}
			}
			out, err := runIn(t, bin, env, home, args...)
			if err == nil {
				t.Fatalf("%v succeeded: %q", args, out)
			}
			if !strings.Contains(out, "invalid or expired invite code") {
				t.Errorf("%v: the switchboard's message is not shown: %q", args, out)
			}
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "joined") || strings.HasPrefix(strings.TrimSpace(line), "registered") {
					t.Errorf("%v: the switchboard's message forged a line: %q", args, out)
					break
				}
			}
			if strings.ContainsAny(out, "\t\r") {
				t.Errorf("%v: a tab or carriage return from the switchboard reached the terminal: %q", args, out)
			}
		})
	}
}

func TestAFailedJoinKeepsAConfigEditMadeWhileTheRequestWasInFlight(t *testing.T) {
	t.Parallel()
	bin := build(t)
	home := t.TempDir()
	env := isolatedEnv(home)
	original := "# original\nsecret = \"" + secret + "\"\n"
	writeConfigFile(t, home, "config.toml", original)
	edited := original + "# edited by a person while the join was in flight\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeConfigFile(t, home, "config.toml", edited)
		if err := os.MkdirAll(filepath.Join(configDirOf(home), "machine.key", "blocker"), 0o700); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"alpha"}`))
	}))
	t.Cleanup(server.Close)
	out, err := runIn(t, bin, env, home, "server", "join", server.Listener.Addr().String(), "--code", "ABCD-EFGH")
	if err == nil {
		t.Fatalf("the join succeeded though machine.key is a folder: %s", out)
	}
	if got := readFileOrFail(t, configTomlOf(home)); got != edited {
		t.Errorf("the failed join put back config.toml over an edit it did not make:\n%s\nwant:\n%s", scrub(got), scrub(edited))
	}
}
