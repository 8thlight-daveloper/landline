package hubclient

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const tableSecret = "saved-secret-0000000"

func readTOML(t *testing.T, text string) (saved, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return readSaved(dir)
}

func TestAServerTableIsSkippedAndItsKeysAreNeverTakenAsTopLevelKeys(t *testing.T) {
	got, err := readTOML(t, "# mine\nsecret = \""+tableSecret+"\"\nhost = \"saved\"\nport = 1\n\n[server]\n# where the log goes\n"+
		"log = \"~/landline-logs/log.jsonl\"\nkeepalive = \"15s\"\ndelivery-timeout = \"20s\"\ngrace = \"2m\"\ninvite-lifetime = \"5m\"\nsignature-window = \"5s\"\n"+
		"host = \"192.0.2.1\"\nport = 1234\nsecret = \"a-table-secret-is-never-read\"\nmachine = \"table-box\"\n")
	if err != nil {
		t.Fatalf("a [server] table is an error: %v", err)
	}
	if got != (saved{Secret: tableSecret, Host: "saved", Port: 1}) {
		t.Fatalf("a key of [server] was read as a top-level key: host %q port %d machine %q", got.Host, got.Port, got.Machine)
	}
}

func TestTheScreenAndTheMenuBarSkipEveryServerKeyWhateverItsValue(t *testing.T) {
	for _, line := range []string{`log = "relative/log.jsonl"`, `keepalive = "0s"`, `grace = "soon"`, `invite-lifetime = 300`, `colour = "red"`} {
		got, err := readTOML(t, "host = \"saved\"\nport = 1\n[server]\n"+line+"\n")
		if err != nil || got != (saved{Host: "saved", Port: 1}) {
			t.Errorf("%q in [server]: %+v %v", line, got, err)
		}
	}
}

func TestOnlyTheKeysBeforeTheFirstTableLineAreTopLevel(t *testing.T) {
	for name, text := range map[string]string{
		"a server table first":   "[server]\nhost = \"192.0.2.1\"\nport = 1234\nsecret = \"" + tableSecret + "\"\n",
		"an unknown table":       "[elsewhere]\nhost = \"192.0.2.1\"\nport = 1234\nsecret = \"" + tableSecret + "\"\nmachine = \"table-box\"\n",
		"an unknown then server": "host = \"saved\"\n[elsewhere]\nport = 1234\n[server]\nsecret = \"" + tableSecret + "\"\n",
	} {
		got, err := readTOML(t, text)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got.Port != 0 || got.Secret != "" || got.Machine != "" || (got.Host != "" && got.Host != "saved") {
			t.Errorf("%s: a key inside a table was read as a top-level key: %+v", name, saved{Host: got.Host, Port: got.Port, Machine: got.Machine})
		}
	}
}

func TestASecondServerLineAKeyGivenTwiceInServerOrABadTableLineIsRefusedAtItsLineWithoutRepeatingIt(t *testing.T) {
	top := "secret = \"" + tableSecret + "\"\nhost = \"saved\"\nport = 1\n[server]\n"
	for name, each := range map[string]struct{ text, line, echo string }{
		"a second [server] line":     {top + "keepalive = \"15s\"\n[elsewhere]\n[server]\ngrace = \"2m\"\n", "7", ""},
		"keepalive given twice":      {top + "keepalive = \"15s\"\n# again\nkeepalive = \"16s\"\n", "7", "16s"},
		"an unknown key given twice": {top + "colour = \"red\"\ncolour = \"zebra\"\n", "6", "zebra"},
		"a dotted table name":        {top + "[a.b]\n", "5", "a.b"},
		"an array of tables":         {top + "[[server]]\n", "5", "[[server]]"},
		"a table with a comment":     {top + "keepalive = \"15s\"\n[elsewhere] # note\n", "6", "# note"},
		"a key line with no quotes":  {top + "log = ~/zebra\n", "5", "zebra"},
	} {
		_, err := readTOML(t, each.text)
		if err == nil || !regexp.MustCompile(`config\.toml line `+each.line+`\b`).MatchString(err.Error()) {
			t.Errorf("%s: want an error at config.toml line %s, got %v", name, each.line, err)
			continue
		}
		if each.echo != "" && strings.Contains(err.Error(), each.echo) {
			t.Errorf("%s: the error repeats %q: %v", name, each.echo, err)
		}
	}
}
