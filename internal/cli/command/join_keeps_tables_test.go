package command

import (
	"os"
	"strings"
	"testing"
)

const tablesAfterTopLevel = "[server]\n" +
	"# where the log goes\n" +
	"log = \"~/landline-logs/log.jsonl\"\n" +
	"\n" +
	"[extra]\n" +
	"secret = 5\n" +
	"# tables this build does not know\n" +
	"note = \"kept\"\n" +
	"\n" +
	"[other]\n" +
	"secret = \"kept-as-is-in-other\"\n"

func TestASuccessfulJoinWithACodeKeepsEveryLineOfEveryTableIncludingASecretKeyInAnUnknownTable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := joinEnv(dir, &out, &errOut)
	writeFile(t, dir, "config.toml", "secret = \""+longSecret+"\"\nhost = \"old.example\"\nport = 9\n\n"+tablesAfterTopLevel)

	if code := Run([]string{"join", redeemServer(t, "linux-box"), "--code", "ABCD-EFGH"}, env); code != 0 {
		t.Fatalf("join exited %d: %s", code, errOut.String())
	}

	raw, err := os.ReadFile(configPath(env))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.HasSuffix(text, tablesAfterTopLevel) {
		t.Errorf("the join changed or removed a line inside a table. config.toml is now:\n%s", text)
	}
	topLevel, _, _ := strings.Cut(text, "[server]")
	if strings.Contains(topLevel, "secret") {
		t.Errorf("the join kept the top-level secret line:\n%s", topLevel)
	}
	if !strings.Contains(topLevel, "machine = \"linux-box\"") {
		t.Errorf("the join did not save the machine name before the first table:\n%s", topLevel)
	}
}
