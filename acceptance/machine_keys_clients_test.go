package acceptance

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRotateSecretFromTheMenuWithNoFlagsKeepsTheMachinesOfASwitchboardStartedWithACustomLog(t *testing.T) {
	t.Parallel()
	logPath := filepath.Join(t.TempDir(), "custom-data", "log.jsonl")
	h := startHostWith(t, "--log", logPath)
	m := h.machine("linux-box")
	m.registerAgent("alpha")
	if _, err := os.Stat(filepath.Join(filepath.Dir(logPath), "machines.json")); err != nil {
		t.Fatalf("machines.json is not next to the custom log before the rotation: %v", err)
	}

	out, err := h.run("server", "rotate-secret")
	if err != nil || !strings.Contains(out, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret with no flags, as the menu runs it: %v %s", err, h.clean(out))
	}

	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("after a rotation with no flags, the switchboard lists no machine, so a machine that joined has to join again")
	}
	if out, err := m.run("register", "--id", "bravo", "--harness", "claude-code", "--host", "mac"); err != nil {
		t.Errorf("after a rotation with no flags, the machine's key no longer registers an agent: %v %s", err, safe(out))
	}
}

type screenTerminal struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  *syncBuffer
}

func scriptCommand(t *testing.T, program string, args ...string) *exec.Cmd {
	t.Helper()
	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("the screen needs a terminal, and this machine has no script command to give it one")
	}
	sized := append([]string{"-c", `stty cols 160 rows 40 2>/dev/null; exec "$0" "$@"`, program}, args...)
	switch runtime.GOOS {
	case "darwin", "freebsd":
		return exec.Command(script, append([]string{"-q", "/dev/null", "/bin/sh"}, sized...)...)
	case "linux":
		quoted := make([]string, len(sized))
		for i, word := range sized {
			quoted[i] = "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
		}
		return exec.Command(script, "-q", "-e", "-c", "/bin/sh "+strings.Join(quoted, " "), "/dev/null")
	}
	t.Skipf("the screen test does not know the script command of %s", runtime.GOOS)
	return nil
}

func (m *machineRig) openScreen(t *testing.T) *screenTerminal {
	t.Helper()
	cmd := scriptCommand(t, m.host.bin, "--interval", "500ms")
	cmd.Dir, cmd.Env = m.home, append(append([]string{}, m.env...), "TERM=xterm-256color")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	screen := &screenTerminal{command: cmd, input: input, output: &syncBuffer{}}
	cmd.Stdout, cmd.Stderr = screen.output, screen.output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		input.Write([]byte("q"))
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
			<-done
		}
		input.Close()
	})
	return screen
}

func (s *screenTerminal) showsAfter(start int, text string) bool {
	all := s.output.String()
	return start <= len(all) && strings.Contains(all[start:], text)
}

func TestTheScreenOnAMachineWithOnlyAKeyRegistersAgainAfterRotateSecretAndShowsAgentsThatRegisterAfterIt(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	m.registerAgent("alpha-before")
	screen := m.openScreen(t)
	eventually(t, "the screen to show the agent registered before the rotation", func() bool { return screen.showsAfter(0, "alpha-before") })

	if out, err := h.run("server", "rotate-secret"); err != nil || !strings.Contains(out, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret: %v %s", err, h.clean(out))
	}
	mark := len(screen.output.String())
	m.registerAgent("bravo-after")

	deadline := time.Now().Add(pollDeadline)
	for time.Now().Before(deadline) && !screen.showsAfter(mark, "bravo-after") {
		time.Sleep(100 * time.Millisecond)
	}
	if !screen.showsAfter(mark, "bravo-after") {
		tail := screen.output.String()[mark:]
		t.Errorf("the running screen did not register again with the machine's key after the rotation, so it never showed the agent registered after it. It printed join advice: %v", strings.Contains(tail, "--code"))
	}
}
