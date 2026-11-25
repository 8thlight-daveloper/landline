package acceptance

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/testsupport/process/reaper"
)

const (
	reaperChildModeVariable     = "LANDLINE_REAPER_TEST_MODE"
	reaperChildBinariesVariable = "LANDLINE_REAPER_TEST_BINARIES"
	reaperChildHolds            = "hold"
	reaperChildHoldsDirectly    = "hold-directly"
	reaperChildLeaks            = "leak"
	reaperChildSweeps           = "sweep"
	reaperDeadline              = 5 * time.Second
	reaperChildDeadline         = 90 * time.Second
)

var reaperChildLine = regexp.MustCompile(`child switchboard=(\d+) folder=(\S+) reaper=(\d+)`)

func TestReaperChild(t *testing.T) {
	mode := os.Getenv(reaperChildModeVariable)
	if mode == "" {
		t.Skip("runs only as the child test process of a reaper test")
	}
	if mode == reaperChildSweeps {
		return
	}
	source := os.Getenv(reaperChildBinariesVariable)
	dir := t.TempDir()
	bin := copyBinary(t, filepath.Join(source, landlineName), dir)
	copyBinary(t, filepath.Join(source, switchboardName), dir)
	home := t.TempDir()
	env := isolatedEnv(home)
	var pid int
	if mode == reaperChildHoldsDirectly {
		pid = startChildSwitchboardDirectly(t, switchboardOf(bin), env, home)
	} else {
		pid = startChildSwitchboardThroughServerUp(t, bin, env, home, mode == reaperChildHolds)
	}
	run := reaper.Current()
	fmt.Printf("child switchboard=%d folder=%s reaper=%d\n", pid, run.Folder(), run.ReaperPID())
	if mode != reaperChildLeaks {
		io.Copy(io.Discard, os.Stdin)
	}
}

func startChildSwitchboardThroughServerUp(t *testing.T, bin string, env []string, home string, stopsAtTheEnd bool) int {
	t.Helper()
	out, err := runIn(t, bin, env, home, "server", "up", "--addr", freeAddr(t))
	if err != nil {
		t.Fatalf("server up: %v %s", err, out)
	}
	if stopsAtTheEnd {
		t.Cleanup(func() { runIn(t, bin, env, home, "server", "down") })
	}
	match := regexp.MustCompile(`running \(pid (\d+)\)`).FindStringSubmatch(out)
	if match == nil {
		t.Fatalf("server up named no pid: %s", out)
	}
	pid, _ := strconv.Atoi(match[1])
	return pid
}

func startChildSwitchboardDirectly(t *testing.T, switchboard string, env []string, home string) int {
	t.Helper()
	cmd := exec.Command(switchboard, "--addr", freeAddr(t), "--log", filepath.Join(home, "log.jsonl"))
	cmd.Env = withEnv(env, "LANDLINE_SECRET="+secret)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd.Process.Pid
}

type reaperChildOption func(*exec.Cmd)

func inItsOwnProcessGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func withPathFirst(dir string) reaperChildOption {
	return func(cmd *exec.Cmd) {
		cmd.Env = append(cmd.Env, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
}

type reaperChild struct {
	t           *testing.T
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	output      *syncBuffer
	exited      chan struct{}
	switchboard int
	folder      string
	reaper      int
}

func startReaperChild(t *testing.T, mode string, options ...reaperChildOption) *reaperChild {
	t.Helper()
	bin := build(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestReaperChild$", "-test.count=1", "-test.v", "-test.timeout=2m")
	cmd.Env = append(os.Environ(), reaperChildModeVariable+"="+mode, reaperChildBinariesVariable+"="+filepath.Dir(bin))
	for _, option := range options {
		option(cmd)
	}
	c := &reaperChild{t: t, cmd: cmd, output: &syncBuffer{}, exited: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = c.output, c.output
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	c.stdin = stdin
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { cmd.Wait(); close(c.exited) }()
	t.Cleanup(func() {
		c.stdin.Close()
		select {
		case <-c.exited:
		case <-time.After(reaperChildDeadline):
			cmd.Process.Kill()
			<-c.exited
		}
	})
	if mode == reaperChildSweeps {
		return c
	}
	deadline := time.Now().Add(reaperChildDeadline)
	for {
		if match := reaperChildLine.FindStringSubmatch(c.output.String()); match != nil {
			c.switchboard, _ = strconv.Atoi(match[1])
			c.folder = match[2]
			c.reaper, _ = strconv.Atoi(match[3])
			return c
		}
		select {
		case <-c.exited:
			t.Fatalf("the child test process ended before it started a switchboard:\n%s", c.output.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child test process started no switchboard:\n%s", c.output.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (c *reaperChild) wait() int {
	c.t.Helper()
	select {
	case <-c.exited:
	case <-time.After(reaperChildDeadline):
		c.t.Fatalf("the child test process did not end:\n%s", c.output.String())
	}
	return c.cmd.ProcessState.ExitCode()
}

func (c *reaperChild) kill() {
	c.t.Helper()
	if err := c.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		c.t.Fatal(err)
	}
	c.wait()
}

func runSweepChild(t *testing.T) string {
	t.Helper()
	c := startReaperChild(t, reaperChildSweeps)
	c.stdin.Close()
	if code := c.wait(); code != 0 {
		t.Fatalf("the sweeping child run failed with %d:\n%s", code, c.output.String())
	}
	return c.output.String()
}

func processAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	state, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && !strings.HasPrefix(strings.TrimSpace(string(state)), "Z")
}

func processParent(pid int) int {
	out, _ := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	parent, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return parent
}

func diesWithin(pid int, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return !processAlive(pid)
}

func requireOrphanedToInit(t *testing.T, pid int) {
	t.Helper()
	if parent := processParent(pid); parent != 1 && processAlive(parent) {
		t.Skipf("the orphaned switchboard %d was adopted by pid %d and not pid 1, so the sweep's parent rule does not apply on this system", pid, parent)
	}
}

func TestASwitchboardStartedByServerUpDiesWithinSecondsWhenItsTestProcessIsKilled(t *testing.T) {
	child := startReaperChild(t, reaperChildHolds)
	if !processAlive(child.switchboard) {
		t.Fatalf("the child's switchboard %d is not running", child.switchboard)
	}
	child.kill()
	if !diesWithin(child.switchboard, reaperDeadline) {
		syscall.Kill(child.switchboard, syscall.SIGKILL)
		t.Fatalf("the switchboard %d was still running %s after its test process got SIGKILL", child.switchboard, reaperDeadline)
	}
	if !goneWithin(child.folder, reaperDeadline) {
		t.Fatalf("the run folder %s was still there %s after its test process got SIGKILL", child.folder, reaperDeadline)
	}
}

func goneWithin(path string, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func TestTheReaperAndTheSweepLeaveTheSwitchboardOfAnotherLiveRunAlone(t *testing.T) {
	live := startReaperChild(t, reaperChildHolds)
	doomed := startReaperChild(t, reaperChildHolds)
	doomed.kill()
	if !diesWithin(doomed.switchboard, reaperDeadline) {
		syscall.Kill(doomed.switchboard, syscall.SIGKILL)
		t.Fatalf("the killed run's switchboard %d was still running", doomed.switchboard)
	}
	if !diesWithin(doomed.reaper, reaperDeadline) {
		t.Fatalf("the killed run's reaper %d did not exit", doomed.reaper)
	}
	if !processAlive(live.switchboard) {
		t.Fatalf("the reaper of another run killed the live run's switchboard %d", live.switchboard)
	}
	runSweepChild(t)
	if !processAlive(live.switchboard) {
		t.Fatalf("the sweep killed the live run's switchboard %d", live.switchboard)
	}
}

func TestALeakedSwitchboardFailsTheRunAndIsNamed(t *testing.T) {
	child := startReaperChild(t, reaperChildLeaks)
	code := child.wait()
	output := child.output.String()
	if !strings.Contains(output, "--- PASS: TestReaperChild") {
		t.Fatalf("the leaking child test did not pass on its own:\n%s", output)
	}
	if code == 0 {
		syscall.Kill(child.switchboard, syscall.SIGKILL)
		t.Fatalf("the run exited 0 though switchboard %d outlived it:\n%s", child.switchboard, output)
	}
	named := fmt.Sprintf("pid %d: ", child.switchboard)
	if !strings.Contains(output, named+filepath.Dir(child.folder)) || !strings.Contains(output, switchboardName) {
		t.Errorf("the failure does not name the process %q and its command line:\n%s", named, output)
	}
	if !diesWithin(child.switchboard, reaperDeadline) {
		syscall.Kill(child.switchboard, syscall.SIGKILL)
		t.Fatalf("the leaked switchboard %d was left running", child.switchboard)
	}
}

func TestTheSweepKillsTheOrphanOfARunWhoseReaperIsGoneAndLeavesOneWhoseReaperIsAlive(t *testing.T) {
	live := startReaperChild(t, reaperChildHolds)
	dead := startReaperChild(t, reaperChildHolds)
	t.Cleanup(func() { syscall.Kill(dead.switchboard, syscall.SIGKILL) })
	if err := syscall.Kill(dead.reaper, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if !diesWithin(dead.reaper, reaperDeadline) {
		t.Fatalf("the reaper %d did not die", dead.reaper)
	}
	dead.kill()
	time.Sleep(500 * time.Millisecond)
	if !processAlive(dead.switchboard) {
		t.Fatalf("the orphan %d died before the sweep, so this test cannot see the sweep work", dead.switchboard)
	}
	requireOrphanedToInit(t, dead.switchboard)
	requireOrphanedToInit(t, live.switchboard)
	output := runSweepChild(t)
	if !diesWithin(dead.switchboard, reaperDeadline) {
		t.Fatalf("the sweep left the orphan %d of a run whose reaper is gone:\n%s", dead.switchboard, output)
	}
	if !strings.Contains(output, fmt.Sprintf("killed pid %d,", dead.switchboard)) {
		t.Errorf("the sweep did not say it killed %d:\n%s", dead.switchboard, output)
	}
	if !processAlive(live.switchboard) {
		t.Fatalf("the sweep killed the switchboard %d of a run whose reaper is alive", live.switchboard)
	}
}

func (c *reaperChild) signalGroup(signal syscall.Signal) {
	c.t.Helper()
	group := c.cmd.Process.Pid
	if reaperGroup, err := syscall.Getpgid(c.reaper); err != nil || reaperGroup == group {
		c.t.Errorf("the reaper %d is in the test process's group %d, so a signal to the group reaches it too", c.reaper, group)
	}
	if err := syscall.Kill(-group, signal); err != nil {
		c.t.Fatal(err)
	}
	c.wait()
}

func TestASwitchboardDiesWithinSecondsWhenItsTestProcessGroupGetsCtrlC(t *testing.T) {
	child := startReaperChild(t, reaperChildHolds, inItsOwnProcessGroup)
	child.signalGroup(syscall.SIGINT)
	if !diesWithin(child.switchboard, reaperDeadline) {
		syscall.Kill(child.switchboard, syscall.SIGKILL)
		t.Fatalf("the switchboard %d was still running %s after its test process group got SIGINT", child.switchboard, reaperDeadline)
	}
}

func TestASwitchboardDiesWithinSecondsWhenItsTestProcessGroupGetsSIGKILL(t *testing.T) {
	child := startReaperChild(t, reaperChildHolds, inItsOwnProcessGroup)
	child.signalGroup(syscall.SIGKILL)
	if !diesWithin(child.switchboard, reaperDeadline) {
		syscall.Kill(child.switchboard, syscall.SIGKILL)
		t.Fatalf("the switchboard %d was still running %s after its test process group got SIGKILL", child.switchboard, reaperDeadline)
	}
}

func TestTheSweepLeavesAProcessOfARunWhoseReaperIsGoneWhileItsParentIsAlive(t *testing.T) {
	child := startReaperChild(t, reaperChildHoldsDirectly)
	if parent := processParent(child.switchboard); parent != child.cmd.Process.Pid {
		t.Fatalf("the switchboard's parent is %d, want the child test process %d", parent, child.cmd.Process.Pid)
	}
	if err := syscall.Kill(child.reaper, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if !diesWithin(child.reaper, reaperDeadline) {
		t.Fatalf("the reaper %d did not die", child.reaper)
	}
	output := runSweepChild(t)
	if !processAlive(child.switchboard) {
		t.Fatalf("the sweep killed the switchboard %d though its parent, the child test process, is alive:\n%s", child.switchboard, output)
	}
}

func TestTheSweepKillsTheOrphanOfARunWhoseFolderIsGone(t *testing.T) {
	dead := startReaperChild(t, reaperChildHolds)
	t.Cleanup(func() { syscall.Kill(dead.switchboard, syscall.SIGKILL) })
	if err := syscall.Kill(dead.reaper, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if !diesWithin(dead.reaper, reaperDeadline) {
		t.Fatalf("the reaper %d did not die", dead.reaper)
	}
	dead.kill()
	if err := os.RemoveAll(dead.folder); err != nil {
		t.Fatal(err)
	}
	if !processAlive(dead.switchboard) {
		t.Fatalf("the orphan %d died before the sweep, so this test cannot see the sweep work", dead.switchboard)
	}
	requireOrphanedToInit(t, dead.switchboard)
	output := runSweepChild(t)
	if !diesWithin(dead.switchboard, reaperDeadline) {
		t.Fatalf("the sweep left the orphan %d of a run whose folder is gone:\n%s", dead.switchboard, output)
	}
}

func TestARunWhoseLeakCheckCannotListProcessesFails(t *testing.T) {
	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, "ps"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	child := startReaperChild(t, reaperChildSweeps, withPathFirst(broken))
	child.stdin.Close()
	code := child.wait()
	output := child.output.String()
	if !strings.Contains(output, "--- PASS: TestReaperChild") {
		t.Fatalf("the child test did not pass on its own:\n%s", output)
	}
	if code == 0 {
		t.Fatalf("the run exited 0 though its leak check could not list processes:\n%s", output)
	}
	if !strings.Contains(output, "cannot list processes") {
		t.Errorf("the run did not say it could not list processes:\n%s", output)
	}
}
