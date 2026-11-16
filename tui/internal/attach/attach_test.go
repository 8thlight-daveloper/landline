package attach

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	wantDisplayFormat = "#{session_name} #{pane_current_command} #{pane_current_path}"
	nixTmuxPath       = "/nix/store/0rw6c2f25c7z5w2apss5ljif9jig68bm-tmux-3.8/bin/tmux"
)

func TestWithoutTmuxAttributesOrWithABadValueTheTUICannotAttachAndSaysWhy(t *testing.T) {
	long := strings.Repeat("s", 65)
	for name, c := range map[string]struct{ host, socket, pane string }{
		"no attributes":             {"mac", "", ""},
		"no socket":                 {"mac", "", "%3"},
		"no pane":                   {"mac", "default", ""},
		"a socket with a space":     {"mac", "a b", "%3"},
		"a socket with a semicolon": {"mac", "a;id", "%3"},
		"a leading dash socket":     {"mac", "-oFoo", "%3"},
		"a command substitution":    {"mac", "$(id)", "%3"},
		"a socket over 64 bytes":    {"mac", long, "%3"},
		"a socket with a newline":   {"mac", "a\nb", "%3"},
		"a bare percent":            {"mac", "default", "%"},
		"letters in the pane":       {"mac", "default", "%abc"},
		"a pane without a percent":  {"mac", "default", "3"},
		"a pane over nine digits":   {"mac", "default", "%1234567890"},
		"a leading dash pane":       {"mac", "default", "-t"},
		"a pane with a newline":     {"mac", "default", "%3\n"},
		"a bad socket on a remote":  {"buildbox", "a b", "%3"},
		"a bad pane on a remote":    {"buildbox", "default", "%x"},
		"no attributes on a remote": {"buildbox", "", ""},
	} {
		got := Decide("mac", c.host, c.socket, c.pane)
		if got.Outcome != Cannot || strings.TrimSpace(got.Message) == "" {
			t.Errorf("%s: got %+v", name, got)
		}
		if strings.Contains(got.Message, "ssh -t") {
			t.Errorf("%s: the message offers an ssh line: %q", name, got.Message)
		}
	}
}

func TestAnAgentOnAnotherHostGetsTheSSHLineAndNothingElse(t *testing.T) {
	got := Decide("mac", "buildbox", "default", "%3")
	want := "ssh -t buildbox tmux -L default attach-session -t '%3'"
	if got.Outcome != Remote || got.Message != want {
		t.Fatalf("got %+v, want the line %q", got, want)
	}
	if got := Decide("mac", "build-box.example.org", "work-2", "%123456789"); got.Outcome != Remote || got.Message != "ssh -t build-box.example.org tmux -L work-2 attach-session -t '%123456789'" {
		t.Fatalf("got %+v", got)
	}
	if got := Decide("mac", "9host", "a", "%0"); got.Outcome != Remote {
		t.Fatalf("a host that starts with a digit: %+v", got)
	}
}

func TestTheSSHLineIsOfferedOnlyForAHostThatMatchesTheHostPattern(t *testing.T) {
	for _, host := range []string{
		"", "-oProxyCommand=id", "-host", ".host", "_host", "a b", "a;id", "a$(id)", "a`id`", "host\n", "host name", "a/b", "a:b", "a@b", "host'x", "host\"x", "é", "host‮",
		strings.Repeat("h", 65),
	} {
		got := Decide("mac", host, "default", "%3")
		if got.Outcome != Cannot || strings.Contains(got.Message, "ssh") && strings.Contains(got.Message, "attach-session") {
			t.Errorf("host %q: got %+v", host, got)
		}
	}
	if got := Decide("mac", strings.Repeat("h", 64), "default", "%3"); got.Outcome != Remote {
		t.Errorf("a host of 64 bytes: %+v", got)
	}
}

func TestAnAgentOnThisHostNeedsTheConfirmStepAndGetsNoSSHLine(t *testing.T) {
	got := Decide("mac", "mac", "default", "%3")
	if got.Outcome != Local || strings.Contains(got.Message, "ssh") {
		t.Fatalf("got %+v", got)
	}
}

func TestTheHostsAreComparedExactly(t *testing.T) {
	if got := Decide("mac", "Mac", "default", "%3"); got.Outcome != Remote {
		t.Errorf("a host that differs in case: %+v", got)
	}
	if got := Decide("mac", "mac.local", "default", "%3"); got.Outcome != Remote {
		t.Errorf("a host with a domain: %+v", got)
	}
}

func TestAnAgentWithNoHostIsAnotherHostWhoseNameFailsThePattern(t *testing.T) {
	if got := Decide("mac", "", "default", "%3"); got.Outcome != Cannot {
		t.Fatalf("got %+v", got)
	}
}

func testEnvironment(extra ...string) []string {
	return append([]string{"PATH=/usr/bin:/bin", "HOME=/home/dev", "TERM=xterm-256color"}, extra...)
}

func testHas(env []string, name string) bool {
	return slices.ContainsFunc(env, func(pair string) bool { return strings.HasPrefix(pair, name+"=") })
}

func TestTheInspectCommandIsADisplayMessageAnArgumentListWithTheFormatAndNoShell(t *testing.T) {
	got := InspectCommand(testEnvironment(), "default", "%3")
	want := []string{"-L", "default", "display-message", "-p", "-t", "%3", wantDisplayFormat}
	if !slices.Equal(got.Args, want) {
		t.Fatalf("got %q, want %q", got.Args, want)
	}
}

func TestTheSelectCommandsAreSelectWindowThenSelectPaneOnTheSameTarget(t *testing.T) {
	got := SelectCommands(testEnvironment(), "work", "%12")
	if len(got) != 2 || !slices.Equal(got[0].Args, []string{"-L", "work", "select-window", "-t", "%12"}) || !slices.Equal(got[1].Args, []string{"-L", "work", "select-pane", "-t", "%12"}) {
		t.Fatalf("got %q", got)
	}
}

func TestNoCommandOfTheTUIRunsWithTMUXTMPDIRInItsEnvironment(t *testing.T) {
	environ := testEnvironment("TMUX_TMPDIR=/somewhere/else")
	commands := []Command{InspectCommand(environ, "default", "%3"), AttachCommand(environ, "default", "%3")}
	commands = append(commands, SelectCommands(environ, "default", "%3")...)
	for _, c := range commands {
		if testHas(c.Env, "TMUX_TMPDIR") {
			t.Errorf("%q runs with TMUX_TMPDIR: %q", c.Args, c.Env)
		}
		for _, keep := range []string{"PATH", "HOME", "TERM"} {
			if !testHas(c.Env, keep) {
				t.Errorf("%q lost %s: %q", c.Args, keep, c.Env)
			}
		}
	}
}

func TestOutsideTmuxTheTUIAttachesWithAttachSessionAndNoTMUXInTheChildEnvironment(t *testing.T) {
	got := AttachCommand(testEnvironment(), "default", "%3")
	if !slices.Equal(got.Args, []string{"-L", "default", "attach-session", "-t", "%3"}) || testHas(got.Env, "TMUX") {
		t.Fatalf("got %+v", got)
	}
}

func TestInsideTheSameSocketTheTUISwitchesTheClientAndKeepsTMUXInTheChildEnvironment(t *testing.T) {
	for _, tmux := range []string{"TMUX=/tmp/tmux-501/default,1234,0", "TMUX=/private/tmp/tmux-501/default,99,2"} {
		got := AttachCommand(testEnvironment(tmux), "default", "%3")
		if !slices.Equal(got.Args, []string{"-L", "default", "switch-client", "-t", "%3"}) || !slices.Contains(got.Env, tmux) {
			t.Errorf("%s: got %+v", tmux, got)
		}
	}
}

func TestInsideAnotherSocketTheTUIAttachesWithTMUXUnsetSinceTmuxRefusesToNest(t *testing.T) {
	for name, environ := range map[string][]string{
		"another socket":         testEnvironment("TMUX=/tmp/tmux-501/other,1234,0"),
		"another directory":      testEnvironment("TMUX=/home/dev/sockets/default,1234,0"),
		"a TMUX_TMPDIR":          testEnvironment("TMUX=/tmp/tmux-501/default,1234,0", "TMUX_TMPDIR=/tmp"),
		"a bad directory suffix": testEnvironment("TMUX=/tmp/tmux-501x/default,1234,0"),
		"a nested directory":     testEnvironment("TMUX=/tmp/tmux-501/a/default,1234,0"),
	} {
		got := AttachCommand(environ, "default", "%3")
		if !slices.Equal(got.Args, []string{"-L", "default", "attach-session", "-t", "%3"}) || testHas(got.Env, "TMUX") || testHas(got.Env, "TMUX_TMPDIR") {
			t.Errorf("%s: got %+v", name, got)
		}
	}
}

func TestAttachingNeverChangesTheCallersEnvironmentSlice(t *testing.T) {
	environ := testEnvironment("TMUX=/tmp/tmux-501/other,1,0", "TMUX_TMPDIR=/x")
	before := slices.Clone(environ)
	InspectCommand(environ, "default", "%3")
	SelectCommands(environ, "default", "%3")
	AttachCommand(environ, "default", "%3")
	if !slices.Equal(environ, before) {
		t.Fatalf("the slice changed: %q", environ)
	}
}

func TestTheConfirmationShowsTmuxsAnswerWithTheTargetMarkedAsTmuxsAndNotTheAgents(t *testing.T) {
	text, ok := Confirmation("default", "%3", "work vim /home/dev/app\n")
	if !ok {
		t.Fatal("not ok")
	}
	lower := strings.ToLower(text)
	for _, want := range []string{"tmux", "work vim /home/dev/app", "default", "%3", "y"} {
		if !strings.Contains(lower, strings.ToLower(want)) {
			t.Errorf("missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, "\n") {
		t.Errorf("the confirmation spans lines: %q", text)
	}
}

func TestTheConfirmationStopsOnAnEmptyOrWhitespaceAnswer(t *testing.T) {
	for _, answer := range []string{"", "\n", "   \n", "\t \n\n", "   ", " "} {
		text, ok := Confirmation("default", "%3", answer)
		if ok || strings.TrimSpace(text) == "" {
			t.Errorf("answer %q: got %q, %v", answer, text, ok)
		}
	}
}

func TestTheConfirmationCarriesNoControlRuneFromTmuxsAnswer(t *testing.T) {
	text, ok := Confirmation("default", "%3", "s\x1b]0;pwned\x07 cmd /p‮ath x\r\n")
	if !ok || !strings.Contains(text, "pwned") {
		t.Fatalf("got %q, %v", text, ok)
	}
	for _, r := range text {
		if r < ' ' || r == 0x7f || r == 0x202e || r == 0x2028 || r == 0x85 {
			t.Fatalf("%U in %q", r, text)
		}
	}
}

func testTmuxBinary(t *testing.T) string {
	t.Helper()
	if path, err := exec.LookPath("tmux"); err == nil {
		return path
	}
	if _, err := os.Stat(nixTmuxPath); err == nil {
		return nixTmuxPath
	}
	t.Skip("tmux is not installed")
	return ""
}

func testRunCommand(bin string, c Command) (string, error) {
	cmd := exec.Command(bin, c.Args...)
	cmd.Env = c.Env
	out, err := cmd.Output()
	return string(out), err
}

func TestTheInspectAndSelectCommandsWorkAgainstARealTmuxOnAThrowawaySocket(t *testing.T) {
	bin := testTmuxBinary(t)
	socket := fmt.Sprintf("landline-test-%d-%d", os.Getpid(), time.Now().UnixNano()%1000000)
	var clean []string
	for _, pair := range os.Environ() {
		if !strings.HasPrefix(pair, "TMUX=") && !strings.HasPrefix(pair, "TMUX_TMPDIR=") && !strings.HasPrefix(pair, "TMUX_PANE=") {
			clean = append(clean, pair)
		}
	}
	dir := t.TempDir()
	setup := exec.Command(bin, "-L", socket, "-f", "/dev/null", "new-session", "-d", "-s", "probe", "-c", dir, "-x", "80", "-y", "24", "sleep 120")
	setup.Env = clean
	if out, err := setup.CombinedOutput(); err != nil {
		t.Skipf("tmux cannot start a server here: %v: %s", err, out)
	}
	t.Cleanup(func() {
		kill := exec.Command(bin, "-L", socket, "kill-server")
		kill.Env = clean
		kill.Run()
	})
	listed := exec.Command(bin, "-L", socket, "list-panes", "-t", "probe", "-F", "#{pane_id}")
	listed.Env = clean
	raw, err := listed.Output()
	pane := strings.TrimSpace(string(raw))
	if err != nil || !strings.HasPrefix(pane, "%") {
		t.Fatalf("the pane id is %q: %v", pane, err)
	}
	answer, err := testRunCommand(bin, InspectCommand(clean, socket, pane))
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if fields := strings.Fields(answer); len(fields) < 3 || fields[0] != "probe" || !strings.HasSuffix(strings.TrimSpace(answer), filepath.Base(dir)) {
		t.Fatalf("tmux answered %q", answer)
	}
	if text, ok := Confirmation(socket, pane, answer); !ok || !strings.Contains(text, "probe") {
		t.Fatalf("got %q, %v", text, ok)
	}
	for _, c := range SelectCommands(clean, socket, pane) {
		if out, err := testRunCommand(bin, c); err != nil {
			t.Fatalf("%q: %v %q", c.Args, err, out)
		}
	}
	missing, err := testRunCommand(bin, InspectCommand(clean, socket, "%999"))
	if err != nil {
		t.Skipf("this tmux fails on a missing pane instead of printing an empty line: %v", err)
	}
	if strings.TrimSpace(missing) != "" {
		t.Fatalf("a missing pane printed %q", missing)
	}
	if text, ok := Confirmation(socket, "%999", missing); ok {
		t.Fatalf("a missing pane was confirmed: %q", text)
	}
}
