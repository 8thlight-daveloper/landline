package reaper

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) { Main(m, "landline-reaper-test-") }

func startBlockedOn(t *testing.T, fifo string) *exec.Cmd {
	t.Helper()
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", `read _ < "$0"`, fifo)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd
}

func TestKillingARunNeverKillsAProcessOfARunWhoseFolderNameItPrefixes(t *testing.T) {
	parent := t.TempDir()
	short, long := filepath.Join(parent, "run-ab"), filepath.Join(parent, "run-abc")
	for _, folder := range []string{short, long} {
		if err := os.Mkdir(folder, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	other := startBlockedOn(t, filepath.Join(long, "fifo"))
	found, err := matching(short, map[int]bool{os.Getpid(): true})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("the processes of %s include another run's: %+v", short, found)
	}
	if err := killAll(short, map[int]bool{os.Getpid(): true}); err != nil {
		t.Fatal(err)
	}
	if syscall.Kill(other.Process.Pid, 0) != nil || other.ProcessState != nil {
		t.Fatalf("killing the processes of %s killed pid %d of %s", short, other.Process.Pid, long)
	}
}

func TestKillAllFailsWhenProcessesOfTheRunAreStillRunningAfterItsRounds(t *testing.T) {
	folder := t.TempDir()
	fifo := filepath.Join(folder, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	respawner := exec.Command("sh", "-c", `while kill -0 $PPID 2>/dev/null; do sh -c 'read _ < "$0"' "$BLOCKED_ON"; done`)
	respawner.Env = append(os.Environ(), "BLOCKED_ON="+fifo)
	if err := respawner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		respawner.Process.Kill()
		respawner.Wait()
		KillProcessesNaming(folder)
	})
	excluded := map[int]bool{os.Getpid(): true, respawner.Process.Pid: true}
	deadline := time.Now().Add(5 * time.Second)
	for found, _ := matching(folder, excluded); len(found) == 0; found, _ = matching(folder, excluded) {
		if time.Now().After(deadline) {
			t.Fatal("the respawner started nothing under the folder")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := killAll(folder, excluded); err == nil {
		t.Fatal("killAll reported success while a process kept starting again under the folder")
	}
}
