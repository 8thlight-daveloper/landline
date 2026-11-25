package reaper

import (
	"fmt"
	"os"
	"time"
)

const leakGrace = 3 * time.Second

func (r *Run) Finish(code int) int {
	leaks, err := r.leaks()
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL: the leak check cannot list processes, so it cannot tell whether this run left any:", err)
		code = failed(code)
	}
	if len(leaks) > 0 {
		fmt.Fprintf(os.Stderr, "FAIL: %d process(es) started by this test run were still running after the tests ended:\n", len(leaks))
		for _, each := range leaks {
			fmt.Fprintf(os.Stderr, "  pid %d: %s\n", each.PID, each.Command)
		}
		if err := killAll(r.folder, r.excluded()); err != nil {
			fmt.Fprintln(os.Stderr, "FAIL: the leaks could not all be killed:", err)
		} else {
			fmt.Fprintln(os.Stderr, "They have been killed. Stop each process a test starts in t.Cleanup.")
		}
		code = failed(code)
	}
	os.RemoveAll(r.folder)
	r.stop()
	return code
}

func failed(code int) int {
	if code == 0 {
		return 1
	}
	return code
}

func (r *Run) leaks() ([]Process, error) {
	deadline := time.Now().Add(leakGrace)
	for {
		found, err := matching(r.folder, r.excluded())
		if err != nil || len(found) == 0 || time.Now().After(deadline) {
			return found, err
		}
		time.Sleep(killPause)
	}
}

func (r *Run) excluded() map[int]bool {
	return map[int]bool{os.Getpid(): true, r.reaper.Process.Pid: true}
}
