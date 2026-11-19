package acceptance

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func pathWith(dirs ...string) string {
	return "PATH=" + strings.Join(dirs, string(os.PathListSeparator))
}

func lonelyLandline(t *testing.T) (bin string, env []string, home string) {
	t.Helper()
	home = t.TempDir()
	bin = copyBinary(t, build(t), t.TempDir())
	env = withEnv(isolatedEnv(home), pathWith(t.TempDir()))
	return bin, env, home
}

func TestLandlineAgentsTextIsOneTabSeparatedLinePerRowInTheScreensOrderAndMatchesTheJSONRows(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("tango")
	w.registerWithProject("delta", "yankee")
	w.registerWithProject("foxtrot", "bravo-l")
	projectedLate := w.selfRegisteredWithProject("a-x", "zed")
	projectedEarly := w.selfRegisteredWithProject("b-x", "amy")
	plain := w.selfRegisteredWithProject("c-x", "")
	text := runBin(t, w.bin, w.env, "agents")
	if text.code != 0 || text.stderr != "" {
		t.Fatalf("%+v", text)
	}
	var snapshot struct{ Agents []map[string]any }
	asJSON := runBin(t, w.bin, w.env, "agents", "--json")
	if err := json.Unmarshal([]byte(asJSON.stdout), &snapshot); err != nil || asJSON.code != 0 {
		t.Fatalf("%v %+v", err, asJSON)
	}
	rows := strings.Split(strings.TrimSuffix(text.stdout, "\n"), "\n")
	if len(rows) != len(snapshot.Agents) || len(rows) != 6 {
		t.Fatalf("%d lines for %d rows:\n%s", len(rows), len(snapshot.Agents), text.stdout)
	}
	var ids []string
	for i, row := range rows {
		fields := strings.Split(row, "\t")
		if len(fields) < 4 || len(fields) > 5 {
			t.Fatalf("the line %q has %d tab separated fields", row, len(fields))
		}
		entry := snapshot.Agents[i]
		if fields[0] != entry["id"] || fields[1] != entry["harness"] || fields[3] != entry["state"] {
			t.Errorf("line %d %q does not match the row %v", i, row, entry)
		}
		if host, _ := entry["host"].(string); host != "" && fields[2] != host {
			t.Errorf("line %d %q: the host is not %q", i, row, host)
		}
		ids = append(ids, fields[0])
	}
	want := expectedListOrder(listedAgentsOf(snapshot.Agents))
	for i, agent := range want {
		if ids[i] != agent.id {
			t.Fatalf("the order is %v, want %v", ids, want)
		}
	}
	connected := map[string]bool{projectedLate: true, projectedEarly: true, plain: true}
	for i, agentID := range ids {
		if connected[agentID] != (i < 3) {
			t.Fatalf("connected agents are not first: %v", ids)
		}
	}
	for i, agentID := range ids[:2] {
		if agentID != []string{projectedEarly, projectedLate}[i] {
			t.Fatalf("connected agents with a project are not ordered by project: %v", ids)
		}
	}
	if !strings.Contains(rows[0], "project: amy") {
		t.Fatalf("the attribute text lacks the project: %q", rows[0])
	}
}

func TestLandlineAgentsReadsTheListWithAnObserverTokenAndRegistersNoAgentAndTakesNoAs(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("alpha")
	before, _ := os.ReadDir(filepath.Join(w.config, ".config", "landline"))
	if out := w.must("", "agents"); !strings.Contains(out, "alpha\tclaude-code\tmac\tdisconnected") {
		t.Fatalf("agents: %q", out)
	}
	after, _ := os.ReadDir(filepath.Join(w.config, ".config", "landline"))
	if len(after) != len(before) {
		t.Fatalf("landline agents wrote a session file: %v then %v", before, after)
	}
	for _, args := range [][]string{{"agents", "--as", "alpha"}, {"agents", "--json", "--as", "alpha"}} {
		result := runBin(t, w.bin, w.env, args...)
		if result.code == 0 || strings.Contains(result.stdout, "alpha\t") || strings.Contains(result.stdout, `"id"`) {
			t.Errorf("%v: %+v", args, result)
		}
	}
}

func TestLandlineAgentsNeedsTheSecretAndAnAgentSessionFileIsNotEnough(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("alpha")
	var kept []string
	for _, pair := range w.env {
		if !strings.HasPrefix(pair, "LANDLINE_SECRET=") {
			kept = append(kept, pair)
		}
	}
	for _, args := range [][]string{{"agents"}, {"agents", "--json"}} {
		result := runBin(t, w.bin, kept, args...)
		if result.code != 1 || result.stdout != "" || strings.TrimSpace(result.stderr) == "" {
			t.Errorf("%v: %+v", args, result)
		}
	}
}

func TestSetupPrintsTheAbsolutePathOfLandlineMCPWithTheArgumentsLandlineMCPHadAndPrefersTheOneNextToItself(t *testing.T) {
	t.Parallel()
	w := start(t)
	decoys := t.TempDir()
	decoy := filepath.Join(decoys, mcpName)
	if err := os.WriteFile(decoy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := withEnv(w.env, pathWith(decoys, os.Getenv("PATH")))
	for harness, want := range map[string]string{
		"claude": "claude mcp add --scope user landline -- %s --channel",
		"codex":  "codex mcp add landline -- %s",
	} {
		result := runBin(t, w.bin, env, "setup", harness, "--print")
		if result.code != 0 {
			t.Fatalf("%s: %+v", harness, result)
		}
		line := strings.TrimSpace(result.stdout)
		if line != fmt.Sprintf(want, mcpOf(w.bin)) {
			resolved, _ := filepath.EvalSymlinks(mcpOf(w.bin))
			if line != fmt.Sprintf(want, resolved) {
				t.Errorf("%s: %q", harness, line)
			}
		}
	}
}

func TestSetupFindsLandlineMCPOnThePathWhenItIsNotNextToLandline(t *testing.T) {
	t.Parallel()
	bin, env, _ := lonelyLandline(t)
	elsewhere := copyBinary(t, mcpOf(build(t)), t.TempDir())
	result := runBin(t, bin, withEnv(env, pathWith(filepath.Dir(elsewhere))), "setup", "claude", "--print")
	if result.code != 0 || !containsPath(result.stdout, elsewhere) || !strings.Contains(result.stdout, "--channel") {
		t.Fatalf("%+v", result)
	}
}

func TestSetupExitsOneNamingLandlineMCPAndJustInstallWhenItIsMissing(t *testing.T) {
	t.Parallel()
	bin, env, home := lonelyLandline(t)
	for _, args := range [][]string{{"setup", "claude"}, {"setup", "codex"}} {
		result := runBin(t, bin, env, args...)
		if result.code != 1 || !strings.Contains(result.stderr, mcpName) || !strings.Contains(result.stderr, justInstall) {
			t.Errorf("%v: %+v", args, result)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "landline", "SKILL.md")); err == nil {
		t.Error("the skill was installed with no landline-mcp to name")
	}
}

func TestSetupWarnsAboutANixStorePathOnlyWhenLandlineMCPIsThere(t *testing.T) {
	t.Parallel()
	bin, env, _ := lonelyLandline(t)
	store := filepath.Join(t.TempDir(), "nix", "store", "abc123-landline", "bin")
	inStore := copyBinary(t, mcpOf(build(t)), store)
	warned := runBin(t, bin, withEnv(env, pathWith(store)), "setup", "claude", "--print")
	if warned.code != 0 || !containsPath(warned.stdout, inStore) || !strings.Contains(warned.stderr, "Nix store") {
		t.Fatalf("landline-mcp in the store: %+v", warned)
	}
	outside := copyBinary(t, mcpOf(build(t)), t.TempDir())
	quiet := runBin(t, bin, withEnv(env, pathWith(filepath.Dir(outside))), "setup", "claude", "--print")
	if quiet.code != 0 || strings.Contains(quiet.stderr, "Nix store") {
		t.Fatalf("landline-mcp outside the store: %+v", quiet)
	}
}

func TestServerUpRestartAndInstallExitOneNamingTheSwitchboardAndJustInstallWhenItIsMissing(t *testing.T) {
	t.Parallel()
	bin, env, home := lonelyLandline(t)
	addr := freeAddr(t)
	for _, args := range [][]string{{"server", "up", "--addr", addr}, {"server", "restart", "--addr", addr}, {"server", "install"}} {
		result := runBin(t, bin, env, args...)
		if result.code != 1 || !strings.Contains(result.stderr, switchboardName) || !strings.Contains(result.stderr, justInstall) {
			t.Errorf("%v: %+v", args, result)
		}
	}
	if c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond); err == nil {
		c.Close()
		t.Error("a listener is open")
	}
	for _, unit := range []string{
		filepath.Join(home, "Library", "LaunchAgents", "dev.landline.switchboard.plist"),
		filepath.Join(home, ".config", "systemd", "user", "landline-switchboard.service"),
	} {
		if _, err := os.Stat(unit); err == nil {
			t.Errorf("%s was written for a switchboard that is not there", unit)
		}
	}
}

func TestServerUpFindsTheSwitchboardOnThePathWhenItIsNotNextToLandline(t *testing.T) {
	t.Parallel()
	bin, env, home := lonelyLandline(t)
	elsewhere := copyBinary(t, switchboardOf(build(t)), t.TempDir())
	env = withEnv(env, pathWith(filepath.Dir(elsewhere), "/usr/bin", "/bin"))
	t.Cleanup(func() { runIn(t, bin, env, home, "server", "down") })
	addr := freeAddr(t)
	out, err := runIn(t, bin, env, home, "server", "up", "--addr", addr)
	if err != nil || !strings.Contains(out, "is running") {
		t.Fatalf("up: %v %s", err, out)
	}
	pid, _ := os.ReadFile(filepath.Join(home, ".local", "share", "landline", "switchboard.pid"))
	if ps, _ := exec.Command("ps", "-o", "command=", "-p", strings.TrimSpace(string(pid))).Output(); !strings.Contains(string(ps), elsewhere) {
		t.Fatalf("the running process is %q, want the one at %s", ps, elsewhere)
	}
}

func TestServerInstallRefusesASwitchboardInTheNixStoreAndNotALandlineThatIsThere(t *testing.T) {
	t.Parallel()
	root := build(t)
	store := filepath.Join(t.TempDir(), "nix", "store", "abc123-landline", "bin")

	landlineInStore := copyBinary(t, root, store)
	outside := copyBinary(t, switchboardOf(root), t.TempDir())
	home := t.TempDir()
	env := withEnv(isolatedEnv(home), pathWith(filepath.Dir(outside)))
	if out, err := runIn(t, landlineInStore, env, home, "server", "install"); err != nil {
		t.Fatalf("landline in the store, switchboard outside: %v %s", err, out)
	}

	switchboardInStore := copyBinary(t, switchboardOf(root), filepath.Join(t.TempDir(), "nix", "store", "def456-landline", "bin"))
	bin := copyBinary(t, root, t.TempDir())
	home = t.TempDir()
	env = withEnv(isolatedEnv(home), pathWith(filepath.Dir(switchboardInStore)))
	result := runBin(t, bin, env, "server", "install")
	if result.code != 1 || !strings.Contains(result.stderr, "Nix store") {
		t.Fatalf("switchboard in the store: %+v", result)
	}
}

func TestServerDownAndStatusIgnoreAPidInTheSwitchboardPidFileThatIsNotTheSwitchboard(t *testing.T) {
	t.Parallel()
	h := newBackgroundHub(t)
	data := filepath.Join(h.home, ".local", "share", "landline")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	sleeper := exec.Command("sleep", "60")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sleeper.Process.Kill(); sleeper.Wait() })
	if err := os.WriteFile(filepath.Join(data, "switchboard.pid"), []byte(fmt.Sprint(sleeper.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, _ := h.run("server", "status"); !strings.Contains(status, "no switchboard started by landline server up is running") {
		t.Fatalf("status: %s", status)
	}
	if out, err := h.run("server", "down"); err != nil {
		t.Fatalf("down: %v %s", err, out)
	}
	if err := sleeper.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("server down stopped a process that is not the switchboard: %v", err)
	}
}

func TestLandlineClaudePrintsTheChannelLaunchWithTheServerNamedLandline(t *testing.T) {
	t.Parallel()
	w := start(t)
	result := runBin(t, w.bin, w.env, "claude", "--project", "lead", "--print")
	if result.code != 0 || strings.TrimSpace(result.stdout) != "LANDLINE_PROJECT=lead claude --dangerously-load-development-channels server:landline" {
		t.Fatalf("%+v", result)
	}
}
