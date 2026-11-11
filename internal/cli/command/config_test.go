package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheSavedConfigIsPrivateAndReadBack(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "a", "b")
	env := Env{ConfigDir: dir}
	if err := SaveConfig(env, Config{URL: "http://127.0.0.1:9", Secret: "0123456789abcdef"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", info, err)
	}
	if dirInfo, _ := os.Stat(dir); dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("folder %v", dirInfo.Mode().Perm())
	}
	if got := LoadConfig(env); got.URL != "http://127.0.0.1:9" || got.Secret != "0123456789abcdef" {
		t.Fatalf("%+v", got)
	}
}

func TestSavingOverALooseConfigMakesItPrivate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "config.json.tmp"), []byte("{}"), 0o666)
	if err := SaveConfig(Env{ConfigDir: dir}, Config{URL: "http://x:1", Secret: "0123456789abcdef"}); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(filepath.Join(dir, "config.json")); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestTheEnvironmentWinsOverTheSavedConfigAndTheConfigFillsTheGaps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	SaveConfig(Env{ConfigDir: dir}, Config{URL: "http://saved:1", Secret: "saved-secret-0123456789"})
	get := func(values map[string]string) Env {
		return withConfig(Env{ConfigDir: dir, Getenv: func(k string) string { return values[k] }})
	}
	if e := get(nil); e.Getenv("LANDLINE_URL") != "http://saved:1" || e.Getenv("LANDLINE_SECRET") != "saved-secret-0123456789" {
		t.Fatalf("fill: %q %q", e.Getenv("LANDLINE_URL"), e.Getenv("LANDLINE_SECRET"))
	}
	if e := get(map[string]string{"LANDLINE_URL": "http://env:2", "LANDLINE_SECRET": "env-secret-0123456789"}); e.Getenv("LANDLINE_URL") != "http://env:2" || e.Getenv("LANDLINE_SECRET") != "env-secret-0123456789" {
		t.Fatalf("env: %q", e.Getenv("LANDLINE_URL"))
	}
	if e := get(map[string]string{"HOME": "/x"}); e.Getenv("HOME") != "/x" || e.Getenv("OTHER") != "" {
		t.Fatal("other variables must pass through unchanged")
	}
}

func TestTheHubURLDefaultsToLocalhost(t *testing.T) {
	t.Parallel()
	if got := HubURL(Env{ConfigDir: t.TempDir()}); got != DefaultURL {
		t.Fatalf("%q", got)
	}
	dir := t.TempDir()
	SaveConfig(Env{ConfigDir: dir}, Config{URL: "http://tailnet-mac:8787", Secret: "0123456789abcdef"})
	if got := HubURL(Env{ConfigDir: dir}); got != "http://tailnet-mac:8787" {
		t.Fatalf("%q", got)
	}
}

func TestJoinSavesTheAddressAndSecretAndRefusesBadOnes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := Env{ConfigDir: dir, Stdout: &out, Stderr: &errOut, Stdin: strings.NewReader("a-long-enough-secret-1\n"), Getenv: func(string) string { return "" }}
	if code := Run([]string{"join", "http://tailnet-mac:8787/"}, env); code != 0 {
		t.Fatalf("%d %s", code, errOut.String())
	}
	if got := LoadConfig(env); got.URL != "http://tailnet-mac:8787" || got.Secret != "a-long-enough-secret-1" {
		t.Fatalf("%+v", got)
	}
	for _, bad := range [][]string{{"join"}, {"join", "ftp://x"}, {"join", "http://u:p@x:1"}, {"join", "http://x:1/path"}, {"join", "not a url"}} {
		var e strings.Builder
		env := Env{ConfigDir: t.TempDir(), Stdout: &out, Stderr: &e, Stdin: strings.NewReader("a-long-enough-secret-1\n"), Getenv: func(string) string { return "" }}
		if code := Run(bad, env); code == 0 {
			t.Errorf("%v was accepted", bad)
		}
	}
	short := Env{ConfigDir: t.TempDir(), Stdout: &out, Stderr: &errOut, Stdin: strings.NewReader("short\n"), Getenv: func(string) string { return "" }}
	if Run([]string{"join", "http://x:1"}, short) == 0 {
		t.Fatal("a short secret was accepted")
	}
}
