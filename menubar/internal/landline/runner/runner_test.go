package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/testsupport/process/reaper"
)

func fake(t *testing.T, body string) *Runner {
	t.Helper()
	path := filepath.Join(t.TempDir(), "landline")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return New(path, "/bin:/usr/bin")
}

func TestASuccessfulCommandReturnsNoFailureAndItsOutputIsDiscarded(t *testing.T) {
	r := fake(t, `echo noisy; echo noisy >&2`)
	if failure, err := r.Run([]string{"server", "up"}); failure != "" || err != nil {
		t.Fatalf("%q %v", failure, err)
	}
}

func TestACommandGetsExactlyTheArgumentsItWasGiven(t *testing.T) {
	out := filepath.Join(t.TempDir(), "args")
	r := fake(t, `printf '%s|' "$@" > `+out)
	r.Run([]string{"setup", "claude", "--remove"})
	raw, _ := os.ReadFile(out)
	if string(raw) != "setup|claude|--remove|" {
		t.Fatalf("%q", raw)
	}
}

func TestANonZeroExitShowsTheFirstLineOfStandardErrorSanitised(t *testing.T) {
	r := fake(t, `printf '\n  claude: command not found\x1b[2J\nsecond line\n' >&2; exit 4`)
	failure, _ := r.Run([]string{"setup", "claude"})
	if failure != "Failed: claude: command not found [2J" {
		t.Fatalf("%q", failure)
	}
}

func TestAFailureWithNoStandardErrorNamesTheExit(t *testing.T) {
	failure, _ := fake(t, `exit 3`).Run(nil)
	if failure != "Failed: exit status 3" {
		t.Fatalf("%q", failure)
	}
}

func TestALongStandardErrorLineIsCut(t *testing.T) {
	r := fake(t, `head -c 3000 /dev/zero | tr '\0' x >&2; exit 1`)
	failure, _ := r.Run(nil)
	if n := len([]rune(failure)); n > len("Failed: ")+maxFailedRune {
		t.Fatalf("%d runes", n)
	}
}

func TestACommandThatRunsPastTheLimitIsStoppedAndCountedAsFailed(t *testing.T) {
	r := fake(t, `read _ < "$(dirname "$0")/never-written"`)
	neverWritten(t, r)
	r.Limit = 300 * time.Millisecond
	start := time.Now()
	failure, _ := r.Run(nil)
	if !strings.HasPrefix(failure, "Failed: timed out after") || time.Since(start) > 6*time.Second {
		t.Fatalf("%q after %s", failure, time.Since(start))
	}
}

func TestTheLimitIsThirtySeconds(t *testing.T) {
	if New("x", "").Limit != 30*time.Second {
		t.Fatal("limit")
	}
}

func TestACommandThatLeavesAChildHoldingItsOutputReturnsAfterItExits(t *testing.T) {
	r := fake(t, `blocked="$(dirname "$0")/never-written"; (sh -c 'read _ < "$0"' "$blocked" >/dev/null 2>&1 &) ; sh -c 'read _ < "$0"' "$blocked" >&2 & exit 0`)
	neverWritten(t, r)
	t.Cleanup(func() { reaper.KillProcessesNaming(filepath.Dir(r.Binary)) })
	start := time.Now()
	failure, _ := r.Run(nil)
	if failure != "" || time.Since(start) > 10*time.Second {
		t.Fatalf("%q after %s", failure, time.Since(start))
	}
}

func TestOnlyOneCommandRunsAtATime(t *testing.T) {
	r := fake(t, `sleep 1`)
	done := make(chan struct{})
	go func() { r.Run(nil); close(done) }()
	time.Sleep(300 * time.Millisecond)
	if _, err := r.Run(nil); !errors.Is(err, ErrBusy) {
		t.Errorf("a second run got %v", err)
	}
	<-done
	if _, err := r.Run(nil); errors.Is(err, ErrBusy) {
		t.Error("the lock was not released")
	}
}

func TestTheChildGetsTheLoginPathAndNoOtherPathEntry(t *testing.T) {
	out := filepath.Join(t.TempDir(), "path")
	r := fake(t, `/usr/bin/printf %s "$PATH" > `+out)
	r.Path = "/fixed/login/path"
	t.Setenv("PATH", "/own/path:/usr/bin:/bin")
	r.Run(nil)
	raw, _ := os.ReadFile(out)
	if string(raw) != "/fixed/login/path" {
		t.Fatalf("%q", raw)
	}
}

func TestWithPathReplacesEveryPathEntryAndKeepsTheRest(t *testing.T) {
	got := WithPath([]string{"A=1", "PATH=/old", "B=2"}, "/new")
	if strings.Join(got, ",") != "A=1,B=2,PATH=/new" {
		t.Fatal(got)
	}
	if got := WithPath([]string{"PATH=/old"}, ""); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestNoBinaryFailsWithTheJustInstallLine(t *testing.T) {
	failure, err := New("", "").Run(nil)
	if err != nil || failure != "Failed: landline not found. Run just install" {
		t.Fatalf("%q %v", failure, err)
	}
}

func TestAMissingBinaryFails(t *testing.T) {
	failure, _ := New("/does/not/exist", "").Run(nil)
	if !strings.HasPrefix(failure, "Failed: ") {
		t.Fatalf("%q", failure)
	}
}

func TestTheCapKeepsTheFirstBytesOnly(t *testing.T) {
	c := &capped{limit: 4}
	c.Write([]byte("ab"))
	n, _ := c.Write([]byte("cdef"))
	if n != 4 || c.String() != "abcd" {
		t.Fatalf("%d %q", n, c.String())
	}
}

func neverWritten(t *testing.T, r *Runner) {
	t.Helper()
	if err := syscall.Mkfifo(filepath.Join(filepath.Dir(r.Binary), "never-written"), 0o600); err != nil {
		t.Fatal(err)
	}
}
