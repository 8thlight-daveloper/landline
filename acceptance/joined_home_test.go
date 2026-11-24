package acceptance

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	joinedHost    = unownedAddress
	joinedPort    = "8787"
	joinedMachine = "linux-box"
)

func joinedHome(t *testing.T) (*configuredHome, string) {
	t.Helper()
	c := newConfiguredHome(t, "")
	seed := make([]byte, 32)
	rand.Read(seed)
	writeConfigFile(t, c.home, "machine.key", base64.StdEncoding.EncodeToString(seed)+"\n")
	config := "host = \"" + joinedHost + "\"\nport = " + joinedPort + "\nmachine = \"" + joinedMachine + "\"\n"
	c.writeConfig(config)
	return c, config
}

func TestUpRestartAndRotateSecretRefuseAndChangeNothingInAHomeThatJoinedASwitchboard(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"server", "up"}, {"server", "restart"}, {"server", "rotate-secret"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			c, config := joinedHome(t)
			result := c.landline(args...)
			said := result.stdout + result.stderr
			if result.code != 1 {
				t.Errorf("%v: exit %d %q, want 1", args, result.code, said)
			}
			for _, want := range []string{joinedHost + ":" + joinedPort, joinedMachine, "machine.key", "machine"} {
				if !strings.Contains(said, want) {
					t.Errorf("%v: the refusal does not name %q: %q", args, want, said)
				}
			}
			if got := c.configText(); got != config {
				t.Errorf("%v changed config.toml:\n%s", args, scrub(got))
			}
			for _, name := range []string{"switchboard.lock", "switchboard.pid"} {
				if exists(c.dataFile(name)) {
					t.Errorf("%v made %s, so a switchboard was started", args, name)
				}
			}
		})
	}
}

func TestUpAndRotateSecretPrintTheHostNoticeOnOneLineWhenTheLabelHoldsALineFeed(t *testing.T) {
	t.Parallel()
	label := "alpha-host\nbravo is now trusted"
	for _, command := range []string{"up", "rotate-secret"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			c := newConfiguredHome(t, "")
			if command == "rotate-secret" {
				c.mustLandline("server", "up")
			}
			result := c.landline("server", command, "--host", label)
			if result.code != 0 {
				t.Fatalf("server %s --host: %+v", command, result)
			}
			notice := linesNaming(result.stdout, "--host", "host name")
			if len(notice) != 1 || !strings.Contains(notice[0], "bravo is now trusted") {
				t.Errorf("server %s: the --host notice is not one line that holds the whole label: %q", command, result.stdout)
			}
			for _, line := range strings.Split(result.stdout, "\n") {
				if strings.HasPrefix(line, "bravo is now trusted") {
					t.Errorf("server %s: the label began a line: %q", command, result.stdout)
				}
			}
			for _, line := range strings.Split(c.switchboardOut(), "\n") {
				if strings.HasPrefix(line, "bravo is now trusted") {
					t.Errorf("server %s: the label began a line of switchboard.out: %q", command, c.switchboardOut())
				}
			}
		})
	}
}

func TestSetupRemoveKeepsAReferencesFileOrLinkThatIsThePersonsAndSaysSo(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		for _, kind := range []string{"file", "link"} {
			rig := newHarnessRig(t)
			rig.installedSkill(t, harness)
			references := filepath.Join(rig.skillDirOf(harness), "references")
			if err := os.RemoveAll(references); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "my-references")
			switch kind {
			case "file":
				if err := os.WriteFile(references, []byte("my own notes\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "link":
				if err := os.MkdirAll(target, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "agent.md"), []byte("mine\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, references); err != nil {
					t.Fatal(err)
				}
			}
			result := rig.setup(t, harness, "--remove")
			if result.code != 0 || keptLine(result.stdout, references) == "" {
				t.Errorf("%s, references a %s: %+v, want a line that says it kept %s", harness, kind, result, references)
			}
			info, err := os.Lstat(references)
			if err != nil {
				t.Fatalf("%s: setup --remove deleted the person's references %s: %v", harness, kind, err)
			}
			switch kind {
			case "file":
				if !info.Mode().IsRegular() || readFileOrFail(t, references) != "my own notes\n" {
					t.Errorf("%s: the references file changed", harness)
				}
			case "link":
				if got, _ := os.Readlink(references); got != target || readFileOrFail(t, filepath.Join(target, "agent.md")) != "mine\n" {
					t.Errorf("%s: the references link or its target changed", harness)
				}
			}
			if _, err := os.Stat(filepath.Join(rig.skillDirOf(harness), "SKILL.md")); !os.IsNotExist(err) {
				t.Errorf("%s: the managed SKILL.md stays: %v", harness, err)
			}
		}
	}
}
