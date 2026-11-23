package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const longSecret = "0123456789abcdef"

func envFor(dir string, values map[string]string) Env {
	return Env{ConfigDir: dir, Getenv: func(key string) string { return values[key] }}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTheSavedConfigIsPrivateAndReadBack(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "a", "b")
	env := Env{ConfigDir: dir}
	if err := SaveConfig(env, Config{Host: "tailnet-mac", Port: 9, Secret: longSecret}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "config.toml"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", info, err)
	}
	if dirInfo, _ := os.Stat(dir); dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("folder %v", dirInfo.Mode().Perm())
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if string(raw) != "secret = \""+longSecret+"\"\nhost = \"tailnet-mac\"\nport = 9\n" {
		t.Fatalf("%q", raw)
	}
	if got, err := ReadConfig(env); err != nil || got != (Config{Host: "tailnet-mac", Port: 9, Secret: longSecret}) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestSavingOverALooseConfigMakesItPrivateAndKeepsTheOtherLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "config.toml", "# mine\nsecret = \"old\"\nextra = 5\n")
	writeFile(t, dir, "config.toml.tmp", "x")
	os.Chmod(filepath.Join(dir, "config.toml.tmp"), 0o666)
	if err := SaveConfig(Env{ConfigDir: dir}, Config{Secret: longSecret, Host: "h"}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(dir, "config.toml"))
	raw, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if info.Mode().Perm() != 0o600 || string(raw) != "# mine\nsecret = \""+longSecret+"\"\nextra = 5\nhost = \"h\"\n" {
		t.Fatalf("%v %q", info.Mode().Perm(), raw)
	}
}

func TestSavingOnlyOneKeyLeavesTheOthersAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := Env{ConfigDir: dir}
	SaveConfig(env, Config{Host: "tailnet-mac", Port: 9, Secret: longSecret})
	SaveConfig(env, Config{Port: 10})
	if got, _ := ReadConfig(env); got != (Config{Host: "tailnet-mac", Port: 10, Secret: longSecret}) {
		t.Fatalf("%+v", got)
	}
}

func TestAnOldConfigJSONIsReadWhenThereIsNoTomlAndTheFirstSaveReplacesIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "config.json", `{"url":"http://tailnet-mac:8787/","secret":"`+longSecret+`"}`)
	env := Env{ConfigDir: dir}
	if got, err := ReadConfig(env); err != nil || got != (Config{Host: "tailnet-mac", Port: 8787, Secret: longSecret}) {
		t.Fatalf("%+v %v", got, err)
	}
	if err := SaveConfig(env, Config{Port: 9}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
		t.Fatal("config.json stayed")
	}
	if got, _ := ReadConfig(env); got != (Config{Host: "tailnet-mac", Port: 9, Secret: longSecret}) {
		t.Fatalf("%+v", got)
	}
}

func TestConfigTomlWinsOverConfigJSONAndAnHTTPSURLIsAnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "config.json", `{"url":"http://old:1","secret":"`+longSecret+`"}`)
	writeFile(t, dir, "config.toml", "host = \"new\"\n")
	if got, _ := ReadConfig(Env{ConfigDir: dir}); got != (Config{Host: "new"}) {
		t.Fatalf("%+v", got)
	}
	other := t.TempDir()
	writeFile(t, other, "config.json", `{"url":"https://old:1","secret":"`+longSecret+`"}`)
	if _, err := ReadConfig(Env{ConfigDir: other}); err == nil || !strings.Contains(err.Error(), "http only") || !strings.Contains(err.Error(), "config.json") {
		t.Fatalf("%v", err)
	}
}

func TestTheReaderAcceptsBlankLinesCommentsAndUnknownKeysAndRefusesEverythingElseAtItsLine(t *testing.T) {
	t.Parallel()
	good := "# note\n\nextra = \"x\"\nother = 5\nsecret = \"" + longSecret + "\"\n  host = \"h\"  \nport = 8787\n"
	if got, err := parseConfig("f", good); err != nil || got != (Config{Host: "h", Port: 8787, Secret: longSecret}) {
		t.Fatalf("%+v %v", got, err)
	}
	for text, line := range map[string]string{
		"host = h":                "line 1",
		"host = 5":                "line 1",
		"secret = 5":              "line 1",
		"port = \"8787\"":         "line 1",
		"port = 0":                "line 1",
		"port = 65536":            "line 1",
		"port = 87.87":            "line 1",
		"port = 8787 # note":      "line 1",
		"host = 'h'":              "line 1",
		"[a.b]":                   "line 1",
		"list = [1, 2]":           "line 1",
		"just words":              "line 1",
		"# ok\n\nhost = \"h\"\n!": "line 4",
	} {
		_, err := parseConfig("/x/config.toml", text)
		if err == nil || !strings.Contains(err.Error(), "/x/config.toml line "+line[len("line "):]) {
			t.Errorf("%q: %v", text, err)
			continue
		}
		for _, echoed := range strings.Fields(strings.ReplaceAll(text, "\n", " ")) {
			if len(echoed) > 3 && echoed != "port" && echoed != "host" && echoed != "secret" && strings.Contains(err.Error(), echoed) {
				t.Errorf("%q: the error repeats %q: %v", text, echoed, err)
			}
		}
	}
}

func TestTheEnvironmentWinsOverTheSavedConfigAndTheConfigFillsTheGaps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	SaveConfig(Env{ConfigDir: dir}, Config{Host: "saved", Port: 1, Secret: "saved-secret-0123456789"})
	get := func(values map[string]string) Env { return withConfig(envFor(dir, values)) }
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

func TestAMalformedConfigIsKeptAsAProblemOfTheEnvironment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "config.toml", "secret = \"x\"\nbad line\n")
	if e := withConfig(envFor(dir, nil)); e.configProblem == nil || !strings.Contains(e.configProblem.Error(), "line 2") {
		t.Fatalf("%v", e.configProblem)
	}
}

func TestTheHubURLDefaultsToLoopbackAtTheLandlinePortAndTheSavedPortWinsOverIt(t *testing.T) {
	t.Parallel()
	withPort := func(dir, port string) Env { return envFor(dir, map[string]string{"LANDLINE_PORT": port}) }
	empty := t.TempDir()
	for port, want := range map[string]string{"9191": "http://127.0.0.1:9191", "": DefaultURL, "nope": DefaultURL} {
		if got := HubURL(withPort(empty, port)); got != want {
			t.Errorf("%q: %q", port, got)
		}
	}
	hostOnly := t.TempDir()
	SaveConfig(Env{ConfigDir: hostOnly}, Config{Host: "tailnet-mac", Secret: longSecret})
	if got := HubURL(withPort(hostOnly, "9191")); got != "http://tailnet-mac:9191" {
		t.Fatalf("a saved host with no port uses LANDLINE_PORT: %q", got)
	}
	both := t.TempDir()
	SaveConfig(Env{ConfigDir: both}, Config{Host: "tailnet-mac", Port: 8787, Secret: longSecret})
	if got := HubURL(withPort(both, "9191")); got != "http://tailnet-mac:8787" {
		t.Fatalf("a saved port must win over LANDLINE_PORT: %q", got)
	}
	v6 := t.TempDir()
	SaveConfig(Env{ConfigDir: v6}, Config{Host: "::1", Port: 8787, Secret: longSecret})
	if got := HubURL(withPort(v6, "")); got != "http://[::1]:8787" {
		t.Fatalf("an IPv6 host takes brackets: %q", got)
	}
}

func TestJoinSavesHostPortAndSecretFromEitherFormAndRefusesBadOnes(t *testing.T) {
	t.Parallel()
	for _, form := range []string{"tailnet-mac:8787", "http://tailnet-mac:8787/"} {
		dir := t.TempDir()
		var out, errOut strings.Builder
		env := Env{ConfigDir: dir, Stdout: &out, Stderr: &errOut, Stdin: strings.NewReader("a-long-enough-secret-1\n"), Getenv: func(string) string { return "" }}
		if code := Run([]string{"join", form}, env); code != 0 {
			t.Fatalf("%s: %d %s", form, code, errOut.String())
		}
		if got, _ := ReadConfig(env); got != (Config{Host: "tailnet-mac", Port: 8787, Secret: "a-long-enough-secret-1"}) {
			t.Fatalf("%s: %+v", form, got)
		}
	}
	for _, bad := range []string{"https://x:1", "ftp://x:1", "http://u:p@x:1", "http://x:1/path", "not an address", "x", "x:0", "x:99999"} {
		var e strings.Builder
		env := Env{ConfigDir: t.TempDir(), Stdout: &e, Stderr: &e, Stdin: strings.NewReader("a-long-enough-secret-1\n"), Getenv: func(string) string { return "" }}
		code := Run([]string{"join", bad}, env)
		wanted := "the switchboard address must look like host:port"
		if strings.HasPrefix(bad, "https://") {
			wanted = "http only"
		}
		if code == 0 || !strings.Contains(e.String(), wanted) {
			t.Errorf("%s: %d %q", bad, code, e.String())
		}
		if _, err := os.Stat(filepath.Join(env.ConfigDir, "config.toml")); err == nil {
			t.Errorf("%s: a file was written", bad)
		}
	}
	var usage strings.Builder
	bare := Env{ConfigDir: t.TempDir(), Stdout: &usage, Stderr: &usage, Stdin: strings.NewReader("a-long-enough-secret-1\n"), Getenv: func(string) string { return "" }}
	if code := Run([]string{"join"}, bare); code == 0 || !strings.Contains(usage.String(), "usage: landline server join <host:port> [--code <code>]. Without a code, the switchboard secret comes from") {
		t.Errorf("bare join: %d %q", code, usage.String())
	}
	for secret, wanted := range map[string]string{"short": "at least 16 characters", "a-secret-with-a-\"-in-it": "cannot hold a quote"} {
		var e strings.Builder
		env := Env{ConfigDir: t.TempDir(), Stdout: &e, Stderr: &e, Stdin: strings.NewReader(secret + "\n"), Getenv: func(string) string { return "" }}
		if Run([]string{"join", "x:1"}, env) == 0 || !strings.Contains(e.String(), wanted) {
			t.Errorf("%q: %q", secret, e.String())
		}
	}
}

func TestDefaultAddrReadsLandlinePortAndRefusesBadValues(t *testing.T) {
	t.Parallel()
	with := func(value string) func(string) string { return func(string) string { return value } }
	for value, want := range map[string]string{"": "127.0.0.1:8787", "1": "127.0.0.1:1", "65535": "127.0.0.1:65535", "9191": "127.0.0.1:9191"} {
		if got, err := DefaultAddr(with(value)); err != nil || got != want {
			t.Errorf("%q: %q %v", value, got, err)
		}
	}
	for _, bad := range []string{"0", "65536", "-1", "80a", "1.5", " 80"} {
		if _, err := DefaultAddr(with(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestJoinReplacesTheSecretAlreadySavedInTheFileWithTheOneOnStandardInput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := SaveConfig(Env{ConfigDir: dir}, Config{Host: "old.example", Port: 9, Secret: "old-secret-0123456789"}); err != nil {
		t.Fatal(err)
	}
	var out, errOut strings.Builder
	env := Env{ConfigDir: dir, Stdout: &out, Stderr: &errOut, Stdin: strings.NewReader("new-secret-0123456789\n"), Getenv: func(string) string { return "" }}
	if code := Run([]string{"join", "tailnet-mac:8787"}, env); code != 0 {
		t.Fatalf("%d %s", code, errOut.String())
	}
	if got, _ := ReadConfig(env); got != (Config{Host: "tailnet-mac", Port: 8787, Secret: "new-secret-0123456789"}) {
		t.Fatalf("%+v", got)
	}
}

func TestJoinPrefersTheSecretInTheProcessEnvironmentOverStandardInput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := Env{ConfigDir: dir, Stdout: &out, Stderr: &errOut, Stdin: strings.NewReader("stdin-secret-0123456789\n"),
		Getenv: func(key string) string {
			return map[string]string{"LANDLINE_SECRET": "environment-secret-0123456789"}[key]
		}}
	if code := Run([]string{"join", "tailnet-mac:8787"}, env); code != 0 {
		t.Fatalf("%d %s", code, errOut.String())
	}
	if got, _ := ReadConfig(env); got.Secret != "environment-secret-0123456789" {
		t.Fatalf("%+v", got)
	}
}
