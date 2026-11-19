package acceptance

import (
	"os"
	"path/filepath"
	"regexp"
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
			"printf '%s\\n' \"$*\" >> \"$HOME/" + name + "-calls\"\n" +
			"code=0\n" +
			"if [ -f \"$HOME/" + name + "-exit\" ]; then read code < \"$HOME/" + name + "-exit\"; fi\n" +
			"if [ -f \"$HOME/" + name + "-exit-$2\" ]; then read code < \"$HOME/" + name + "-exit-$2\"; fi\n" +
			"if [ \"$code\" != 0 ]; then echo \"$2 failed\" >&2; exit \"$code\"; fi\n" +
			"if [ \"$1 $2\" = \"mcp add\" ]; then\n" +
			"  if [ -f \"$HOME/" + name + "-registered\" ]; then echo \"MCP server landline already exists in user config\" >&2; exit 1; fi\n" +
			"  : > \"$HOME/" + name + "-registered\"\n" +
			"fi\n" +
			"if [ \"$1 $2\" = \"mcp remove\" ]; then /bin/rm -f \"$HOME/" + name + "-registered\"; fi\n" +
			"exit 0\n"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestSetupClaudeRemoveWithNoSkillFileExitsZeroAndCreatesNothing(t *testing.T) {
	t.Parallel()
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

func TestSetupRemovePrintShowsTheHarnessCommandInEitherArgumentOrderAndChangesNothing(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

var (
	managedMarker = regexp.MustCompile(`<!-- managed by landline setup, build ([^ >]+) -->`)
	skillFiles    = []string{"SKILL.md", filepath.Join("references", "register.md"), filepath.Join("references", "agent.md")}
	olderManaged  = "<!-- managed by landline setup, build 0.0.1 -->\nan older copy\n"
	olderRouter   = "---\nname: landline\ndescription: an older router\n---\n<!-- managed by landline setup, build 0.0.1 -->\nan older copy\n"
)

func (r harnessRig) skillDirOf(harness string) string {
	if harness == "codex" {
		return filepath.Join(r.home, ".codex", "skills", "landline")
	}
	return r.skillDir()
}

func (r harnessRig) setup(t *testing.T, harness string, args ...string) binResult {
	t.Helper()
	return r.run(t, append([]string{"setup", harness}, args...)...)
}

func (r harnessRig) installedSkill(t *testing.T, harness string) map[string]string {
	t.Helper()
	if result := r.setup(t, harness); result.code != 0 {
		t.Fatalf("setup %s: %+v", harness, result)
	}
	r.forgetArgs(t, harness)
	out := map[string]string{}
	for _, name := range skillFiles {
		raw, err := os.ReadFile(filepath.Join(r.skillDirOf(harness), name))
		if err != nil {
			t.Fatalf("setup %s did not install %s: %v", harness, name, err)
		}
		out[name] = string(raw)
	}
	return out
}

func buildVersion(t *testing.T, rig harnessRig) string {
	t.Helper()
	fields := strings.Fields(rig.run(t, "version").stdout)
	if len(fields) < 2 {
		t.Fatal("landline version printed no version")
	}
	return fields[1]
}

func TestSetupInstallsTheSkillAsThreeManagedFilesForEachHarnessAndSaysWhereItWent(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		version := buildVersion(t, rig)
		result := rig.setup(t, harness)
		if result.code != 0 || !strings.Contains(result.stdout, rig.skillDirOf(harness)) {
			t.Errorf("%s: %+v", harness, result)
		}
		for _, name := range skillFiles {
			raw, err := os.ReadFile(filepath.Join(rig.skillDirOf(harness), name))
			if err != nil {
				t.Errorf("%s: %s is missing: %v", harness, name, err)
				continue
			}
			marker := managedMarker.FindString(string(raw))
			if marker == "" || !strings.Contains(marker, version) {
				t.Errorf("%s: %s has no managed line that names the build %s: %q", harness, name, version, marker)
			}
			if name == "SKILL.md" {
				lines := strings.Split(string(raw), "\n")
				closing := -1
				for i, line := range lines[1:] {
					if line == "---" {
						closing = i + 1
						break
					}
				}
				if lines[0] != "---" || closing < 0 || !strings.Contains(strings.Join(lines[:closing], "\n"), "name: landline") || strings.Contains(strings.Join(lines[:closing+1], "\n"), "managed by landline setup") {
					t.Errorf("%s: SKILL.md does not start with its front matter, with the managed line after it", harness)
				}
			} else if !strings.HasPrefix(strings.Split(string(raw), "\n")[0], "<!--") || !managedMarker.MatchString(strings.Split(string(raw), "\n")[0]) {
				t.Errorf("%s: %s does not carry the managed line on its first line", harness, name)
			}
		}
	}
}

func TestSetupCodexInstallsUnderCodexHomeWhenItIsSetAndUnderTheHomeFolderWhenItIsNot(t *testing.T) {
	t.Parallel()
	rig := newHarnessRig(t)
	if result := rig.setup(t, "codex"); result.code != 0 {
		t.Fatalf("%+v", result)
	}
	if _, err := os.Stat(filepath.Join(rig.home, ".codex", "skills", "landline", "SKILL.md")); err != nil {
		t.Errorf("setup codex did not install under ~/.codex/skills: %v", err)
	}
	custom := newHarnessRig(t)
	codexHome := t.TempDir()
	custom.env = withEnv(custom.env, "CODEX_HOME="+codexHome)
	if result := custom.setup(t, "codex"); result.code != 0 {
		t.Fatalf("%+v", result)
	}
	for _, name := range skillFiles {
		if _, err := os.Stat(filepath.Join(codexHome, "skills", "landline", name)); err != nil {
			t.Errorf("CODEX_HOME was ignored for %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(custom.home, ".codex")); !os.IsNotExist(err) {
		t.Errorf("setup codex wrote under ~/.codex although CODEX_HOME was set: %v", err)
	}
}

func TestSetupForOneHarnessLeavesTheOtherHarnessesSkillAlone(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		other := map[string]string{"claude": "codex", "codex": "claude"}[harness]
		if result := rig.setup(t, harness); result.code != 0 {
			t.Fatalf("%+v", result)
		}
		if _, err := os.Stat(filepath.Join(rig.skillDirOf(harness), "references", "agent.md")); err != nil {
			t.Errorf("setup %s did not install its own skill: %v", harness, err)
		}
		if _, err := os.Stat(rig.skillDirOf(other)); !os.IsNotExist(err) {
			t.Errorf("setup %s created the %s skill folder: %v", harness, other, err)
		}
	}
}

func TestSetupTwiceChangesNoFile(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		first := rig.installedSkill(t, harness)
		second := rig.installedSkill(t, harness)
		for name, content := range first {
			if second[name] != content {
				t.Errorf("%s: running setup again changed %s", harness, name)
			}
		}
	}
}

func TestSetupReplacesAManagedFileFromAnyBuildSoAMachineIsNeverStuckOnAnOlderSkill(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		fresh := rig.installedSkill(t, harness)
		for _, name := range skillFiles {
			older := olderManaged
			if name == "SKILL.md" {
				older = olderRouter
			}
			if err := os.WriteFile(filepath.Join(rig.skillDirOf(harness), name), []byte(older), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		after := rig.installedSkill(t, harness)
		for name, content := range fresh {
			if after[name] != content {
				t.Errorf("%s: %s was not replaced by the current skill", harness, name)
			}
		}
	}
}

func TestSetupKeepsAFileThatDoesNotCarryTheManagedLineAndSaysSoAndStillInstallsTheOthers(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		mine := filepath.Join(rig.skillDirOf(harness), "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(mine), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mine, []byte("my own skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result := rig.setup(t, harness)
		if line := keptLine(result.stdout, mine); result.code != 0 || !strings.Contains(line, "not managed") {
			t.Errorf("%s: %+v", harness, result)
		}
		if kept, _ := os.ReadFile(mine); string(kept) != "my own skill\n" {
			t.Errorf("%s: a file that is not managed was changed", harness)
		}
		for _, name := range skillFiles[1:] {
			if _, err := os.Stat(filepath.Join(rig.skillDirOf(harness), name)); err != nil {
				t.Errorf("%s: %s was not installed beside the kept file: %v", harness, name, err)
			}
		}
	}
}

func linkedSkillFolder(t *testing.T, rig harnessRig, harness string) (link, target string) {
	t.Helper()
	target = t.TempDir()
	for name, content := range map[string]string{
		"SKILL.md": olderRouter,
		filepath.Join("references", "register.md"): olderManaged,
		filepath.Join("references", "agent.md"):    olderManaged,
		"notes.txt":                                "theirs notes\n",
	} {
		path := filepath.Join(target, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link = rig.skillDirOf(harness)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return link, target
}

func realSkillFolderWithLinks(t *testing.T, rig harnessRig, harness string) map[string]string {
	t.Helper()
	dir := rig.skillDirOf(harness)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	links := map[string]string{}
	linkedFile := filepath.Join(elsewhere, "agent.md")
	if err := os.WriteFile(linkedFile, []byte(olderManaged), 0o644); err != nil {
		t.Fatal(err)
	}
	linkedRouter := filepath.Join(elsewhere, "SKILL.md")
	if err := os.WriteFile(linkedRouter, []byte(olderRouter), 0o644); err != nil {
		t.Fatal(err)
	}
	linkedReferences := filepath.Join(elsewhere, "references")
	if err := os.MkdirAll(linkedReferences, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(linkedReferences, "register.md"), []byte(olderManaged), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkedRouter, filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkedReferences, filepath.Join(dir, "references")); err != nil {
		t.Fatal(err)
	}
	links[filepath.Join(dir, "SKILL.md")] = linkedRouter
	links[filepath.Join(dir, "references")] = linkedReferences
	return links
}

func TestSetupNeverWritesThroughASymlinkedSkillFolderFileOrReferencesFolderAndSaysWhatItKept(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		link, target := linkedSkillFolder(t, rig, harness)
		before := filesUnder(t, target)
		result := rig.setup(t, harness)
		if line := keptLine(result.stdout, link); result.code != 0 || !strings.Contains(line, target) || !strings.Contains(line, "link") {
			t.Errorf("%s folder link: %+v", harness, result)
		}
		after := filesUnder(t, target)
		if len(after) != len(before) {
			t.Errorf("%s: setup added files through the link: %d before, %d after", harness, len(before), len(after))
		}
		for path, content := range before {
			if after[path] != content {
				t.Errorf("%s: setup changed %s through the link", harness, path)
			}
		}
		if got, err := os.Readlink(link); err != nil || got != target {
			t.Errorf("%s: the link was replaced: %v %q", harness, err, got)
		}

		mixed := newHarnessRig(t)
		links := realSkillFolderWithLinks(t, mixed, harness)
		beforeLinked := map[string]string{}
		for linkPath, linkTarget := range links {
			for path, content := range filesUnder(t, linkTarget) {
				beforeLinked[path] = content
			}
			_ = linkPath
		}
		linkedAgent := filepath.Join(t.TempDir(), "agent.md")
		if err := os.WriteFile(linkedAgent, []byte(olderManaged), 0o644); err != nil {
			t.Fatal(err)
		}
		linkedReferences := links[filepath.Join(mixed.skillDirOf(harness), "references")]
		if err := os.Symlink(linkedAgent, filepath.Join(linkedReferences, "agent.md")); err != nil {
			t.Fatal(err)
		}
		beforeLinked[linkedAgent] = olderManaged
		mixedResult := mixed.setup(t, harness)
		if mixedResult.code != 0 {
			t.Errorf("%s mixed links: %+v", harness, mixedResult)
		}
		for linkPath, linkTarget := range links {
			if line := keptLine(mixedResult.stdout, linkPath); !strings.Contains(line, linkTarget) {
				t.Errorf("%s: no kept line names %s and its target %s: %q", harness, linkPath, linkTarget, mixedResult.stdout)
			}
		}
		afterLinked := map[string]string{}
		for _, linkTarget := range links {
			for path, content := range filesUnder(t, linkTarget) {
				afterLinked[path] = content
			}
		}
		afterLinked[linkedAgent] = readFileOrFail(t, linkedAgent)
		for path, content := range beforeLinked {
			if afterLinked[path] != content {
				t.Errorf("%s: setup changed %s through a link", harness, path)
			}
		}
		if len(afterLinked) != len(beforeLinked) {
			t.Errorf("%s: setup added files through a link", harness)
		}
		if _, err := os.Stat(filepath.Join(mixed.skillDirOf(harness), "references", "agent.md")); err != nil {
			t.Errorf("%s: the link target of agent.md vanished: %v", harness, err)
		}
	}
}

func TestSetupDoesNotCreateTheTargetOfADanglingSymlink(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		dir := rig.skillDirOf(harness)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		missing := filepath.Join(t.TempDir(), "gone", "SKILL.md")
		if err := os.Symlink(missing, filepath.Join(dir, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
		result := rig.setup(t, harness)
		if line := keptLine(result.stdout, filepath.Join(dir, "SKILL.md")); result.code != 0 || !strings.Contains(line, missing) {
			t.Errorf("%s: %+v", harness, result)
		}
		if _, err := os.Stat(missing); !os.IsNotExist(err) {
			t.Errorf("%s: setup created the target of a dangling link: %v", harness, err)
		}
	}
}

func TestSetupRemoveDeletesEveryManagedFileFromAnyBuildEvenOneTheUserEditedAndTheEmptyFolders(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		rig.installedSkill(t, harness)
		dir := rig.skillDirOf(harness)
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(olderRouter), 0o644); err != nil {
			t.Fatal(err)
		}
		agent := filepath.Join(dir, "references", "agent.md")
		raw, _ := os.ReadFile(agent)
		if err := os.WriteFile(agent, append(raw, []byte("\nmy own note\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
		result := rig.setup(t, harness, "--remove")
		if result.code != 0 {
			t.Fatalf("%s: %+v", harness, result)
		}
		for _, folder := range []string{filepath.Join(dir, "references"), dir} {
			if _, err := os.Stat(folder); !os.IsNotExist(err) {
				t.Errorf("%s: %s stays: %v", harness, folder, err)
			}
		}
		if info, err := os.Stat(filepath.Dir(dir)); err != nil || !info.IsDir() {
			t.Errorf("%s: the skills folder was removed with it: %v", harness, err)
		}
	}
}

func TestSetupRemoveKeepsEveryFileWithoutTheManagedLineAndNamesIt(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		rig.installedSkill(t, harness)
		dir := rig.skillDirOf(harness)
		mine := filepath.Join(dir, "SKILL.md")
		notes := filepath.Join(dir, "references", "notes.txt")
		if err := os.WriteFile(mine, []byte("my own skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(notes, []byte("mine\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result := rig.setup(t, harness, "--remove")
		if result.code != 0 || !strings.Contains(result.stdout, "kept") || !strings.Contains(result.stdout, mine) || !strings.Contains(result.stdout, notes) {
			t.Errorf("%s: %+v", harness, result)
		}
		for path, want := range map[string]string{mine: "my own skill\n", notes: "mine\n"} {
			if kept, err := os.ReadFile(path); err != nil || string(kept) != want {
				t.Errorf("%s: %s did not survive: %v", harness, path, err)
			}
		}
		for _, name := range skillFiles[1:] {
			if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				t.Errorf("%s: the managed file %s stays: %v", harness, name, err)
			}
		}
	}
}

func TestSetupRemoveNeverFollowsASymlinkAndNamesWhatItKept(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		link, target := linkedSkillFolder(t, rig, harness)
		before := filesUnder(t, target)
		result := rig.setup(t, harness, "--remove")
		if line := keptLine(result.stdout, link); result.code != 0 || !strings.Contains(line, target) {
			t.Errorf("%s: %+v", harness, result)
		}
		after := filesUnder(t, target)
		if len(after) != len(before) {
			t.Errorf("%s: remove deleted files through the link: %d before, %d after", harness, len(before), len(after))
		}
		for path, content := range before {
			if after[path] != content {
				t.Errorf("%s: remove changed %s through the link", harness, path)
			}
		}
		if got, err := os.Readlink(link); err != nil || got != target {
			t.Errorf("%s: the link was removed: %v %q", harness, err, got)
		}

		mixed := newHarnessRig(t)
		links := realSkillFolderWithLinks(t, mixed, harness)
		linkedAgent := filepath.Join(t.TempDir(), "agent.md")
		if err := os.WriteFile(linkedAgent, []byte(olderManaged), 0o644); err != nil {
			t.Fatal(err)
		}
		linkedReferences := links[filepath.Join(mixed.skillDirOf(harness), "references")]
		if err := os.Symlink(linkedAgent, filepath.Join(linkedReferences, "agent.md")); err != nil {
			t.Fatal(err)
		}
		beforeLinked := map[string]string{linkedAgent: olderManaged}
		for _, linkTarget := range links {
			for path, content := range filesUnder(t, linkTarget) {
				beforeLinked[path] = content
			}
		}
		if mixedResult := mixed.setup(t, harness, "--remove"); mixedResult.code != 0 {
			t.Errorf("%s mixed links: %+v", harness, mixedResult)
		}
		afterLinked := map[string]string{linkedAgent: readFileOrFail(t, linkedAgent)}
		for _, linkTarget := range links {
			for path, content := range filesUnder(t, linkTarget) {
				afterLinked[path] = content
			}
		}
		for path, content := range beforeLinked {
			if afterLinked[path] != content {
				t.Errorf("%s: remove deleted or changed %s through a link", harness, path)
			}
		}
	}
}

func TestSetupCodexRemoveUsesCodexHomeWhenItIsSet(t *testing.T) {
	t.Parallel()
	rig := newHarnessRig(t)
	codexHome := t.TempDir()
	rig.env = withEnv(rig.env, "CODEX_HOME="+codexHome)
	if result := rig.setup(t, "codex"); result.code != 0 {
		t.Fatalf("%+v", result)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "skills", "landline", "SKILL.md")); err != nil {
		t.Fatalf("the skill was not installed under CODEX_HOME: %v", err)
	}
	if result := rig.setup(t, "codex", "--remove"); result.code != 0 {
		t.Fatalf("%+v", result)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "skills", "landline")); !os.IsNotExist(err) {
		t.Errorf("setup codex --remove left the skill under CODEX_HOME: %v", err)
	}
	if info, err := os.Stat(filepath.Join(codexHome, "skills")); err != nil || !info.IsDir() {
		t.Errorf("the skills folder under CODEX_HOME was removed with it: %v", err)
	}
}

func TestSetupAndSetupRemovePrintChangeNoFileOnEitherHarness(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		for _, args := range [][]string{{"--print"}, {"--remove", "--print"}} {
			rig := newHarnessRig(t)
			result := rig.setup(t, harness, args...)
			if result.code != 0 {
				t.Errorf("%s %v: %+v", harness, args, result)
			}
			for _, folder := range []string{".claude", ".codex"} {
				if _, err := os.Stat(filepath.Join(rig.home, folder)); !os.IsNotExist(err) {
					t.Errorf("%s %v created %s: %v", harness, args, folder, err)
				}
			}
		}
	}
}

func TestSetupReplacesAndRemoveDeletesAnEarlierShippedSkillFileThatCarriesNoManagedLine(t *testing.T) {
	t.Parallel()
	legacy, err := filepath.Glob(filepath.Join("testdata", "legacy-skill", "*.md"))
	if err != nil || len(legacy) == 0 {
		t.Fatalf("no legacy fixtures: %v", err)
	}
	for _, fixture := range legacy {
		old := []byte(readFileOrFail(t, fixture))
		for _, harness := range []string{"claude", "codex"} {
			rig := newHarnessRig(t)
			fresh := rig.installedSkill(t, harness)
			router := filepath.Join(rig.skillDirOf(harness), "SKILL.md")
			if err := os.WriteFile(router, old, 0o644); err != nil {
				t.Fatal(err)
			}
			rig.setup(t, harness)
			if raw, _ := os.ReadFile(router); string(raw) != fresh["SKILL.md"] {
				t.Errorf("%s: an earlier shipped copy %s was not replaced", harness, filepath.Base(fixture))
			}
			if err := os.WriteFile(router, old, 0o644); err != nil {
				t.Fatal(err)
			}
			rig.setup(t, harness, "--remove")
			if _, err := os.Stat(rig.skillDirOf(harness)); !os.IsNotExist(err) {
				t.Errorf("%s: remove kept an earlier shipped copy %s: %v", harness, filepath.Base(fixture), err)
			}
			edited := append(append([]byte{}, old...), '\n')
			rig2 := newHarnessRig(t)
			rig2.installedSkill(t, harness)
			router2 := filepath.Join(rig2.skillDirOf(harness), "SKILL.md")
			if err := os.WriteFile(router2, edited, 0o644); err != nil {
				t.Fatal(err)
			}
			result := rig2.setup(t, harness)
			if raw, _ := os.ReadFile(router2); string(raw) != string(edited) || !strings.Contains(keptLine(result.stdout, router2), "not managed") {
				t.Errorf("%s: a copy that differs by one byte from %s was replaced", harness, filepath.Base(fixture))
			}
		}
	}
}

func TestSetupRemoveForOneHarnessRemovesOnlyThatHarnessesSkill(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		other := map[string]string{"claude": "codex", "codex": "claude"}[harness]
		rig.installedSkill(t, harness)
		kept := rig.installedSkill(t, other)
		if result := rig.setup(t, harness, "--remove"); result.code != 0 {
			t.Fatalf("%+v", result)
		}
		if _, err := os.Stat(rig.skillDirOf(harness)); !os.IsNotExist(err) {
			t.Errorf("setup %s --remove left its own skill: %v", harness, err)
		}
		for name, content := range kept {
			if raw, err := os.ReadFile(filepath.Join(rig.skillDirOf(other), name)); err != nil || string(raw) != content {
				t.Errorf("setup %s --remove touched the %s skill: %v", harness, other, err)
			}
		}
	}
}

func TestSetupRemoveDeletesTheManagedSkillEvenWhenTheHarnessCommandFailsOrIsNotOnThePathForEitherHarness(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		failing := newHarnessRig(t)
		failing.installedSkill(t, harness)
		failing.failWith(t, harness, "3")
		if result := failing.setup(t, harness, "--remove"); result.code != 3 {
			t.Errorf("%s: exit %d, want the status 3 of the harness command", harness, result.code)
		}
		if _, err := os.Stat(failing.skillDirOf(harness)); !os.IsNotExist(err) {
			t.Errorf("%s: the stale skill was not cleaned after a failed mcp remove: %v", harness, err)
		}

		missing := newHarnessRig(t)
		missing.installedSkill(t, harness)
		missing.env = withEnv(missing.env, pathWith(t.TempDir()))
		result := missing.setup(t, harness, "--remove")
		if result.code != 1 || !strings.Contains(result.stderr, harness) {
			t.Errorf("%s: %+v", harness, result)
		}
		if _, err := os.Stat(missing.skillDirOf(harness)); !os.IsNotExist(err) {
			t.Errorf("%s: the skill stays although the harness is missing: %v", harness, err)
		}
	}
}

func TestSetupRemoveWithNoSkillCreatesNothingForEitherHarness(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		result := rig.setup(t, harness, "--remove")
		if result.code != 0 {
			t.Fatalf("%s: %+v", harness, result)
		}
		for _, folder := range []string{".claude", ".codex"} {
			if _, err := os.Stat(filepath.Join(rig.home, folder)); !os.IsNotExist(err) {
				t.Errorf("%s --remove created %s: %v", harness, folder, err)
			}
		}
	}
}

func TestTheInstalledSkillNamesItsSubcommandsLevelsAndWhatItRefusesAndBothHarnessesGetTheSameText(t *testing.T) {
	t.Parallel()
	rig := newHarnessRig(t)
	files := rig.installedSkill(t, "claude")
	codex := rig.installedSkill(t, "codex")
	for name, content := range files {
		if codex[name] != content {
			t.Errorf("%s differs between the Claude Code and the Codex install", name)
		}
	}
	router := strings.ToLower(strings.Join(strings.Fields(files["SKILL.md"]), " "))
	for _, want := range []string{"register [project]", "agent off", "agent full", "help", "unknown", "no action", "permission mode", "$landline", "/landline", "references/register.md", "references/agent.md"} {
		if !strings.Contains(router, want) {
			t.Errorf("SKILL.md does not mention %q", want)
		}
	}
	agent := strings.ToLower(strings.Join(strings.Fields(files[filepath.Join("references", "agent.md")]), " "))
	for _, want := range []string{"read", "work", "full", "exact", "registry", "explicit", "not the user"} {
		if !strings.Contains(agent, want) {
			t.Errorf("references/agent.md does not mention %q", want)
		}
	}
	register := strings.Join(strings.Fields(files[filepath.Join("references", "register.md")]), " ")
	for _, want := range []string{"landline_register", "landline_status", "CODEX_THREAD_ID"} {
		if !strings.Contains(register, want) {
			t.Errorf("references/register.md does not mention %q", want)
		}
	}
}

func keptLine(stdout, path string) string {
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "kept") && strings.Contains(line, path) {
			return line
		}
	}
	return ""
}

func (r harnessRig) callsOf(name string) []string {
	raw, err := os.ReadFile(filepath.Join(r.home, name+"-calls"))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func (r harnessRig) failVerb(t *testing.T, name, verb, code string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.home, name+"-exit-"+verb), []byte(code+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSetupReplacesAnExistingRegistrationSoRunningItTwiceSucceedsAndEachRunEndsWithTheAdd(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		for run := 1; run <= 2; run++ {
			if result := rig.setup(t, harness); result.code != 0 {
				t.Fatalf("%s run %d: %+v", harness, run, result)
			}
		}
		calls := rig.callsOf(harness)
		if len(calls) != 4 || !strings.HasPrefix(calls[0], "mcp remove") || !strings.HasPrefix(calls[1], "mcp add") || !strings.HasPrefix(calls[2], "mcp remove") || !strings.HasPrefix(calls[3], "mcp add") {
			t.Errorf("%s ran %q, want a remove and then an add on each run", harness, calls)
		}
		if _, err := os.Stat(filepath.Join(rig.skillDirOf(harness), "SKILL.md")); err != nil {
			t.Errorf("%s: the skill was not installed: %v", harness, err)
		}
	}
}

func TestSetupIgnoresAFailingRemoveAndItsOutputAndStillAddsAndInstallsTheSkill(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		rig.failVerb(t, harness, "remove", "1")
		result := rig.setup(t, harness)
		if result.code != 0 || strings.Contains(result.stderr, "remove failed") || strings.Contains(result.stdout, "remove failed") {
			t.Errorf("%s: %+v", harness, result)
		}
		calls := rig.callsOf(harness)
		if len(calls) != 2 || !strings.HasPrefix(calls[1], "mcp add") {
			t.Errorf("%s ran %q, want a remove and then an add", harness, calls)
		}
		if _, err := os.Stat(filepath.Join(rig.skillDirOf(harness), "SKILL.md")); err != nil {
			t.Errorf("%s: the skill was not installed after a failed remove: %v", harness, err)
		}
	}
}

func TestSetupWithAFailingAddExitsOneAndInstallsNoSkill(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		rig.failVerb(t, harness, "add", "1")
		if result := rig.setup(t, harness); result.code != 1 {
			t.Errorf("%s: %+v", harness, result)
		}
		if _, err := os.Stat(rig.skillDirOf(harness)); !os.IsNotExist(err) {
			t.Errorf("%s: a skill was installed although the add failed: %v", harness, err)
		}
	}
}

func TestSetupPrintShowsTheAddCommandOnlyAndRunsNoHarnessCommand(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex"} {
		rig := newHarnessRig(t)
		result := rig.setup(t, harness, "--print")
		lines := strings.Split(strings.TrimSpace(result.stdout), "\n")
		if result.code != 0 || len(lines) != 1 || !strings.HasPrefix(lines[0], harness+" mcp add") || strings.Contains(result.stdout, "remove") {
			t.Errorf("%s: %+v", harness, result)
		}
		if calls := rig.callsOf(harness); len(calls) != 0 {
			t.Errorf("%s --print ran the harness: %q", harness, calls)
		}
	}
}
