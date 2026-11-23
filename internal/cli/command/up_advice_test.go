package command

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const translatingSwitchboard = `#!/bin/sh
output=$(%q "$@" 2>&1)
status=$?
printf '%%s\n' "$output" | sed -e 's/cannot listen on/kann nicht lauschen auf/g' -e 's/listening on/lauscht auf/g' >&2
%s
exit $status
`

func buildRealSwitchboard(t *testing.T) string {
	t.Helper()
	goMod, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Skipf("the go tool is not available to build the switchboard: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "landline-switchboard-real")
	build := exec.Command("go", "build", "-o", binary, "./cmd/landline-switchboard")
	build.Dir = filepath.Dir(strings.TrimSpace(string(goMod)))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the switchboard: %v\n%s", err, out)
	}
	return binary
}

type upRun struct {
	env    Env
	stderr *bytes.Buffer
}

func newUpRun(t *testing.T, extraOutput string) *upRun {
	t.Helper()
	real := buildRealSwitchboard(t)
	bin, home := t.TempDir(), t.TempDir()
	wrapper := fmt.Sprintf(translatingSwitchboard, real, extraOutput)
	if err := os.WriteFile(filepath.Join(bin, SwitchboardBinaryName), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", home)
	for _, name := range []string{"LANDLINE_URL", "LANDLINE_SECRET", "LANDLINE_PORT", "XDG_CONFIG_HOME", "XDG_DATA_HOME"} {
		t.Setenv(name, "")
	}
	run := &upRun{stderr: &bytes.Buffer{}}
	run.env = Env{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: run.stderr,
		ConfigDir: filepath.Join(home, ".config", "landline"), Home: home,
		Getenv: func(string) string { return "" },
	}
	t.Cleanup(func() {
		Run([]string{"server", "down"}, Env{Stdout: io.Discard, Stderr: io.Discard, ConfigDir: run.env.ConfigDir, Home: home})
	})
	return run
}

func (r *upRun) saveAddress(t *testing.T, address string) {
	t.Helper()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(r.env.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r.env.ConfigDir, "config.toml", fmt.Sprintf("host = %q\nport = %s\n", host, port))
}

func (r *upRun) gaveSavedAddressAdvice() bool {
	text := r.stderr.String()
	return strings.Contains(text, configPath(r.env)) && strings.Contains(text, "--addr")
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}

func TestUpGivesTheSavedAddressAdviceWhenTheSwitchboardCannotListenWhateverWordsItPrints(t *testing.T) {
	r := newUpRun(t, "")
	r.saveAddress(t, listening(t))

	code := Run([]string{"server", "up"}, r.env)

	if code == 0 {
		t.Fatalf("up exited 0 though the saved address is taken: %s", r.stderr.String())
	}
	if strings.Contains(r.stderr.String(), "cannot listen on") {
		t.Fatalf("the translating switchboard printed the English text: %s", r.stderr.String())
	}
	if !r.gaveSavedAddressAdvice() {
		t.Errorf("the switchboard could not listen on the saved address and printed its reason in other words, and up gave no advice about the saved address:\n%s", r.stderr.String())
	}
}

func TestUpGivesNoSavedAddressAdviceWhenTheSwitchboardFailsForAnotherReasonAndItsOutputSaysCannotListenOn(t *testing.T) {
	address := freeAddress(t)
	r := newUpRun(t, fmt.Sprintf("echo 'landline-switchboard: cannot listen on %s: as a person might write it' >&2", address))
	r.saveAddress(t, address)
	blocker := filepath.Join(r.env.Home, "a-file")
	writeFile(t, r.env.Home, "a-file", "not a folder\n")

	code := Run([]string{"server", "up", "--log", filepath.Join(blocker, "log.jsonl")}, r.env)

	if code == 0 {
		t.Fatalf("up exited 0 though the log cannot be opened: %s", r.stderr.String())
	}
	if !strings.Contains(r.stderr.String(), "cannot listen on") {
		t.Fatalf("the switchboard output does not hold the misleading text: %s", r.stderr.String())
	}
	if r.gaveSavedAddressAdvice() {
		t.Errorf("the switchboard failed because its log cannot be opened, and up blamed the saved address because of the words in the output:\n%s", r.stderr.String())
	}
}
