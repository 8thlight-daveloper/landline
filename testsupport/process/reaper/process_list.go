package reaper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	killRounds = 50
	killPause  = 100 * time.Millisecond
)

var psLine = regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+(.*\S)\s*$`)

type Process struct {
	PID     int
	Parent  int
	Command string
}

func list() ([]Process, error) {
	out, err := exec.Command("ps", "-ww", "-A", "-o", "pid=,ppid=,command=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	var found []Process
	for _, line := range strings.Split(string(out), "\n") {
		parts := psLine.FindStringSubmatch(line)
		if parts == nil {
			continue
		}
		pid, _ := strconv.Atoi(parts[1])
		parent, _ := strconv.Atoi(parts[2])
		found = append(found, Process{PID: pid, Parent: parent, Command: parts[3]})
	}
	return found, nil
}

func forms(path string) []string {
	clean := filepath.Clean(path)
	all := []string{clean}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil && resolved != clean {
		all = append(all, resolved)
	}
	return all
}

func normalized(command string) string {
	for strings.Contains(command, "//") {
		command = strings.ReplaceAll(command, "//", "/")
	}
	return command
}

func names(command string, folders []string) bool {
	command = normalized(command)
	for _, folder := range folders {
		if strings.Contains(command, folder+"/") {
			return true
		}
	}
	return false
}

func matching(folder string, excluded map[int]bool) ([]Process, error) {
	all, err := list()
	if err != nil {
		return nil, err
	}
	folders := forms(folder)
	var found []Process
	for _, each := range all {
		if !excluded[each.PID] && names(each.Command, folders) {
			found = append(found, each)
		}
	}
	return found, nil
}

func KillProcessesNaming(folder string) error {
	return killAll(folder, map[int]bool{os.Getpid(): true})
}

func killAll(folder string, excluded map[int]bool) error {
	folders := forms(folder)
	var left []Process
	for round := 0; round < killRounds; round++ {
		all, err := list()
		if err != nil {
			return err
		}
		left = nil
		for _, each := range all {
			if !excluded[each.PID] && names(each.Command, folders) {
				syscall.Kill(each.PID, syscall.SIGKILL)
				left = append(left, each)
			}
		}
		if len(left) == 0 {
			return nil
		}
		time.Sleep(killPause)
	}
	return fmt.Errorf("%d process(es) were still running after %d rounds of SIGKILL, the last of them pid %d: %s", len(left), killRounds, left[len(left)-1].PID, left[len(left)-1].Command)
}
