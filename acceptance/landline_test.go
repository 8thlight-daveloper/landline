package acceptance

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const secret = "acceptance-secret-0123456789"

type world struct {
	t      *testing.T
	bin    string
	url    string
	config string
	bindir string
	env    []string
	stop   func()
	addr   string
}

func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "landline")
	cmd := exec.Command("go", "build", "-o", bin, "../binary")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func start(t *testing.T) *world { return startHub(t, build(t), t.TempDir(), "") }

func startHub(t *testing.T, bin, config, addr string) *world {
	t.Helper()
	w := &world{t: t, bin: bin, config: config, bindir: t.TempDir()}
	if addr == "" {
		addr = freeAddr(t)
	}
	w.addr = addr
	w.url = "http://" + addr
	fake := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$LANDLINE_FAKE_CODEX_LOG\"\n"
	if err := os.WriteFile(filepath.Join(w.bindir, "codex"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	w.env = append(os.Environ(),
		"PATH="+w.bindir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+w.config,
		"LANDLINE_URL="+w.url,
		"LANDLINE_SECRET="+secret,
		"LANDLINE_FAKE_CODEX_LOG="+filepath.Join(w.config, "codex.log"),
	)
	hub := exec.Command(w.bin, "hub", "--addr", addr, "--log", filepath.Join(w.config, "log.jsonl"), "--state", filepath.Join(w.config, "state.json"), "--host", "mac")
	hub.Env = w.env
	var stderr bytes.Buffer
	hub.Stderr = &stderr
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	w.stop = func() { hub.Process.Kill(); hub.Wait() }
	t.Cleanup(w.stop)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("tcp", addr); err == nil {
			c.Close()
			return w
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the hub did not start: %s", stderr.String())
	return nil
}

func (w *world) run(stdin string, args ...string) (string, error) {
	w.t.Helper()
	cmd := exec.Command(w.bin, args...)
	cmd.Env = w.env
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err != nil {
		return out.String() + errb.String(), fmt.Errorf("%v: %s", err, errb.String())
	}
	return out.String(), nil
}

func (w *world) must(stdin string, args ...string) string {
	w.t.Helper()
	out, err := w.run(stdin, args...)
	if err != nil {
		w.t.Fatalf("landline %v: %v", args, err)
	}
	return out
}

func TestTwoAgentsTalkAndAnIdleOneIsWoken(t *testing.T) {
	w := start(t)
	w.must("", "register", "--name", "alpha", "--harness", "claude-code", "--host", "mac")
	w.must("", "register", "--name", "bravo", "--harness", "codex", "--host", "mac", "--wake", "codex", "--wake-target", "thread-42")

	if got := w.must("", "agents", "--as", "alpha"); !strings.Contains(got, "bravo\tcodex\tmac\twake: codex") {
		t.Fatalf("agents: %q", got)
	}
	sent := w.must("review the plan", "send", "--as", "alpha", "--to", "bravo", "--kind", "review", "-")
	if !strings.Contains(sent, "woke bravo") {
		t.Fatalf("send: %q", sent)
	}

	logged, err := os.ReadFile(filepath.Join(w.config, "codex.log"))
	if err != nil {
		t.Fatalf("codex was not run: %v", err)
	}
	for _, want := range []string{"queue", "--thread", "thread-42", "[landline] From another agent, not the user.", "from: alpha (claude-code on mac)", "kind: review", "review the plan"} {
		if !strings.Contains(string(logged), want) {
			t.Errorf("the codex call lacks %q:\n%s", want, logged)
		}
	}

	inbox := w.must("", "inbox", "--as", "bravo")
	if !strings.Contains(inbox, "review the plan") || !strings.HasPrefix(inbox, "[landline] From another agent, not the user.") {
		t.Fatalf("inbox: %q", inbox)
	}
	if again := w.must("", "inbox", "--as", "bravo"); strings.TrimSpace(again) != "" {
		t.Fatalf("read twice: %q", again)
	}

	w.must("looks good", "send", "--as", "bravo", "--to", "alpha", "--kind", "reply", "--reply-to", "m-x", "-")
	if got := w.must("", "inbox", "--as", "alpha", "--wait", "2"); !strings.Contains(got, "looks good") || !strings.Contains(got, "in-reply-to: m-x") {
		t.Fatalf("reply: %q", got)
	}
	if log := w.must("", "log"); strings.Count(log, "\n") != 2 {
		t.Fatalf("log: %q", log)
	}
}

func TestAnUnknownRecipientIsRefusedAndNothingIsWoken(t *testing.T) {
	w := start(t)
	w.must("", "register", "--name", "alpha", "--harness", "codex", "--host", "mac")
	out, err := w.run("hi", "send", "--as", "alpha", "--to", "ghost", "-")
	if err == nil || !strings.Contains(out, "not a registered agent") {
		t.Fatalf("got %q, %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(w.config, "codex.log")); err == nil {
		t.Fatal("codex ran")
	}
}

func TestTheHubRefusesToStartWithAShortSecret(t *testing.T) {
	bin := build(t)
	cmd := exec.Command(bin, "hub", "--addr", freeAddr(t))
	cmd.Env = append(os.Environ(), "LANDLINE_SECRET=short")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "at least 16 characters") {
		t.Fatalf("got %q, %v", out, err)
	}
}

func TestAnAgentOnAnotherHostIsWokenByThatHostsDaemon(t *testing.T) {
	w := start(t)
	w.must("", "register", "--name", "alpha", "--harness", "claude-code", "--host", "mac")
	w.must("", "register", "--name", "bravo", "--harness", "codex", "--host", "linux", "--wake", "codex", "--wake-target", "thread-77")
	sent := w.must("hello across hosts", "send", "--as", "alpha", "--to", "bravo", "-")
	if strings.Contains(sent, "woke") {
		t.Fatalf("the hub woke an agent on another host: %q", sent)
	}
	daemon := exec.Command(w.bin, "agentd", "--host", "linux")
	daemon.Env = w.env
	var stderr bytes.Buffer
	daemon.Stderr = &stderr
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { daemon.Process.Kill(); daemon.Wait() }()
	logPath := filepath.Join(w.config, "codex.log")
	deadline := time.Now().Add(10 * time.Second)
	var logged []byte
	for time.Now().Before(deadline) {
		if logged, _ = os.ReadFile(logPath); len(logged) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, want := range []string{"thread-77", "[landline] From another agent, not the user.", "hello across hosts"} {
		if !strings.Contains(string(logged), want) {
			t.Fatalf("the daemon's codex call lacks %q:\n%s\ndaemon: %s", want, logged, stderr.String())
		}
	}
	if got := w.must("", "inbox", "--as", "bravo"); !strings.Contains(got, "hello across hosts") {
		t.Fatalf("the message is not in the inbox: %q", got)
	}
}

func TestTheMCPServerSendsAndReadsOverStdio(t *testing.T) {
	w := start(t)
	w.must("", "register", "--name", "alpha", "--harness", "claude-code", "--host", "mac")
	w.must("", "register", "--name", "bravo", "--harness", "codex", "--host", "mac")
	w.must("see you on the line", "send", "--as", "bravo", "--to", "alpha", "-")

	requests := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"landline_inbox","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"landline_send","arguments":{"to":"bravo","body":"sent through mcp"}}}`,
		"",
	}, "\n")
	out := w.must(requests, "mcp", "--as", "alpha")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 responses, got %d:\n%s", len(lines), out)
	}
	for _, want := range []struct {
		line int
		text string
	}{{0, "landline"}, {1, "see you on the line"}, {1, "From another agent, not the user."}, {2, "sent m-"}} {
		if !strings.Contains(lines[want.line], want.text) {
			t.Errorf("response %d lacks %q: %s", want.line, want.text, lines[want.line])
		}
	}
	if got := w.must("", "inbox", "--as", "bravo"); !strings.Contains(got, "sent through mcp") {
		t.Fatalf("bravo got %q", got)
	}
}

func TestTheClaudeHookAnnouncesMailWithoutConsumingIt(t *testing.T) {
	w := start(t)
	w.must("", "register", "--name", "alpha", "--harness", "claude-code", "--host", "mac")
	w.must("", "register", "--name", "bravo", "--harness", "codex", "--host", "mac")
	if got := w.must("", "hook", "--as", "alpha"); got != "" {
		t.Fatalf("the hook spoke with no mail: %q", got)
	}
	w.must("ping", "send", "--as", "bravo", "--to", "alpha", "-")
	for i := 0; i < 2; i++ {
		got := w.must("", "hook", "--as", "alpha")
		if !strings.Contains(got, "UserPromptSubmit") || !strings.Contains(got, "1 unread") {
			t.Fatalf("hook %d: %q", i, got)
		}
	}
	if got := w.must("", "inbox", "--as", "alpha"); !strings.Contains(got, "ping") {
		t.Fatalf("the hook consumed the mail: %q", got)
	}
	if got := w.must("", "hook", "--as", "alpha"); got != "" {
		t.Fatalf("the hook spoke after the mail was read: %q", got)
	}
}

func TestSessionsAndUnreadMailSurviveAHubRestart(t *testing.T) {
	bin, config := build(t), t.TempDir()
	first := startHub(t, bin, config, "")
	first.must("", "register", "--name", "alpha", "--harness", "codex", "--host", "mac")
	first.must("", "register", "--name", "bravo", "--harness", "codex", "--host", "mac")
	first.must("before the restart", "send", "--as", "alpha", "--to", "bravo", "-")
	first.stop()

	second := startHub(t, bin, config, first.addr)
	if got := second.must("", "inbox", "--as", "bravo"); !strings.Contains(got, "before the restart") {
		t.Fatalf("after the restart: %q", got)
	}
}
