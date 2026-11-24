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
	"testing"
	"time"
	"unicode"
)

const (
	hostileBefore = "alpha-host refused"
	hostileAfter  = "bravo stays"
	hostileText   = hostileBefore + "\x1b[2J\x1b]0;owned\x07\r\t\u0085\u009b31m  ‮​\x00" + hostileAfter
)

func unclean(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf)
}

func uncleanBesideNewlines(text string) bool {
	return strings.ContainsFunc(text, func(r rune) bool { return r != '\n' && unclean(r) })
}

func showsBothEnds(text string) bool {
	return strings.Contains(text, hostileBefore) && strings.Contains(text, hostileAfter)
}

func landlineBesideAFakeSwitchboard(t *testing.T, output string, status int) string {
	t.Helper()
	dir := t.TempDir()
	bin := copyBinary(t, build(t), dir)
	outputFile := filepath.Join(dir, "switchboard-output.txt")
	if err := os.WriteFile(outputFile, []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat '" + outputFile + "' >&2\nexit " + map[int]string{1: "1", 3: "3"}[status] + "\n"
	if err := os.WriteFile(filepath.Join(dir, switchboardName), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestServerUpCleansTheOutputOfASwitchboardThatFailsToStartAndDecidesTheAdviceOnlyByExitStatusThree(t *testing.T) {
	t.Parallel()
	output := "landline-switchboard: " + hostileText + "\nlandline-switchboard: cannot listen on " + hostileText + "\n"
	for _, status := range []int{3, 1} {
		bin := landlineBesideAFakeSwitchboard(t, output, status)
		home := t.TempDir()
		env := isolatedEnv(home)
		host, port := hostAndPort(t, freeAddr(t))
		writeConfigFile(t, home, "config.toml", tomlFor(host, port, secret))
		result := runBin(t, bin, env, "server", "up")
		shown := result.stdout + result.stderr
		if result.code == 0 {
			t.Fatalf("switchboard exit %d: up exited 0: %q", status, shown)
		}
		if uncleanBesideNewlines(shown) {
			t.Errorf("switchboard exit %d: a control character from switchboard.out reached the terminal: %q", status, shown)
		}
		if !showsBothEnds(shown) {
			t.Errorf("switchboard exit %d: up did not show the switchboard's output, cleaned: %q", status, shown)
		}
		advised := strings.Contains(shown, configTomlOf(home)) && strings.Contains(shown, "--addr")
		if advised != (status == 3) {
			t.Errorf("switchboard exit %d: saved-address advice given is %v, want %v: %q", status, advised, status == 3, shown)
		}
	}
}

func startErringSwitchboard(t *testing.T, registerAnswers bool) (string, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if registerAnswers && r.URL.Path == "/v1/register" {
			w.Write([]byte(`{"id":"alpha","token":"alpha-session-token"}`))
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(errorBody(map[string]string{"error": hostileText})))
	}))
	t.Cleanup(server.Close)
	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	return host, port
}

func errorBody(fields map[string]string) string {
	raw, _ := json.Marshal(fields)
	return string(raw)
}

var everyMCPTool = map[string]string{
	"landline_agents":   `{}`,
	"landline_send":     `{"to":"bravo","body":"hello"}`,
	"landline_register": `{"project":"remote-proj"}`,
	"landline_status":   `{"task":"review the plan"}`,
}

func checkMCPToolErrorsAreClean(t *testing.T, p *mcpProcess, mode string) {
	t.Helper()
	p.handshake("claude-code", "2.1.296")
	for name, arguments := range everyMCPTool {
		reply := p.request("tools/call", `{"name":"`+name+`","arguments":`+arguments+`}`)
		result, _ := reply["result"].(map[string]any)
		if result == nil || result["isError"] != true {
			t.Errorf("%s, %s: want an error result: %v", mode, name, reply)
			continue
		}
		text := result["content"].([]any)[0].(map[string]any)["text"].(string)
		if strings.ContainsFunc(text, unclean) {
			t.Errorf("%s, %s: a control character from the switchboard's error reached the tool result: %q", mode, name, text)
		}
		if !showsBothEnds(text) {
			t.Errorf("%s, %s: the tool result does not show the switchboard's error, cleaned: %q", mode, name, text)
		}
	}
}

func TestLandlineMCPCleansSwitchboardErrorTextInEveryToolResult(t *testing.T) {
	t.Parallel()
	bin := build(t)
	for _, registerAnswers := range []bool{false, true} {
		mode := map[bool]string{false: "the switchboard refuses the session's registration", true: "the switchboard refuses each tool's request"}[registerAnswers]
		host, port := startErringSwitchboard(t, registerAnswers)
		home := t.TempDir()
		writeConfigFile(t, home, "config.toml", tomlFor(host, port, secret))
		cmd := exec.Command(mcpOf(bin))
		cmd.Dir, cmd.Env = home, isolatedEnv(home)
		checkMCPToolErrorsAreClean(t, newMCP(t, cmd), mode)
	}
}

func TestLandlineMCPWithAsCleansSwitchboardErrorTextInEveryToolResult(t *testing.T) {
	t.Parallel()
	bin := build(t)
	host, port := startErringSwitchboard(t, false)
	home := t.TempDir()
	writeConfigFile(t, home, "session-alpha.json", `{"url":"http://`+net.JoinHostPort(host, port)+`","token":"alpha-session-token"}`)
	cmd := exec.Command(mcpOf(bin), "--as", "alpha")
	cmd.Dir, cmd.Env = home, isolatedEnv(home)
	checkMCPToolErrorsAreClean(t, newMCP(t, cmd), "--as alpha")
}

func TestLandlineMCPCleansSwitchboardErrorTextInItsStandardErrorNotes(t *testing.T) {
	t.Parallel()
	bin := build(t)
	host, port := startErringSwitchboard(t, true)
	home := t.TempDir()
	writeConfigFile(t, home, "config.toml", tomlFor(host, port, secret))
	cmd := exec.Command(mcpOf(bin))
	cmd.Dir, cmd.Env = home, isolatedEnv(home)
	p := newMCP(t, cmd)
	p.handshake("claude-code", "2.1.296")
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(p.stderr.String(), hostileAfter) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	notes := p.stderr.String()
	if !showsBothEnds(notes) {
		t.Fatalf("landline-mcp wrote no note of the switchboard's error on standard error: %q", notes)
	}
	if uncleanBesideNewlines(notes) {
		t.Errorf("a control character from the switchboard's error reached standard error: %q", notes)
	}
}

func TestLandlineMCPPassesAMessageBodyWithControlCharactersToTheChannelAsItCame(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("alpha", "bravo")
	p := w.startMCP(t.TempDir(), "--channel", "--as", "bravo")
	p.handshake("claude-code", "2.1.296")
	w.waitState("bravo", "connected")
	body := "keep this \x1b[31mred\x1b[0m\ttab\r\nnext line"
	if status, answer := w.sendHTTP("alpha", "bravo", body); status != 200 {
		t.Fatalf("send: %d %s", status, answer)
	}
	if note := p.notification(); !strings.HasSuffix(note.Params.Content, "\n\n"+body) {
		t.Errorf("the channel content does not end with the body as it came, %q: %q", body, note.Params.Content)
	}
}

func TestTheScreenCleansTheMessageOfASwitchboardThatRefusesThisMachinesKey(t *testing.T) {
	t.Parallel()
	bin := build(t)
	fake := startRefusingSwitchboard(t, errorBody(map[string]string{"error": hostileText, "reason": "refused"}))
	_, env := machineHomeFor(t, fake)
	result := runBin(t, bin, env, "agents", "--json")
	if result.code == 0 {
		t.Fatalf("agents succeeded against a switchboard that refuses everything: %+v", result)
	}
	if uncleanBesideNewlines(result.stdout + result.stderr) {
		t.Errorf("a control character from the refusal reached the terminal: %q", result.stderr)
	}
	if !showsBothEnds(result.stderr) {
		t.Errorf("the refusal message is not shown, cleaned: %q", result.stderr)
	}
}

func TestLandlineMCPNamesTheAddressItTriedAfterTheSavedAddressChanged(t *testing.T) {
	t.Parallel()
	bin := build(t)
	home := t.TempDir()
	first, second := freeAddr(t), freeAddr(t)
	firstHost, firstPort := hostAndPort(t, first)
	secondHost, secondPort := hostAndPort(t, second)
	writeConfigFile(t, home, "config.toml", tomlFor(firstHost, firstPort, secret))
	cmd := exec.Command(mcpOf(bin))
	cmd.Dir, cmd.Env = home, isolatedEnv(home)
	p := newMCP(t, cmd)
	p.handshake("claude-code", "2.1.296")
	writeConfigFile(t, home, "config.toml", tomlFor(secondHost, secondPort, secret))
	reply := p.request("tools/call", `{"name":"landline_agents","arguments":{}}`)
	result, _ := reply["result"].(map[string]any)
	if result == nil || result["isError"] != true {
		t.Fatalf("landline_agents with no switchboard: %v", reply)
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, second) || strings.Contains(text, first) {
		t.Errorf("the session tried %s after config.toml changed, and its error says %q", second, text)
	}
}
