package acceptance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	tmuxDefaultEnv = "TMUX=/tmp/tmux-501/default,123,0"
	tmuxPaneEnv    = "TMUX_PANE=%3"
)

func (w *world) selfRegisteredWithEnv(env []string, folder string) (*mcpProcess, string) {
	w.t.Helper()
	dir := filepath.Join(w.t.TempDir(), folder)
	os.MkdirAll(dir, 0o755)
	p := w.startMCPWith(env, dir)
	p.handshake("claude-code", "1.0.0")
	claimed := p.tool("landline_register", `{"label":"`+folder+`"}`)
	handle := strings.TrimRight(strings.Fields(strings.SplitN(claimed, "you are ", 2)[1])[0], ";,")
	w.waitState(handle, "connected")
	return p, handle
}

func (w *world) agentsJSONRow(handle string) map[string]any {
	w.t.Helper()
	result := runBin(w.t, w.bin, w.env, "agents", "--json")
	var snapshot struct{ Agents []map[string]any }
	if result.code != 0 || json.Unmarshal([]byte(result.stdout), &snapshot) != nil {
		w.t.Fatalf("landline agents --json: %+v", result)
	}
	for _, row := range snapshot.Agents {
		if row["handle"] == handle {
			return row
		}
	}
	w.t.Fatalf("no row for %s in %v", handle, snapshot.Agents)
	return nil
}

func TestAnMCPProcessInTmuxRegistersItsSocketAndPaneAndTheListTheTextAndTheJSONShowThem(t *testing.T) {
	w := start(t)
	_, handle := w.selfRegisteredWithEnv(withEnv(w.env, tmuxDefaultEnv, tmuxPaneEnv), "demo-folder")
	_, plain := w.selfRegistered("other-folder", "claude-code")
	entry := w.agentJSON(handle)
	if entry["tmux_socket"] != "default" || entry["tmux_pane"] != "%3" {
		t.Fatalf("the wire entry: %v", entry)
	}
	absent(t, w.agentJSON(plain), "tmux_socket", "tmux_pane")
	var fields []string
	for _, line := range strings.Split(w.must("", "agents"), "\n") {
		if strings.HasPrefix(line, handle+"\t") {
			fields = strings.Split(line, "\t")
		}
		if strings.HasPrefix(line, plain+"\t") && strings.Contains(line, "tmux") {
			t.Fatalf("an agent outside tmux shows a tmux item: %q", line)
		}
	}
	if len(fields) != 5 || !strings.Contains(fields[4], "tmux: default:%3") || !strings.Contains(fields[4], "label: demo-folder") {
		t.Fatalf("the text row: %q", fields)
	}
	row := w.agentsJSONRow(handle)
	if row["tmux_socket"] != "default" || row["tmux_pane"] != "%3" {
		t.Fatalf("the JSON row: %v", row)
	}
	absent(t, w.agentsJSONRow(plain), "tmux_socket", "tmux_pane")
}

func TestTheSocketNameIsTheLastSegmentOfTheFirstFieldOfTMUXInEitherDefaultDirectory(t *testing.T) {
	w := start(t)
	for name, c := range map[string]struct{ tmux, socket string }{
		"tmp":         {"TMUX=/tmp/tmux-501/work-2,4242,7", "work-2"},
		"private tmp": {"TMUX=/private/tmp/tmux-501/default,99,0", "default"},
		"root user":   {"TMUX=/tmp/tmux-0/default,1,0", "default"},
		"a long id":   {"TMUX=/tmp/tmux-1234567/s.1_x,1,0", "s.1_x"},
	} {
		_, handle := w.selfRegisteredWithEnv(withEnv(w.env, c.tmux, "TMUX_PANE=%12"), strings.ReplaceAll(name, " ", "-"))
		entry := w.agentJSON(handle)
		if entry["tmux_socket"] != c.socket || entry["tmux_pane"] != "%12" {
			t.Errorf("%s: %v", name, entry)
		}
	}
}

func TestAnMCPProcessOutsideTmuxOrWithASocketThatTmuxwouldNotPutInTheDefaultDirectoryRegistersNoTmuxAttributes(t *testing.T) {
	w := start(t)
	cases := map[string][]string{
		"TMUX unset":                      {"TMUX_PANE=%3"},
		"both unset":                      {},
		"both empty":                      {"TMUX=", "TMUX_PANE="},
		"TMUX_TMPDIR set":                 {tmuxDefaultEnv, tmuxPaneEnv, "TMUX_TMPDIR=/tmp"},
		"another directory":               {"TMUX=/home/user/sockets/default,1,0", tmuxPaneEnv},
		"a directory that is not tmux-N":  {"TMUX=/tmp/other-501/default,1,0", tmuxPaneEnv},
		"a non digit user id":             {"TMUX=/tmp/tmux-abc/default,1,0", tmuxPaneEnv},
		"an empty user id":                {"TMUX=/tmp/tmux-/default,1,0", tmuxPaneEnv},
		"a user id with a suffix":         {"TMUX=/tmp/tmux-501x/default,1,0", tmuxPaneEnv},
		"a nested directory":              {"TMUX=/tmp/tmux-501/sub/default,1,0", tmuxPaneEnv},
		"under var tmp":                   {"TMUX=/var/tmp/tmux-501/default,1,0", tmuxPaneEnv},
		"a relative path":                 {"TMUX=tmux-501/default,1,0", tmuxPaneEnv},
		"a socket with no directory":      {"TMUX=default,1,0", tmuxPaneEnv},
		"a dot dot directory":             {"TMUX=/tmp/../etc/tmux-501/default,1,0", tmuxPaneEnv},
		"a trailing slash":                {"TMUX=/tmp/tmux-501/,1,0", tmuxPaneEnv},
		"a leading space in the pathname": {"TMUX= /tmp/tmux-501/default,1,0", tmuxPaneEnv},
	}
	var seen []string
	for name, pairs := range cases {
		env := withEnv(w.env, pairs...)
		if name == "both unset" || name == "TMUX unset" {
			env = withoutEnv(env, "TMUX")
		}
		_, handle := w.selfRegisteredWithEnv(env, fmt.Sprintf("case-%d", len(seen)))
		seen = append(seen, name)
		t.Logf("%s registered as %s", name, handle)
		entry := w.agentJSON(handle)
		absent(t, entry, "tmux_socket", "tmux_pane")
		if line := w.agentLine(handle); strings.Contains(line, "tmux") {
			t.Errorf("%s: the text row shows a tmux item: %q", name, line)
		}
	}
}

func TestAHostileTMUXPaneRefusesTheRegistrationAndNoAgentAppears(t *testing.T) {
	for _, pane := range []string{"%abc", "3", "% 3", "%3;id", "$(id)", "`id`", "%1234567890", "-1", "%-1", "%3\nx", "%", "%3 ", "%３"} {
		t.Run(pane, func(t *testing.T) {
			w := start(t)
			observer := w.registerObserver("tui")
			dir := filepath.Join(t.TempDir(), "demo-folder")
			os.MkdirAll(dir, 0o755)
			p := w.startMCPWith(withEnv(w.env, tmuxDefaultEnv, "TMUX_PANE="+pane), dir)
			p.handshake("claude-code", "1.0.0")
			p.request("tools/call", `{"name":"landline_agents","arguments":{}}`)
			register, _ := json.Marshal(p.request("tools/call", `{"name":"landline_register","arguments":{"label":"lead"}}`))
			if strings.Contains(string(register), "you are ") {
				t.Fatalf("the register call succeeded: %s", register)
			}
			if listed := w.observerAgents(observer); len(listed) != 0 {
				t.Fatalf("an agent appeared: %v", listed)
			}
		})
	}
}

func TestAHostileSocketNameInTMUXRefusesTheRegistrationAndNoAgentAppears(t *testing.T) {
	for _, tmux := range []string{
		"TMUX=/tmp/tmux-501/bad name,1,0",
		"TMUX=/tmp/tmux-501/-x,1,0",
		"TMUX=/tmp/tmux-501/a;id,1,0",
		"TMUX=/tmp/tmux-501/$(id),1,0",
		"TMUX=/tmp/tmux-501/" + strings.Repeat("s", 65) + ",1,0",
	} {
		t.Run(tmux, func(t *testing.T) {
			w := start(t)
			observer := w.registerObserver("tui")
			dir := filepath.Join(t.TempDir(), "demo-folder")
			os.MkdirAll(dir, 0o755)
			p := w.startMCPWith(withEnv(w.env, tmux, tmuxPaneEnv), dir)
			p.handshake("claude-code", "1.0.0")
			p.request("tools/call", `{"name":"landline_agents","arguments":{}}`)
			if listed := w.observerAgents(observer); len(listed) != 0 {
				t.Fatalf("an agent appeared: %v", listed)
			}
		})
	}
}

func TestAnMCPProcessWithASocketNameOfExactlySixtyFourBytesRegistersIt(t *testing.T) {
	w := start(t)
	socket := strings.Repeat("s", 64)
	_, handle := w.selfRegisteredWithEnv(withEnv(w.env, "TMUX=/tmp/tmux-501/"+socket+",1,0", tmuxPaneEnv), "demo-folder")
	if got := w.agentJSON(handle)["tmux_socket"]; got != socket {
		t.Fatalf("tmux_socket = %v", got)
	}
}

func TestTheTmuxAttributesOverRawHTTPAreAcceptedWhenValidAndRefusedWithNothingRegisteredWhenNot(t *testing.T) {
	w := start(t)
	register := func(fields map[string]string) (int, string) {
		body := map[string]string{"name": "alpha", "harness": "codex", "host": "mac"}
		for key, value := range fields {
			body[key] = value
		}
		return w.post("/v1/register", secret, body)
	}
	if status, body := register(map[string]string{"tmux_socket": "default", "tmux_pane": "%3"}); status != http.StatusOK {
		t.Fatalf("a valid registration: %d %s", status, body)
	}
	if entry := w.agentJSON("alpha"); entry["tmux_socket"] != "default" || entry["tmux_pane"] != "%3" {
		t.Fatalf("entry %v", entry)
	}
	if line := w.agentLine("alpha"); !strings.Contains(line, "tmux: default:%3") {
		t.Fatalf("line %q", line)
	}
	if status, body := register(map[string]string{"tmux_socket": "work", "tmux_pane": "%9"}); status != http.StatusOK {
		t.Fatalf("a second registration: %d %s", status, body)
	}
	if entry := w.agentJSON("alpha"); entry["tmux_socket"] != "work" || entry["tmux_pane"] != "%9" {
		t.Fatalf("a registration by name did not replace the values: %v", entry)
	}
	for name, fields := range map[string]map[string]string{
		"a space in the socket":      {"tmux_socket": "a b", "tmux_pane": "%3"},
		"a semicolon":                {"tmux_socket": "a;b", "tmux_pane": "%3"},
		"a command substitution":     {"tmux_socket": "$(id)", "tmux_pane": "%3"},
		"backticks":                  {"tmux_socket": "`id`", "tmux_pane": "%3"},
		"a leading dash":             {"tmux_socket": "-x", "tmux_pane": "%3"},
		"a newline in the socket":    {"tmux_socket": "a\nb", "tmux_pane": "%3"},
		"a socket over 64 bytes":     {"tmux_socket": strings.Repeat("s", 65), "tmux_pane": "%3"},
		"a bare percent sign":        {"tmux_socket": "default", "tmux_pane": "%"},
		"letters after the percent":  {"tmux_socket": "default", "tmux_pane": "%abc"},
		"a pane over nine digits":    {"tmux_socket": "default", "tmux_pane": "%1234567890"},
		"a newline in the pane":      {"tmux_socket": "default", "tmux_pane": "%3\n"},
		"a pane with a leading dash": {"tmux_socket": "default", "tmux_pane": "-%3"},
		"a bad pane and no socket":   {"tmux_pane": "%abc"},
		"a bad socket and no pane":   {"tmux_socket": "a b"},
	} {
		fields["name"] = "bravo"
		if status, body := register(fields); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, status, body)
		}
		if w.state("bravo") != "" {
			t.Fatalf("%s: an agent was registered", name)
		}
	}
	if status, body := register(map[string]string{"tmux_socket": "a b", "tmux_pane": "%3"}); status != http.StatusBadRequest {
		t.Fatalf("a refused re-registration: %d %s", status, body)
	}
	if entry := w.agentJSON("alpha"); entry["tmux_socket"] != "work" || entry["tmux_pane"] != "%9" {
		t.Fatalf("a refused re-registration changed the entry: %v", entry)
	}
	if status, body := register(nil); status != http.StatusOK {
		t.Fatalf("registering without tmux values: %d %s", status, body)
	}
	absent(t, w.agentJSON("alpha"), "tmux_socket", "tmux_pane")
	if status, body := w.post("/v1/register", secret, map[string]string{"seed": "gamma", "harness": "codex", "host": "mac", "tmux_socket": "default", "tmux_pane": "%4"}); status != http.StatusOK {
		t.Fatalf("a seed registration: %d %s", status, body)
	}
}

func TestNoLabelUpdateAttachCallFlagEnvironmentVariableOrRegisterArgumentCanSetTheTmuxAttributes(t *testing.T) {
	w := start(t)
	token := w.registerAgentToken("alpha")
	for _, field := range []string{"tmux_socket", "tmux_pane"} {
		if status, body := w.post("/v1/attach", token, map[string]string{"label": "mine", field: "default"}); status != http.StatusBadRequest {
			t.Errorf("attach with %s: %d %s, want 400", field, status, body)
		}
	}
	absent(t, w.agentJSON("alpha"), "tmux_socket", "tmux_pane")
	forged := withoutEnv(w.env, "TMUX", "TMUX_PANE")
	forged = append(forged, "LANDLINE_TMUX_SOCKET=forged", "LANDLINE_TMUX_PANE=%9", "LANDLINE_TMUX=/tmp/tmux-501/forged,1,0")
	dir := filepath.Join(t.TempDir(), "demo-folder")
	os.MkdirAll(dir, 0o755)
	p := w.startMCPWith(forged, dir, "--label", "lead")
	p.handshake("claude-code", "1.0.0")
	p.request("tools/call", `{"name":"landline_register","arguments":{"label":"lead","tmux_socket":"forged","tmux_pane":"%9","tmux":"forged"}}`)
	p.tool("landline_agents", `{}`)
	handle := ""
	eventually(t, "the agent to be listed", func() bool {
		for _, entry := range w.agentsJSON() {
			if entry["label"] == "lead" {
				handle, _ = entry["name"].(string)
				return true
			}
		}
		return false
	})
	absent(t, w.agentJSON(handle), "tmux_socket", "tmux_pane")
	for name, args := range map[string][]string{
		"--tmux-socket": {"--tmux-socket", "forged"},
		"--tmux-pane":   {"--tmux-pane", "%9"},
		"--tmux":        {"--tmux", "forged"},
	} {
		cmd := exec.Command(mcpOf(w.bin), args...)
		cmd.Env, cmd.Dir = forged, dir
		cmd.Stdin = strings.NewReader("")
		if err := cmd.Run(); err == nil {
			t.Errorf("%s: landline-mcp accepted the flag", name)
		}
	}
	for _, entry := range w.agentsJSON() {
		if entry["tmux_socket"] == "forged" || entry["tmux_pane"] == "%9" {
			t.Errorf("a forged value was registered: %v", entry)
		}
	}
	if out, err := w.run("", "register", "--name", "bravo", "--harness", "codex", "--tmux-socket", "default"); err == nil {
		t.Errorf("landline register accepted --tmux-socket: %q", out)
	}
	if w.state("bravo") != "" {
		t.Error("a registration with a tmux flag registered an agent")
	}
}
