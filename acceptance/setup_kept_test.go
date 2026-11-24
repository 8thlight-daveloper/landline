package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupRemoveKeepsAnEmptyFolderThePersonMadeInsideTheSkillFolder(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		rig.installedSkill(t, harness)
		dir := rig.skillDirOf(harness)
		mine := []string{filepath.Join(dir, "my-empty-folder"), filepath.Join(dir, "references", "my-drafts")}
		for _, folder := range mine {
			if err := os.MkdirAll(folder, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if result := rig.setup(t, harness, "--remove"); result.code != 0 {
			t.Fatalf("%s: %+v", harness, result)
		}
		for _, folder := range mine {
			if info, err := os.Stat(folder); err != nil || !info.IsDir() {
				t.Errorf("%s: setup --remove deleted %s, a folder the person made: %v", harness, folder, err)
			}
		}
		for _, name := range skillFiles {
			if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				t.Errorf("%s: the managed file %s stays: %v", harness, name, err)
			}
		}
	}
}

func TestSetupCleansTheTargetOfALinkItKeepsBeforeItPrintsIt(t *testing.T) {
	t.Parallel()
	hostile := "/alpha-host/notes\x1b[2J\x1b]0;owned\x07\rkept nothing\nremoved everything\tx"
	for _, harness := range []string{"claude", "codex"} {
		for _, args := range [][]string{nil, {"--remove"}} {
			rig := newHarnessRig(t)
			rig.installedSkill(t, harness)
			link := filepath.Join(rig.skillDirOf(harness), "references", "agent.md")
			if err := os.Remove(link); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(hostile, link); err != nil {
				t.Fatal(err)
			}
			result := rig.setup(t, harness, args...)
			if keptLine(result.stdout, link) == "" || !strings.Contains(result.stdout, "/alpha-host/notes") {
				t.Errorf("%s %v: the kept link is not named: %q", harness, args, result.stdout)
			}
			if strings.ContainsAny(result.stdout+result.stderr, "\x1b\x07\r\t") {
				t.Errorf("%s %v: a control character from the link target reached the terminal: %q", harness, args, result.stdout)
			}
			for _, line := range strings.Split(result.stdout, "\n") {
				if strings.HasPrefix(line, "removed everything") {
					t.Errorf("%s %v: the link target forged a line: %q", harness, args, result.stdout)
				}
			}
		}
	}
}
