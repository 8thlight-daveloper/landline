package command

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const tableFile = "# mine\n" +
	"secret = \"" + longSecret + "\"\n" +
	"host = \"tailnet-mac\"\n" +
	"port = 9\n" +
	"\n" +
	"[server]\n" +
	"# where the log goes\n" +
	"log = \"~/landline-logs/log.jsonl\"\n" +
	"keepalive = \"15s\"\n" +
	"delivery-timeout = \"20s\"\n" +
	"grace = \"2m\"\n" +
	"invite-lifetime = \"5m\"\n" +
	"signature-window = \"5s\"\n"

func lineNumberIn(err error, number string) bool {
	return err != nil && regexp.MustCompile(`config\.toml line `+number+`\b`).MatchString(err.Error())
}

func TestAServerTableIsReadAndItsKeysAreNeverTakenAsTopLevelKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "config.toml", tableFile+
		"host = \"192.0.2.1\"\n"+
		"port = 1234\n"+
		"secret = \"a-table-secret-is-never-read\"\n"+
		"machine = \"table-box\"\n")
	got, err := ReadConfig(Env{ConfigDir: dir, Home: t.TempDir()})
	if err != nil {
		t.Fatalf("a [server] table is an error: %v", err)
	}
	if got.Host != "tailnet-mac" || got.Port != 9 || got.Secret != longSecret || got.Machine != "" {
		t.Fatalf("a key of [server] was read as a top-level key: host %q port %d machine %q secret changed %v", got.Host, got.Port, got.Machine, got.Secret != longSecret)
	}
}

func TestOnlyTheKeysBeforeTheFirstTableLineAreTopLevel(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{
		"a server table first":   "[server]\nhost = \"192.0.2.1\"\nport = 1234\nsecret = \"" + longSecret + "\"\n",
		"an unknown table":       "[elsewhere]\nhost = \"192.0.2.1\"\nport = 1234\nsecret = \"" + longSecret + "\"\nmachine = \"table-box\"\n",
		"an unknown then server": "host = \"tailnet-mac\"\n[elsewhere]\nport = 1234\n[server]\nsecret = \"" + longSecret + "\"\n",
	} {
		dir := t.TempDir()
		writeFile(t, dir, "config.toml", text)
		got, err := ReadConfig(Env{ConfigDir: dir, Home: t.TempDir()})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got.Port != 0 || got.Secret != "" || got.Machine != "" || (got.Host != "" && got.Host != "tailnet-mac") {
			t.Errorf("%s: a key inside a table was read as a top-level key: host %q port %d machine %q secret set %v", name, got.Host, got.Port, got.Machine, got.Secret != "")
		}
	}
}

func TestASecondServerLineOrAKeyGivenTwiceInServerIsRefusedAtItsLineWithoutRepeatingIt(t *testing.T) {
	t.Parallel()
	top := "secret = \"" + longSecret + "\"\nhost = \"tailnet-mac\"\nport = 9\n[server]\n"
	for name, each := range map[string]struct{ text, line, echo string }{
		"a second [server] line":     {top + "keepalive = \"15s\"\n[elsewhere]\n[server]\ngrace = \"2m\"\n", "7", ""},
		"keepalive given twice":      {top + "keepalive = \"15s\"\n# again\nkeepalive = \"16s\"\n", "7", "16s"},
		"log given twice":            {top + "log = \"~/first/log.jsonl\"\nlog = \"~/zebra/log.jsonl\"\n", "6", "zebra"},
		"an unknown key given twice": {top + "colour = \"red\"\ncolour = \"zebra\"\n", "6", "zebra"},
	} {
		dir := t.TempDir()
		writeFile(t, dir, "config.toml", each.text)
		_, err := ReadConfig(Env{ConfigDir: dir, Home: t.TempDir()})
		if !lineNumberIn(err, each.line) {
			t.Errorf("%s: want an error at config.toml line %s, got %v", name, each.line, err)
			continue
		}
		if each.echo != "" && strings.Contains(err.Error(), each.echo) {
			t.Errorf("%s: the error repeats %q: %v", name, each.echo, err)
		}
	}
}

func TestATableLineMustHoldOnlyABareKeyInBracketsAndOtherLinesStayErrors(t *testing.T) {
	t.Parallel()
	top := "secret = \"" + longSecret + "\"\nhost = \"tailnet-mac\"\n"
	for bad, echo := range map[string]string{
		"[a.b]":               "a.b",
		"[[server]]":          "[[server]]",
		"[]":                  "[]",
		"[server] # note":     "# note",
		"[ser ver]":           "ser ver",
		"[server]\nlog = ~/x": "~/x",
	} {
		dir := t.TempDir()
		writeFile(t, dir, "config.toml", top+bad+"\n")
		number := "3"
		if strings.Contains(bad, "\n") {
			number = "4"
		}
		_, err := ReadConfig(Env{ConfigDir: dir, Home: t.TempDir()})
		if !lineNumberIn(err, number) {
			t.Errorf("%q: want an error at config.toml line %s, got %v", bad, number, err)
			continue
		}
		if strings.Contains(err.Error(), echo) {
			t.Errorf("%q: the error repeats %q: %v", bad, echo, err)
		}
	}
}

func TestSavingInsertsAMissingTopLevelKeyBeforeTheFirstTableLineAndKeepsTheTableAndItsComments(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	original := "# mine\nhost = \"tailnet-mac\"\n\n[server]\n# where the log goes\nlog = \"~/landline-logs/log.jsonl\"\nkeepalive = \"15s\"\n\n[elsewhere]\nport = 1234\n"
	writeFile(t, dir, "config.toml", original)
	env := Env{ConfigDir: dir, Home: t.TempDir()}
	if err := SaveConfig(env, Config{Secret: longSecret, Port: 9, Machine: "linux-box"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "config.toml"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config.toml after a save: %v %v", info, err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	firstTable := -1
	for i, line := range lines {
		if line == "[server]" {
			firstTable = i
			break
		}
	}
	if firstTable < 0 {
		t.Fatalf("the save lost the [server] line:\n%s", raw)
	}
	for _, key := range []string{"secret", "port", "machine"} {
		count := 0
		for i, line := range lines {
			if strings.HasPrefix(line, key+" ") || strings.HasPrefix(line, key+"=") {
				count++
				if i > firstTable && !(key == "port" && line == "port = 1234") {
					t.Errorf("%s was written after the first table line, at line %d:\n%s", key, i+1, raw)
				}
			}
		}
		if want := map[string]int{"secret": 1, "port": 2, "machine": 1}[key]; count != want {
			t.Errorf("the file holds %d %s lines, want %d:\n%s", count, key, want, raw)
		}
	}
	if !strings.HasSuffix(string(raw), "[server]\n# where the log goes\nlog = \"~/landline-logs/log.jsonl\"\nkeepalive = \"15s\"\n\n[elsewhere]\nport = 1234\n") {
		t.Errorf("the save changed the tables or their comments:\n%s", raw)
	}
	if !strings.HasPrefix(string(raw), "# mine\nhost = \"tailnet-mac\"\n") {
		t.Errorf("the save changed the top-level lines it did not set:\n%s", raw)
	}
	got, err := ReadConfig(env)
	if err != nil || got.Secret != longSecret || got.Port != 9 || got.Machine != "linux-box" || got.Host != "tailnet-mac" {
		t.Errorf("the saved keys are not read back as top-level keys: host %q port %d machine %q secret set %v, %v", got.Host, got.Port, got.Machine, got.Secret == longSecret, err)
	}
}

func TestSavingAKeyNeverChangesAKeyOfTheSameNameInATable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "config.toml", "[elsewhere]\nhost = \"kept-as-is\"\nsecret = \"kept-as-is\"\n")
	env := Env{ConfigDir: dir, Home: t.TempDir()}
	if err := SaveConfig(env, Config{Host: "tailnet-mac", Secret: longSecret}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if !strings.HasSuffix(string(raw), "[elsewhere]\nhost = \"kept-as-is\"\nsecret = \"kept-as-is\"\n") {
		t.Errorf("the save changed a key inside a table:\n%s", raw)
	}
	if got, err := ReadConfig(env); err != nil || got.Host != "tailnet-mac" || got.Secret != longSecret {
		t.Errorf("host %q, secret saved %v, %v", got.Host, got.Secret == longSecret, err)
	}
}
