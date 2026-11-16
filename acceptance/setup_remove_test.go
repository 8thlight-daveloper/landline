package acceptance

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type harnessRig struct {
	bin  string
	env  []string
	home string
}

func newHarnessRig(t *testing.T) harnessRig {
	t.Helper()
	home := t.TempDir()
	fakes := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		script := "#!/bin/sh\n" +
			"printf '%s\\n' \"$@\" > \"$HOME/" + name + "-args\"\n" +
			"code=0\n" +
			"if [ -f \"$HOME/" + name + "-exit\" ]; then read code < \"$HOME/" + name + "-exit\"; fi\n" +
			"exit \"$code\"\n"
		if err := os.WriteFile(filepath.Join(fakes, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return harnessRig{bin: build(t), env: withEnv(isolatedEnv(home), pathWith(fakes)), home: home}
}

func (r harnessRig) run(t *testing.T, args ...string) binResult {
	t.Helper()
	return runBin(t, r.bin, r.env, args...)
}

func (r harnessRig) skillDir() string { return filepath.Join(r.home, ".claude", "skills", "landline") }

func (r harnessRig) skillPath() string { return filepath.Join(r.skillDir(), "SKILL.md") }

func (r harnessRig) argsOf(name string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(r.home, name+"-args"))
	return string(raw), err == nil
}

func (r harnessRig) forgetArgs(t *testing.T, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(r.home, name+"-args")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func (r harnessRig) failWith(t *testing.T, name, code string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.home, name+"-exit"), []byte(code+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (r harnessRig) installSkill(t *testing.T) []byte {
	t.Helper()
	if result := r.run(t, "setup", "claude"); result.code != 0 {
		t.Fatalf("setup claude: %+v", result)
	}
	skill, err := os.ReadFile(r.skillPath())
	if err != nil {
		t.Fatal(err)
	}
	r.forgetArgs(t, "claude")
	return skill
}

func TestSetupRemoveRunsTheHarnessesOwnRemoveCommandAndNothingElse(t *testing.T) {
	for harness, want := range map[string]string{
		"claude": "mcp\nremove\n--scope\nuser\nlandline\n",
		"codex":  "mcp\nremove\nlandline\n",
	} {
		rig := newHarnessRig(t)
		result := rig.run(t, "setup", harness, "--remove")
		if result.code != 0 {
			t.Errorf("%s: %+v", harness, result)
		}
		if args, ran := rig.argsOf(harness); !ran || args != want {
			t.Errorf("%s ran with %q, want %q", harness, args, want)
		}
		other := map[string]string{"claude": "codex", "codex": "claude"}[harness]
		if args, ran := rig.argsOf(other); ran {
			t.Errorf("%s --remove also ran %s: %q", harness, other, args)
		}
	}
}

func TestSetupRemoveExitsWithTheStatusOfTheHarnessCommand(t *testing.T) {
	for _, harness := range []string{"claude", "codex"} {
		for _, code := range []string{"1", "3", "42"} {
			rig := newHarnessRig(t)
			rig.failWith(t, harness, code)
			result := rig.run(t, "setup", harness, "--remove")
			if want, _ := strconv.Atoi(code); want != result.code {
				t.Errorf("%s with a fake that exits %s: landline exited %d (%+v)", harness, code, result.code, result)
			}
		}
	}
}

func TestSetupClaudeRemoveDeletesTheSkillFileThatSetupInstalledAndTheEmptyFolderAndKeepsTheSkillsFolder(t *testing.T) {
	rig := newHarnessRig(t)
	rig.installSkill(t)
	result := rig.run(t, "setup", "claude", "--remove")
	if result.code != 0 {
		t.Fatalf("%+v", result)
	}
	if _, err := os.Stat(rig.skillPath()); !os.IsNotExist(err) {
		t.Errorf("the skill file stays: %v", err)
	}
	if _, err := os.Stat(rig.skillDir()); !os.IsNotExist(err) {
		t.Errorf("the empty landline folder stays: %v", err)
	}
	if info, err := os.Stat(filepath.Dir(rig.skillDir())); err != nil || !info.IsDir() {
		t.Errorf("the skills folder was removed with it: %v", err)
	}
	if info, err := os.Stat(filepath.Join(rig.home, ".claude")); err != nil || !info.IsDir() {
		t.Errorf("the .claude folder was removed with it: %v", err)
	}
}

func TestSetupClaudeRemoveKeepsTheLandlineFolderWhenSomethingElseIsInIt(t *testing.T) {
	rig := newHarnessRig(t)
	rig.installSkill(t)
	notes := filepath.Join(rig.skillDir(), "notes.txt")
	if err := os.WriteFile(notes, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := rig.run(t, "setup", "claude", "--remove")
	if result.code != 0 {
		t.Fatalf("%+v", result)
	}
	if _, err := os.Stat(rig.skillPath()); !os.IsNotExist(err) {
		t.Errorf("the unchanged skill file stays: %v", err)
	}
	if kept, err := os.ReadFile(notes); err != nil || string(kept) != "mine" {
		t.Errorf("the other file did not survive: %v %q", err, kept)
	}
}

func TestSetupClaudeRemoveKeepsAnySkillFileThatDiffersEvenByOneByteSaysSoOnStandardOutputAndExitsZero(t *testing.T) {
	original := newHarnessRig(t).installSkill(t)
	edits := map[string][]byte{
		"an appended line":       append(append([]byte{}, original...), []byte("\nmy own note\n")...),
		"one byte missing":       original[:len(original)-1],
		"a different first byte": append([]byte("#"), original[1:]...),
		"an empty file":          {},
		"a prefix line":          append([]byte("# mine\n"), original...),
	}
	for name, edited := range edits {
		rig := newHarnessRig(t)
		rig.installSkill(t)
		if err := os.WriteFile(rig.skillPath(), edited, 0o644); err != nil {
			t.Fatal(err)
		}
		result := rig.run(t, "setup", "claude", "--remove")
		if result.code != 0 {
			t.Errorf("%s: %+v", name, result)
		}
		if args, ran := rig.argsOf("claude"); !ran || args != "mcp\nremove\n--scope\nuser\nlandline\n" {
			t.Errorf("%s: the registration was not removed: %q", name, args)
		}
		kept, err := os.ReadFile(rig.skillPath())
		if err != nil || string(kept) != string(edited) {
			t.Errorf("%s: the edited skill did not survive unchanged: %v", name, err)
		}
		if strings.TrimSpace(result.stdout) == "" || !strings.Contains(result.stdout, "SKILL.md") {
			t.Errorf("%s: standard output does not say that the file stays and name SKILL.md: %q", name, result.stdout)
		}
	}
}

func TestSetupClaudeRemoveWithNoSkillFileExitsZeroAndCreatesNothing(t *testing.T) {
	rig := newHarnessRig(t)
	result := rig.run(t, "setup", "claude", "--remove")
	if result.code != 0 {
		t.Fatalf("%+v", result)
	}
	if _, err := os.Stat(filepath.Join(rig.home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("remove created %s: %v", filepath.Join(rig.home, ".claude"), err)
	}
	if strings.Contains(result.stdout, rig.skillPath()) {
		t.Errorf("remove names a skill file that was never there: %q", result.stdout)
	}
}

func TestSetupCodexRemoveLeavesTheClaudeSkillAlone(t *testing.T) {
	rig := newHarnessRig(t)
	skill := rig.installSkill(t)
	result := rig.run(t, "setup", "codex", "--remove")
	if result.code != 0 {
		t.Fatalf("%+v", result)
	}
	if kept, err := os.ReadFile(rig.skillPath()); err != nil || string(kept) != string(skill) {
		t.Errorf("codex --remove touched the Claude Code skill: %v", err)
	}
}

func TestSetupRemovePrintShowsTheHarnessCommandInEitherArgumentOrderAndChangesNothing(t *testing.T) {
	for harness, want := range map[string]string{
		"claude": "claude mcp remove --scope user landline\n",
		"codex":  "codex mcp remove landline\n",
	} {
		for _, args := range [][]string{{"setup", harness, "--remove", "--print"}, {"setup", harness, "--print", "--remove"}} {
			rig := newHarnessRig(t)
			skill := rig.installSkill(t)
			result := rig.run(t, args...)
			if result.code != 0 || result.stdout != want {
				t.Errorf("%v: %+v, want stdout %q", args, result, want)
			}
			if args, ran := rig.argsOf(harness); ran {
				t.Errorf("%v ran the harness: %q", args, args)
			}
			if kept, err := os.ReadFile(rig.skillPath()); err != nil || string(kept) != string(skill) {
				t.Errorf("%v changed the skill: %v", args, err)
			}
		}
	}
}

func TestSetupRemoveRefusesAnyOtherArgumentAndAnUnknownHarnessWithExitTwoAndTheUsageAndChangesNothing(t *testing.T) {
	for name, args := range map[string][]string{
		"an extra word":       {"setup", "claude", "--remove", "extra"},
		"an extra flag":       {"setup", "claude", "--remove", "--force"},
		"a second harness":    {"setup", "claude", "--remove", "codex"},
		"a codex extra flag":  {"setup", "codex", "--remove", "--all"},
		"a flag with a value": {"setup", "codex", "--remove=yes"},
		"print and an extra":  {"setup", "claude", "--remove", "--print", "extra"},
		"no harness":          {"setup", "--remove"},
		"an unknown harness":  {"setup", "gemini", "--remove"},
		"a misspelt flag":     {"setup", "claude", "--remov"},
	} {
		rig := newHarnessRig(t)
		skill := rig.installSkill(t)
		result := rig.run(t, args...)
		if result.code != 2 || result.stdout != "" {
			t.Errorf("%s %v: %+v", name, args, result)
		}
		for _, want := range []string{"claude", "codex"} {
			if !strings.Contains(result.stderr, want) {
				t.Errorf("%s %v: standard error does not name %s: %q", name, args, want, result.stderr)
			}
		}
		for _, harness := range []string{"claude", "codex"} {
			if args, ran := rig.argsOf(harness); ran {
				t.Errorf("%s: %s ran: %q", name, harness, args)
			}
		}
		if kept, err := os.ReadFile(rig.skillPath()); err != nil || string(kept) != string(skill) {
			t.Errorf("%s: the skill changed: %v", name, err)
		}
	}
	rig := newHarnessRig(t)
	result := rig.run(t, "setup", "claude", "--remove", "extra")
	if !strings.Contains(result.stderr, "--remove") || !strings.Contains(result.stderr, "usage: landline setup claude|codex") {
		t.Errorf("the usage does not show the flag: %q", result.stderr)
	}
}

func TestSetupUsageNamesRemoveAndSetupWithoutItStillAddsAndInstallsTheSkill(t *testing.T) {
	rig := newHarnessRig(t)
	result := rig.run(t, "setup", "claude")
	if result.code != 0 {
		t.Fatalf("%+v", result)
	}
	if args, ran := rig.argsOf("claude"); !ran || !strings.HasPrefix(args, "mcp\nadd\n--scope\nuser\nlandline\n") {
		t.Errorf("setup claude ran %q", args)
	}
	if _, err := os.Stat(rig.skillPath()); err != nil {
		t.Errorf("setup claude did not install the skill: %v", err)
	}
	for _, args := range [][]string{{"help"}, {"-h"}} {
		help := rig.run(t, args...)
		if help.code != 0 || !strings.Contains(help.stdout, "--remove") {
			t.Errorf("%v does not mention --remove in the usage: %+v", args, help)
		}
	}
}

func TestSetupClaudeRemoveDeletesTheUnchangedSkillEvenWhenTheHarnessCommandFailsAndStillExitsWithItsStatus(t *testing.T) {
	rig := newHarnessRig(t)
	rig.installSkill(t)
	rig.failWith(t, "claude", "3")
	result := rig.run(t, "setup", "claude", "--remove")
	if result.code != 3 {
		t.Errorf("exit %d, want the status 3 of the harness command: %+v", result.code, result)
	}
	if _, err := os.Stat(rig.skillDir()); !os.IsNotExist(err) {
		t.Errorf("the stale skill was not cleaned after a failed claude mcp remove: %v", err)
	}
}

func TestSetupClaudeRemoveKeepsAnEditedSkillWhenTheHarnessCommandFails(t *testing.T) {
	rig := newHarnessRig(t)
	rig.installSkill(t)
	edited := []byte("my own skill\n")
	if err := os.WriteFile(rig.skillPath(), edited, 0o644); err != nil {
		t.Fatal(err)
	}
	rig.failWith(t, "claude", "1")
	result := rig.run(t, "setup", "claude", "--remove")
	if result.code != 1 {
		t.Errorf("%+v", result)
	}
	if kept, err := os.ReadFile(rig.skillPath()); err != nil || string(kept) != string(edited) {
		t.Errorf("the edited skill did not survive: %v", err)
	}
}

func TestSetupRemoveWithTheHarnessNotOnThePathExitsOneNamingItAndClaudeStillCleansTheSkill(t *testing.T) {
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		skill := rig.installSkill(t)
		rig.env = withEnv(rig.env, pathWith(t.TempDir()))
		result := rig.run(t, "setup", harness, "--remove")
		if result.code != 1 || !strings.Contains(result.stderr, harness) {
			t.Errorf("%s: %+v", harness, result)
		}
		_, err := os.ReadFile(rig.skillPath())
		switch harness {
		case "claude":
			if !os.IsNotExist(err) {
				t.Errorf("claude: the skill stays although the harness is missing: %v", err)
			}
		case "codex":
			if kept, err := os.ReadFile(rig.skillPath()); err != nil || string(kept) != string(skill) {
				t.Errorf("codex --remove touched the Claude Code skill: %v", err)
			}
		}
	}
}
