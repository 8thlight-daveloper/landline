package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
)

const (
	menubarName    = "landline-menubar"
	menubarTimeout = 30 * time.Second
	menuSeparator  = "---"
	disabledMark   = " (disabled)"
	liveMarker     = "connected"
)

func releaseVersion(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../VERSION")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func defaultFlags(t *testing.T) string { return "-X main.version=" + releaseVersion(t) }

func menubarBinary(t *testing.T, flags string) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("landline-menubar is macOS only")
	}
	builtMu.Lock()
	defer builtMu.Unlock()
	key := "menubar:" + flags
	if dir, ok := built[key]; ok {
		return filepath.Join(dir, menubarName)
	}
	dir, err := os.MkdirTemp("", "landline-menubar-build-")
	if err != nil {
		t.Fatal(err)
	}
	finished := false
	defer func() {
		if !finished {
			os.RemoveAll(dir)
		}
	}()
	buildOne(t, dir, menubarName, "../menubar", ".", flags)
	finished = true
	built[key] = dir
	return filepath.Join(dir, menubarName)
}

type landlineAt int

const (
	landlineNextToBinary landlineAt = iota
	landlineOnPath
	landlineMissing
)

type menubarRig struct {
	bin  string
	env  []string
	home string
	dirs []string
}

func newMenubarRig(t *testing.T, flags, url string, where landlineAt) menubarRig {
	t.Helper()
	source := menubarBinary(t, flags)
	home := t.TempDir()
	dir := t.TempDir()
	bin := copyBinary(t, source, dir)
	pathDir := t.TempDir()
	switch where {
	case landlineNextToBinary:
		copyBinary(t, filepath.Join(filepath.Dir(build(t)), landlineName), dir)
	case landlineOnPath:
		copyBinary(t, filepath.Join(filepath.Dir(build(t)), landlineName), pathDir)
	}
	shellDir := t.TempDir()
	shell := filepath.Join(shellDir, "shell")
	script := "#!/bin/sh\nprintf %s '" + pathDir + "'\n"
	if err := os.WriteFile(shell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := withEnv(isolatedEnv(home), pathWith(pathDir), "SHELL="+shell)
	if url != "" {
		env = withEnv(env, "LANDLINE_URL="+url, "LANDLINE_SECRET="+secret)
	}
	return menubarRig{bin: bin, env: env, home: home, dirs: []string{home, dir, pathDir, shellDir}}
}

func (r menubarRig) run(t *testing.T, args ...string) binResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), menubarTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = r.env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	result := binResult{stdout: out.String(), stderr: errb.String()}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		result.code = exit.ExitCode()
	case err != nil:
		t.Fatalf("%s %v: %v (stdout %q, stderr %q)", menubarName, args, err, result.stdout, result.stderr)
	}
	return result
}

func (r menubarRig) menu(t *testing.T) string {
	t.Helper()
	result := r.run(t, "menu", "--text")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("menu --text: %+v", result)
	}
	if strings.Contains(result.stdout, secret) || strings.Contains(result.stdout, "observer-token-") {
		t.Fatalf("the menu shows the secret or a token: %q", result.stdout)
	}
	return result.stdout
}

func cleaned(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf) {
			return ' '
		}
		return r
	}, text)
}

type fakeRequest struct{ method, path, authorization, body string }

type fakeSwitchboard struct {
	t           *testing.T
	url         string
	mu          sync.Mutex
	requests    []fakeRequest
	agents      any
	buildStatus int
	buildBody   any
	staleFirst  bool
	rejectAll   bool
	agentsCode  int
	agentsRaw   string
	buildRaw    string
	issued      int
}

func fakeAgent(id string, connected bool, extra map[string]any) map[string]any {
	entry := map[string]any{
		"id": id, "harness": "claude-code", "host": "mac", "session": "s-" + id,
		"connected": connected, "last_seen": "2026-11-16T10:00:00Z",
	}
	for key, value := range extra {
		entry[key] = value
	}
	return entry
}

func fakeBuild(version, commit, date string) map[string]string {
	return map[string]string{"version": version, "commit": commit, "date": date}
}

func newFakeSwitchboard(t *testing.T, host string, agents any, buildStatus int, buildBody any) *fakeSwitchboard {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSwitchboard{t: t, agents: agents, buildStatus: buildStatus, buildBody: buildBody}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	f.url = "http://" + net.JoinHostPort(host, port)
	server := &http.Server{Handler: http.HandlerFunc(f.serve)}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	return f
}

func (f *fakeSwitchboard) tokenIsLive(authorization string) bool {
	var n int
	if _, err := fmt.Sscanf(authorization, "Bearer observer-token-%d", &n); err != nil || n < 1 || n > f.issued {
		return false
	}
	return !(f.staleFirst && n == 1)
}

func (f *fakeSwitchboard) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body bytes.Buffer
	body.ReadFrom(r.Body)
	authorization := r.Header.Get("Authorization")
	f.requests = append(f.requests, fakeRequest{r.Method, r.URL.Path, authorization, body.String()})
	reply := func(status int, value any) {
		raw, _ := json.Marshal(value)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(raw)
	}
	switch {
	case r.Method == "POST" && r.URL.Path == "/v1/observers":
		if authorization != "Bearer "+secret {
			reply(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		f.issued++
		reply(http.StatusOK, map[string]string{"token": fmt.Sprintf("observer-token-%d", f.issued)})
	case r.Method == "GET" && r.URL.Path == "/v1/agents":
		if f.rejectAll || !f.tokenIsLive(authorization) {
			reply(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if f.agentsRaw != "" {
			w.WriteHeader(max(f.agentsCode, http.StatusOK))
			w.Write([]byte(f.agentsRaw))
			return
		}
		reply(max(f.agentsCode, http.StatusOK), f.agents)
	case r.Method == "GET" && r.URL.Path == "/v1/build":
		if !f.tokenIsLive(authorization) && authorization != "Bearer "+secret {
			reply(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if f.buildRaw != "" {
			w.WriteHeader(f.buildStatus)
			w.Write([]byte(f.buildRaw))
			return
		}
		reply(f.buildStatus, f.buildBody)
	default:
		reply(http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (f *fakeSwitchboard) seen() []fakeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeRequest{}, f.requests...)
}

func (f *fakeSwitchboard) count(method, path string) int {
	n := 0
	for _, each := range f.seen() {
		if each.method == method && each.path == path {
			n++
		}
	}
	return n
}

type switchboardItems struct {
	first                string
	start, stop, restart bool
	build                string
}

func enabledOrDisabled(title string, enabled bool) string {
	if enabled {
		return title
	}
	return title + disabledMark
}

func indented(lines ...string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = "  " + line
	}
	return out
}

func agentRow(title, marker string, fields ...string) []string {
	lines := []string{shapeFor(marker) + " " + title, "  State: " + marker + disabledMark}
	for _, field := range fields {
		lines = append(lines, "  "+field+disabledMark)
	}
	return append(lines, "  "+menuSeparator, "  Copy id")
}

func expectedMenu(icon string, head []string, sb switchboardItems, menubarLine string) string {
	lines := []string{"icon: " + icon}
	lines = append(lines, head...)
	if len(head) > 0 {
		lines = append(lines, menuSeparator)
	}
	lines = append(lines, "Switchboard")
	lines = append(lines, indented(
		sb.first+disabledMark,
		menuSeparator,
		enabledOrDisabled("Start", sb.start),
		enabledOrDisabled("Stop", sb.stop),
		enabledOrDisabled("Restart", sb.restart),
		enabledOrDisabled("Rotate secret…", sb.restart),
		menuSeparator,
		sb.build+disabledMark,
	)...)
	lines = append(lines, "Harnesses")
	lines = append(lines, indented("Set up Claude Code", "Remove from Claude Code", "Set up Codex", "Remove from Codex")...)
	lines = append(lines, menuSeparator, menubarLine+disabledMark, "About Landline", "Refresh", menuSeparator, "Quit")
	return strings.Join(lines, "\n") + "\n"
}

func unreachableHead() []string { return []string{"Switchboard unreachable" + disabledMark} }

func notFoundLine() string { return "landline not found. Run just install" }

func localSwitchboard(address string, reachable bool, build string) switchboardItems {
	first := "Not running"
	if reachable {
		first = "Running at " + address
	}
	return switchboardItems{first: first, start: !reachable, stop: reachable, restart: reachable, build: build}
}

func shortOf(commit string) string {
	if len(commit) <= 7 {
		return commit
	}
	short := commit[:7]
	if strings.HasSuffix(commit, routeDirtyTag) {
		short += routeDirtyTag
	}
	return short
}

func menubarBuildLine(version, commit string) string {
	if commit == "" {
		return "Menubar " + version
	}
	return "Menubar " + version + " (" + shortOf(commit) + ")"
}

func switchboardBuildLine(version, commit string) string {
	if commit == "" {
		return "Switchboard " + version
	}
	return "Switchboard " + version + " (" + shortOf(commit) + ")"
}

func deadAddress(t *testing.T) string { return "http://" + freeAddr(t) }

func menuLines(text string) []string { return strings.Split(strings.TrimSuffix(text, "\n"), "\n") }

func assertNoCredentialsOnDisk(t *testing.T, rig menubarRig) {
	t.Helper()
	for _, dir := range rig.dirs {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte("observer-token-")) {
				t.Errorf("%s holds the secret or an observer token", path)
			}
			return nil
		})
	}
}

func shapeFor(state string) string {
	if state == "connected" {
		return "\u25cf"
	}
	return "\u25cb"
}
