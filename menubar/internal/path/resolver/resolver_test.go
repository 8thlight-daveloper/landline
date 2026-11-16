package resolver

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseTakesTheLastLineAndRefusesEmptyOrControlOutput(t *testing.T) {
	cases := map[string]string{
		"/a:/b":                  "/a:/b",
		"noise from rc\n/a:/b\n": "/a:/b",
		"":                       "own",
		"  \n":                   "own",
		"/a\x1b[2J:/b":           "own",
		"/a:/b\nmotd text \x07":  "own",
	}
	for in, want := range cases {
		if got := Parse(in, "own"); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestLoginAsksTheShellForItsPathWithTheLoginFlag(t *testing.T) {
	shell := script(t, t.TempDir(), "shell", `[ "$1" = -l ] && [ "$2" = -c ] && [ "$3" = 'printf %s "$PATH"' ] && printf %s /login/bin`)
	if got := Login(shell, "own", 10*time.Second); got != "/login/bin" {
		t.Fatal(got)
	}
}

func TestLoginKeepsTheOwnPathWhenTheShellIsMissingFailsOrHangs(t *testing.T) {
	dir := t.TempDir()
	hang := script(t, dir, "hang", "sleep 30")
	fail := script(t, dir, "fail", "exit 3")
	for name, shell := range map[string]string{"empty": "", "missing": filepath.Join(dir, "nope"), "fail": fail} {
		if got := Login(shell, "own", time.Second); got != "own" {
			t.Errorf("%s: %q", name, got)
		}
	}
	start := time.Now()
	if got := Login(hang, "own", 200*time.Millisecond); got != "own" || time.Since(start) > 5*time.Second {
		t.Errorf("hang: %q after %s", got, time.Since(start))
	}
}

func TestFindLooksNextToTheBinaryBeforeThePath(t *testing.T) {
	beside, onPath := t.TempDir(), t.TempDir()
	script(t, beside, "landline", "")
	script(t, onPath, "landline", "")
	self := filepath.Join(beside, "landline-menubar")
	got, ok := Find("landline", self, onPath)
	if !ok || got != filepath.Join(beside, "landline") {
		t.Fatalf("%q %v", got, ok)
	}
	got, ok = Find("landline", filepath.Join(t.TempDir(), "landline-menubar"), onPath)
	if !ok || got != filepath.Join(onPath, "landline") {
		t.Fatalf("%q %v", got, ok)
	}
}

func TestFindSkipsDirectoriesAndFilesThatAreNotExecutable(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "landline"), 0o755)
	other := t.TempDir()
	os.WriteFile(filepath.Join(other, "landline"), []byte("x"), 0o644)
	if got, ok := Find("landline", "", dir+":"+other+": "); ok {
		t.Fatalf("found %q", got)
	}
}

func TestFindFollowsASymlinkedBinaryToItsRealFolder(t *testing.T) {
	real, links := t.TempDir(), t.TempDir()
	script(t, real, "landline", "")
	os.WriteFile(filepath.Join(real, "landline-menubar"), []byte("x"), 0o755)
	link := filepath.Join(links, "landline-menubar")
	if err := os.Symlink(filepath.Join(real, "landline-menubar"), link); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(real, "landline"))
	got, ok := Find("landline", link, "")
	if resolved, _ := filepath.EvalSymlinks(got); !ok || resolved != want {
		t.Fatalf("%q %v", got, ok)
	}
}
