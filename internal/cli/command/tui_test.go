package command

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

type tuiCalls struct {
	path string
	argv []string
}

func tuiFor(onPath string, beside map[string]bool, calls *tuiCalls, execErr error) TUI {
	return TUI{
		Executable: "/opt/landline/bin/landline",
		LookPath: func(name string) (string, error) {
			if name != "landline-tui" || onPath == "" {
				return "", errors.New("not found")
			}
			return onPath, nil
		},
		IsFile: func(path string) bool { return beside[path] },
		Exec: func(path string, argv []string) error {
			calls.path, calls.argv = path, argv
			return execErr
		},
	}
}

func TestTheTUICommandRunsTheBinaryOnThePathWithTheRemainingArguments(t *testing.T) {
	t.Parallel()
	calls := &tuiCalls{}
	var stderr bytes.Buffer
	tui := tuiFor("/usr/local/bin/landline-tui", map[string]bool{"/opt/landline/bin/landline-tui": true}, calls, nil)
	RunTUI([]string{"agents", "--json"}, Env{Stderr: &stderr}, tui)
	if calls.path != "/usr/local/bin/landline-tui" {
		t.Fatalf("path %q", calls.path)
	}
	if !reflect.DeepEqual(calls.argv, []string{"landline-tui", "agents", "--json"}) {
		t.Fatalf("argv %v", calls.argv)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestTheTUICommandFallsBackToTheBinaryBesideTheLandlineExecutable(t *testing.T) {
	t.Parallel()
	calls := &tuiCalls{}
	tui := tuiFor("", map[string]bool{"/opt/landline/bin/landline-tui": true}, calls, nil)
	RunTUI(nil, Env{Stderr: &bytes.Buffer{}}, tui)
	if calls.path != "/opt/landline/bin/landline-tui" {
		t.Fatalf("path %q", calls.path)
	}
	if !reflect.DeepEqual(calls.argv, []string{"landline-tui"}) {
		t.Fatalf("argv %v", calls.argv)
	}
}

func TestTheTUICommandSaysHowToInstallWhenTheBinaryIsMissing(t *testing.T) {
	t.Parallel()
	calls := &tuiCalls{}
	var stdout, stderr bytes.Buffer
	code := RunTUI([]string{"agents"}, Env{Stdout: &stdout, Stderr: &stderr}, tuiFor("", nil, calls, nil))
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if stderr.String() != "landline-tui is not installed. Build it with: just install-tui\n" {
		t.Fatalf("stderr %q", stderr.String())
	}
	if stdout.Len() != 0 || calls.path != "" {
		t.Fatalf("stdout %q, exec %q", stdout.String(), calls.path)
	}
}

func TestTheTUICommandReportsAnExecFailureAndExitsOne(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	code := RunTUI(nil, Env{Stderr: &stderr}, tuiFor("/bin/landline-tui", nil, &tuiCalls{}, errors.New("exec format error")))
	if code != 1 || stderr.String() != "landline: exec format error\n" {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}
