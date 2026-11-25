package acceptance

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
			shown := strings.TrimSpace(result.stderr)
			if result.code != 1 || !strings.HasSuffix(shown, redirect.message()) || strings.Contains(shown, "\n") {
				t.Errorf("agents --json: %+v, want exit 1 and one line that ends with %q", result, redirect.message())
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
	lines := strings.Split(result.stdout, "\n")
	if result.code != 0 || len(lines) < 2 || lines[1] != redirect.message()+disabledMark {
		t.Errorf("%+v, want the state line %q where the menu shows a failure to reach the switchboard", result, redirect.message()+disabledMark)
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

func TestAnAdministrationRefusalPrintsTheSavedHostCleaned(t *testing.T) {
	t.Parallel()
	bin := build(t)
	for _, args := range [][]string{{"server", "machines"}, {"server", "rotate-secret"}, {"server", "invite", "--name", "bravo"}, {"server", "revoke", "bravo"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			writeConfigFile(t, home, "config.toml", "secret = \""+secret+"\"\nhost = \"alpha-host\tbravo\x1b[2J\x07end\"\nport = 8787\n")
			out, err := runIn(t, bin, isolatedEnv(home), home, args...)
			if exitCodeOf(err) != 1 || !strings.Contains(out, "alpha-host") {
				t.Errorf("%v: exit %d %q, want 1 and a refusal that names the saved host", args, exitCodeOf(err), out)
			}
			if strings.ContainsAny(out, "\t\x1b\x07") {
				t.Errorf("%v: a control character in the saved host reached the terminal: %q", args, out)
			}
		})
	}
}

func startAcceptingRedeem(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	reached := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"alpha"}`))
	}))
	t.Cleanup(server.Close)
	return server.Listener.Addr().String(), reached
}

func TestJoinWithACodeRefusesAndChangesNoFileWhileThisHomeHoldsASwitchboardLock(t *testing.T) {
	t.Parallel()
	cases := map[string]func(c *configuredHome){
		"held by a running switchboard": func(c *configuredHome) {
			c.startSwitchboard()
			if !exists(c.dataFile("switchboard.lock")) {
				c.t.Fatalf("the running switchboard made no %s", c.dataFile("switchboard.lock"))
			}
		},
		"left by a switchboard that stopped": func(c *configuredHome) {
			c.writeDataFile("switchboard.lock", "")
		},
	}
	for name, prepare := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newConfiguredHome(t, "")
			prepare(c)
			address, reached := startAcceptingRedeem(t)
			original := c.configText()
			result := c.landline("server", "join", address, "--code", "ABCD-EFGH")
			said := result.stdout + result.stderr
			if result.code != 1 || !strings.Contains(said, "switchboard.lock") || !strings.Contains(said, "secret") {
				t.Errorf("join --code: %d %q, want 1 and a message that names switchboard.lock and says this home uses the shared secret", result.code, said)
			}
			if got := c.configText(); got != original {
				t.Errorf("join --code changed config.toml:\n%s", scrub(got))
			}
			if exists(filepath.Join(configDirOf(c.home), "machine.key")) {
				t.Errorf("join --code saved a machine key")
			}
			if got := reached.Load(); got != 0 {
				t.Errorf("the switchboard named in the join got %d requests, want none", got)
			}
		})
	}
}

func TestJoinWithACodeOverLoopbackFromAHomeWithNoSwitchboardLockJoins(t *testing.T) {
	t.Parallel()
	bin := build(t)
	address, reached := startAcceptingRedeem(t)
	home := t.TempDir()
	out, err := runIn(t, bin, isolatedEnv(home), home, "server", "join", address, "--code", "ABCD-EFGH")
	if err != nil || !strings.Contains(out, "joined the switchboard at "+address+" as alpha") {
		t.Errorf("join --code %s: %v %q, want a join", address, err, out)
	}
	if !exists(filepath.Join(configDirOf(home), "machine.key")) || reached.Load() != 1 {
		t.Errorf("join --code %s: key saved %v, %d requests, want a key and one request", address, exists(filepath.Join(configDirOf(home), "machine.key")), reached.Load())
	}
}

func TestTheMenuBarShowsTheRedirectTextWhenOnlyTheBuildRouteRedirects(t *testing.T) {
	t.Parallel()
	var reachedTarget atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reachedTarget.Add(1) }))
	t.Cleanup(target.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/observers":
			w.Write([]byte(`{"token":"observer-token"}`))
		case "/v1/agents":
			w.Write([]byte(`[]`))
		default:
			http.Redirect(w, r, target.URL+r.URL.RequestURI(), http.StatusTemporaryRedirect)
		}
	}))
	t.Cleanup(server.Close)
	address := server.Listener.Addr().String()
	want := "the switchboard at " + address + " answered with a redirect, which Landline does not follow" + disabledMark
	rig := newMenubarRig(t, defaultFlags(t), server.URL, landlineNextToBinary)
	result := rig.run(t, "menu", "--text")
	lines := strings.Split(result.stdout, "\n")
	if result.code != 0 || len(lines) < 2 || lines[1] != want {
		t.Errorf("%+v, want the state line %q", result, want)
	}
	if got := reachedTarget.Load(); got != 0 {
		t.Errorf("the place the redirect names got %d requests, want none", got)
	}
}

func installedAgentPath(c *configuredHome) string {
	if runtime.GOOS == "darwin" {
		return c.path("Library/LaunchAgents/dev.landline.switchboard.plist")
	}
	return c.path(".config/systemd/user/landline-switchboard.service")
}

func checkJoinRefusedAndChangedNothing(t *testing.T, c *configuredHome, result binResult, original, named string) {
	t.Helper()
	said := result.stdout + result.stderr
	if result.code != 1 || !strings.Contains(said, named) {
		t.Errorf("join --code: %d %q, want 1 and a message that names %s", result.code, said, named)
	}
	if got := c.configText(); got != original {
		t.Errorf("join --code changed config.toml:\n%s", scrub(got))
	}
	if exists(filepath.Join(configDirOf(c.home), "machine.key")) {
		t.Errorf("join --code saved a machine key")
	}
}

func TestJoinWithACodeRefusesInAHomeWhereASwitchboardRanBeforeTheLockFileExisted(t *testing.T) {
	t.Parallel()
	cases := map[string]func(c *configuredHome) string{
		"switchboard.pid": func(c *configuredHome) string { return c.writeDataFile("switchboard.pid", "4242") },
		"log.jsonl":       func(c *configuredHome) string { return c.writeDataFile("log.jsonl", "") },
		"the installed agent": func(c *configuredHome) string {
			path := installedAgentPath(c)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				c.t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("installed by an earlier landline server install\n"), 0o644); err != nil {
				c.t.Fatal(err)
			}
			return path
		},
	}
	for name, leave := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newConfiguredHome(t, "")
			found := leave(c)
			address, reached := startAcceptingRedeem(t)
			original := c.configText()
			result := c.landline("server", "join", address, "--code", "ABCD-EFGH")
			checkJoinRefusedAndChangedNothing(t, c, result, original, filepath.Base(found))
			if exists(c.dataFile("switchboard.lock")) {
				t.Errorf("join made switchboard.lock")
			}
			if got := reached.Load(); got != 0 {
				t.Errorf("the switchboard named in the join got %d requests, want none", got)
			}
		})
	}
}

func TestJoinWithACodeRefusesAndChangesNoFileWhenASwitchboardStartsHereWhileTheRequestIsInFlight(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	original := c.configText()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.writeDataFile("switchboard.lock", "4242")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"alpha"}`))
	}))
	t.Cleanup(server.Close)
	result := c.landline("server", "join", server.Listener.Addr().String(), "--code", "ABCD-EFGH")
	checkJoinRefusedAndChangedNothing(t, c, result, original, "switchboard.lock")
}
