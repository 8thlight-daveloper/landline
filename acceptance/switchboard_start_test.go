package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func (c *configuredHome) dataDir() string { return filepath.Dir(c.dataFile("machines.json")) }

func (c *configuredHome) writeDataFile(name, content string) string {
	c.t.Helper()
	if err := os.MkdirAll(c.dataDir(), 0o700); err != nil {
		c.t.Fatal(err)
	}
	path := c.dataFile(name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		c.t.Fatal(err)
	}
	return path
}

func (c *configuredHome) symlink(target, link string) string {
	c.t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		c.t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		c.t.Fatal(err)
	}
	return link
}

func TestASecondSwitchboardWithTheSameHomeExitsOneNamingTheLockFileWhileTheFirstRuns(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	first := c.startSwitchboard()
	other := freeAddr(t)
	second := c.switchboardExit("--addr", other, "--log", c.path("other-logs/log.jsonl"))
	if second.code != 1 || !strings.Contains(second.stderr, "switchboard.lock") {
		t.Errorf("a second switchboard: %d %q, want 1 and a message that names switchboard.lock", second.code, second.stderr)
	}
	if answers(other) {
		t.Errorf("the second switchboard listens at %s", other)
	}
	info, err := os.Stat(c.dataFile("switchboard.lock"))
	if err != nil {
		t.Fatalf("the running switchboard made no lock file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the lock file has mode %o, want 600", info.Mode().Perm())
	}
	first.stop()
	if !exists(c.dataFile("switchboard.lock")) {
		t.Errorf("the lock file was deleted when the switchboard was killed")
	}
	c.startSwitchboard()
}

func TestTheLockFileStaysAfterTheSwitchboardStopsAndALeftLockFileLetsTheNextOneStart(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.mustLandline("server", "up")
	c.mustLandline("server", "down")
	if !exists(c.dataFile("switchboard.lock")) {
		t.Fatalf("no lock file is left after server down")
	}
	c.mustLandline("server", "up")
	if !answers(c.addr) {
		t.Errorf("the switchboard did not start over the lock file a stopped one left")
	}
}

func TestALogPathThatIsTheMachinesFileOrTheLockFileStopsTheStartWithExitOneNamingBoth(t *testing.T) {
	t.Parallel()
	for name, logOf := range map[string]func(c *configuredHome) (string, string){
		"machines.json itself": func(c *configuredHome) (string, string) {
			return c.dataFile("machines.json"), "machines.json"
		},
		"the lock file itself": func(c *configuredHome) (string, string) {
			return c.dataFile("switchboard.lock"), "switchboard.lock"
		},
		"a link to machines.json": func(c *configuredHome) (string, string) {
			return c.symlink(c.dataFile("machines.json"), c.path("links/log.jsonl")), "machines.json"
		},
		"a link to the lock file": func(c *configuredHome) (string, string) {
			return c.symlink(c.dataFile("switchboard.lock"), c.path("links/log.jsonl")), "switchboard.lock"
		},
		"a second path through a linked folder": func(c *configuredHome) (string, string) {
			return filepath.Join(c.symlink(c.dataDir(), c.path("aliased")), "machines.json"), "machines.json"
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newConfiguredHome(t, "")
			c.writeDataFile("machines.json", "[]\n")
			c.writeDataFile("switchboard.lock", "")
			logPath, named := logOf(c)
			result := c.switchboardExit("--log", logPath)
			if result.code != 1 || !strings.Contains(result.stderr, logPath) || !strings.Contains(result.stderr, named) {
				t.Errorf("--log %s: %d %q, want 1 and a message that names the log and %s", logPath, result.code, result.stderr, named)
			}
			if got := readFileOrFail(t, c.dataFile("machines.json")); got != "[]\n" {
				t.Errorf("machines.json changed: %q", got)
			}
		})
	}
}

func TestALogFolderLinkedToTheDataDirectoryGetsNoLineAboutAnOldMachinesFile(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.writeDataFile("machines.json", "[]\n")
	logs := c.symlink(c.dataDir(), c.path("logs"))
	p := c.startSwitchboard("--log", filepath.Join(logs, "log.jsonl"))
	said := p.stderr.String()
	for _, word := range []string{"no longer read", "delete", "copied", filepath.Join(logs, "machines.json")} {
		if strings.Contains(said, word) {
			t.Errorf("the old and new machines.json are the same file, and the switchboard says %q: %q", word, said)
		}
	}
	if got := readFileOrFail(t, c.dataFile("machines.json")); got != "[]\n" {
		t.Errorf("machines.json changed: %q", got)
	}
}

func TestAnOldMachinesFileThatHoldsNoListStopsTheStartAndWritesNothing(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"null\n", "{}\n", "7\n", "\"alpha\"\n"} {
		t.Run(strings.TrimSpace(content), func(t *testing.T) {
			t.Parallel()
			c := newConfiguredHome(t, "")
			customDir := c.path("custom-logs")
			if err := os.MkdirAll(customDir, 0o700); err != nil {
				t.Fatal(err)
			}
			old := filepath.Join(customDir, "machines.json")
			if err := os.WriteFile(old, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			result := c.switchboardExit("--log", filepath.Join(customDir, "log.jsonl"))
			if result.code != 1 || !strings.Contains(result.stderr, old) {
				t.Errorf("an old machines.json of %q: %d %q, want 1 and the error that names it", content, result.code, result.stderr)
			}
			if exists(c.dataFile("machines.json")) {
				t.Errorf("an old machines.json of %q: the new file was written: %q", content, readFileOrFail(t, c.dataFile("machines.json")))
			}
			if readFileOrFail(t, old) != content {
				t.Errorf("the old file was changed")
			}
		})
	}
}

func TestEverySettingTheSwitchboardCannotReadStopsItWithExitOne(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		env  []string
		args []string
	}{
		"LANDLINE_PORT that is not a number": {[]string{"LANDLINE_PORT=alpha"}, nil},
		"LANDLINE_PORT out of range":         {[]string{"LANDLINE_PORT=70000"}, nil},
		"--signature-window not a duration":  {nil, []string{"--signature-window", "alpha"}},
		"--keepalive not a duration":         {nil, []string{"--keepalive", "alpha"}},
		"--delivery-timeout not a duration":  {nil, []string{"--delivery-timeout", "alpha"}},
		"--grace not a duration":             {nil, []string{"--grace", "alpha"}},
		"--invite-lifetime not a duration":   {nil, []string{"--invite-lifetime", "alpha"}},
	}
	for name, setting := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newConfiguredHome(t, "")
			if setting.env != nil {
				c.writeConfig("secret = \"" + secret + "\"\n")
				c.env = withEnv(c.env, setting.env...)
			}
			if result := c.switchboardExit(setting.args...); result.code != 1 {
				t.Errorf("%d %q, want 1", result.code, result.stderr)
			}
		})
	}
}

func TestALogFlagWithALiteralTildeUsesTheExpandedPathAndComparesWithTheTableAsThatPath(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "log = \"~/logs/alpha.jsonl\"\n")
	work := c.path("work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	same := c.landlineIn(work, "server", "up", "--log", "~/logs/alpha.jsonl")
	if same.code != 0 {
		t.Fatalf("up: %+v", same)
	}
	if len(flagLine(same.stdout, "--log")) != 0 {
		t.Errorf("the flag names the table's log, and up printed a differing line: %q", same.stdout)
	}
	if !exists(c.path("logs/alpha.jsonl")) {
		t.Errorf("the switchboard did not open %s", c.path("logs/alpha.jsonl"))
	}
	if exists(filepath.Join(work, "~")) || exists(filepath.Join(c.home, "~")) {
		t.Errorf("the switchboard made a folder named ~ instead of using the home directory")
	}
	c.mustLandline("server", "down")
	other := c.landlineIn(work, "server", "up", "--log", "~/logs/bravo.jsonl")
	if other.code != 0 || len(flagLine(other.stdout, "--log")) != 1 {
		t.Errorf("a flag that names another log under ~: %+v, want one differing line", other)
	}
	if !exists(c.path("logs/bravo.jsonl")) || exists(filepath.Join(work, "~")) {
		t.Errorf("the switchboard did not open %s", c.path("logs/bravo.jsonl"))
	}
}

func (c *configuredHome) landlineIn(dir string, args ...string) binResult {
	c.t.Helper()
	moved := *c
	moved.home = dir
	return moved.landline(args...)
}

func TestALogPathThatWouldBeMachinesJSONOnceMadeStopsTheStartAndMakesNoMachinesFile(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	if err := os.MkdirAll(c.dataDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	aliased := c.symlink(c.dataDir(), c.path("aliased"))
	logPath := filepath.Join(aliased, "machines.json")
	result := c.switchboardExit("--log", logPath)
	if result.code != 1 || !strings.Contains(result.stderr, logPath) || !strings.Contains(result.stderr, "machines.json") {
		t.Errorf("--log %s on a fresh install: %d %q, want 1 and a message that names the log and machines.json", logPath, result.code, result.stderr)
	}
	if exists(c.dataFile("machines.json")) {
		t.Errorf("the refused start made %s: %q", c.dataFile("machines.json"), readFileOrFail(t, c.dataFile("machines.json")))
	}
}

func TestALogPathThatNamesTheOldMachinesFileBesideACustomLogStopsTheStartAndLeavesItsBytes(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	customDir := c.path("custom-logs")
	if err := os.MkdirAll(customDir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(customDir, "machines.json")
	const oldBytes = "[]\n"
	if err := os.WriteFile(old, []byte(oldBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	result := c.switchboardExit("--log", old)
	if result.code != 1 || !strings.Contains(result.stderr, old) {
		t.Errorf("--log %s: %d %q, want 1 and a message that names it", old, result.code, result.stderr)
	}
	if got := readFileOrFail(t, old); got != oldBytes {
		t.Errorf("the old machines.json changed: %q, want %q", got, oldBytes)
	}
}

const listeningPrefix = "landline-switchboard: listening on "

func TestUpNeverReportsASwitchboardThatFailedBecauseAValueItPrintedHoldsListeningOn(t *testing.T) {
	t.Parallel()
	cases := map[string]func(c *configuredHome) []string{
		"a host label": func(c *configuredHome) []string {
			return []string{"--host", "listening on", "--log", filepath.Join(c.blocker(), "log.jsonl")}
		},
		"a log path with a line feed": func(c *configuredHome) []string {
			return []string{"--log", filepath.Join(c.blocker(), "x\n"+listeningPrefix+"127.0.0.1:1, host \"alpha-host\"", "log.jsonl")}
		},
	}
	for name, argsOf := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newConfiguredHome(t, "")
			result := c.landline(append([]string{"server", "up"}, argsOf(c)...)...)
			if result.code == 0 || strings.Contains(result.stdout, "the switchboard is running") {
				t.Errorf("up reported a switchboard that cannot open its log: %+v", result)
			}
			for _, line := range strings.Split(c.switchboardOut(), "\n") {
				if strings.HasPrefix(line, listeningPrefix) || strings.HasPrefix(line, "listening on") {
					t.Errorf("a value the person chose began a line of switchboard.out: %q", c.switchboardOut())
					break
				}
			}
		})
	}
}

func (c *configuredHome) blocker() string {
	c.t.Helper()
	path := c.path("a-file")
	if err := os.WriteFile(path, []byte("not a folder\n"), 0o600); err != nil {
		c.t.Fatal(err)
	}
	return path
}

func TestALogThatIsADanglingLinkOrACaseVariantOfTheFutureMachinesFileStopsTheStartAndLeavesNoMachinesFile(t *testing.T) {
	t.Parallel()
	cases := map[string]func(c *configuredHome) string{
		"a dangling link to machines.json": func(c *configuredHome) string {
			return c.symlink(c.dataFile("machines.json"), c.path("links/log.jsonl"))
		},
		"Machines.json on a case-insensitive disk": func(c *configuredHome) string {
			probe := c.dataFile("case-probe")
			c.writeDataFile("case-probe", "")
			if !exists(filepath.Join(c.dataDir(), "CASE-PROBE")) {
				t.Skip("the temporary disk is case-sensitive")
			}
			os.Remove(probe)
			return filepath.Join(c.dataDir(), "Machines.json")
		},
	}
	for name, logOf := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newConfiguredHome(t, "")
			if err := os.MkdirAll(c.dataDir(), 0o700); err != nil {
				t.Fatal(err)
			}
			logPath := logOf(c)
			result := c.switchboardExit("--log", logPath)
			if result.code != 1 || !strings.Contains(result.stderr, logPath) || !strings.Contains(result.stderr, "machines.json") {
				t.Errorf("--log %s: %d %q, want 1 and a message that names the log and machines.json", logPath, result.code, result.stderr)
			}
			if exists(c.dataFile("machines.json")) {
				t.Errorf("the refused start left machines.json: %q", readFileOrFail(t, c.dataFile("machines.json")))
			}
			c.startSwitchboard()
			if exists(c.dataFile("machines.json")) {
				t.Errorf("a start with no joined machine made machines.json: %q", readFileOrFail(t, c.dataFile("machines.json")))
			}
		})
	}
}

const oneOldMachine = `[{"name":"linux-box","public_key":"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=","created":"2026-11-20T00:00:00Z"}]` + "\n"

func (c *configuredHome) oldMachinesBesideACustomLog() (string, string) {
	c.t.Helper()
	customDir := c.path("custom-logs")
	if err := os.MkdirAll(customDir, 0o700); err != nil {
		c.t.Fatal(err)
	}
	old := filepath.Join(customDir, "machines.json")
	if err := os.WriteFile(old, []byte(oneOldMachine), 0o600); err != nil {
		c.t.Fatal(err)
	}
	return old, filepath.Join(customDir, "log.jsonl")
}

func checkCarriedOverWithNoDeleteAdvice(t *testing.T, c *configuredHome, said, old string) {
	t.Helper()
	if names := machineNames(t, c.dataFile("machines.json")); len(names) != 1 || !strings.HasPrefix(names[0], "linux-box ") {
		t.Errorf("the new machines.json holds %v, want the old file's linux-box carried over", names)
	}
	if strings.Contains(strings.ToLower(said), "delete") {
		t.Errorf("the switchboard advises deleting %s, which holds the only machine: %q", old, said)
	}
	if readFileOrFail(t, old) != oneOldMachine {
		t.Errorf("the old machines.json changed")
	}
}

func TestAStartWithTheDefaultLogThenACustomLogCarriesOverTheOldMachinesAndNeverAdvisesDeletingThem(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.startSwitchboard().stop()
	old, logPath := c.oldMachinesBesideACustomLog()
	p := c.startSwitchboard("--log", logPath)
	checkCarriedOverWithNoDeleteAdvice(t, c, p.stderr.String(), old)
}

func TestANewMachinesFileHoldingAnEmptyListMeansNoMachineIsJoinedAndTheOldFileIsNotRead(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.writeDataFile("machines.json", "[]\n")
	old, logPath := c.oldMachinesBesideACustomLog()
	c.startSwitchboard("--log", logPath)
	if names := machineNames(t, c.dataFile("machines.json")); len(names) != 0 {
		t.Errorf("the new machines.json held an empty list and now holds %v from the old file", names)
	}
	if readFileOrFail(t, old) != oneOldMachine {
		t.Errorf("the old machines.json changed")
	}
}

func TestARestartAfterRevokeAllWithTheSameCustomLogDoesNotRestoreTheOldMachines(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	_, logPath := c.oldMachinesBesideACustomLog()
	first := c.startSwitchboard("--log", logPath)
	if names := machineNames(t, c.dataFile("machines.json")); len(names) != 1 {
		t.Fatalf("the first start did not carry over the old machine: %v", names)
	}
	c.mustLandline("server", "revoke", "--all")
	if names := machineNames(t, c.dataFile("machines.json")); len(names) != 0 {
		t.Fatalf("revoke --all left %v", names)
	}
	first.stop()
	c.startSwitchboard("--log", logPath)
	if names := machineNames(t, c.dataFile("machines.json")); len(names) != 0 {
		t.Errorf("a restart with the same custom log brought back %v after revoke --all", names)
	}
}

func TestTheLockFileHoldsTheRunningSwitchboardsProcessIDAndNothingElse(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.mustLandline("server", "up")
	pid := strings.TrimSpace(readFileOrFail(t, c.dataFile("switchboard.pid")))
	if got := readFileOrFail(t, c.dataFile("switchboard.lock")); got != pid {
		t.Errorf("switchboard.lock holds %q, want the process id %s in decimal and nothing else", got, pid)
	}
}

func TestRotateSecretNeverSignalsAProcessNamedByAStalePidFileWhoseCommandLineMentionsTheSwitchboard(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	stranger := exec.Command("sh", "-c", "sleep 30; : landline-switchboard")
	if err := stranger.Start(); err != nil {
		t.Fatal(err)
	}
	ended := make(chan struct{})
	go func() { stranger.Wait(); close(ended) }()
	t.Cleanup(func() { stranger.Process.Kill(); <-ended })
	c.writeDataFile("switchboard.pid", strconv.Itoa(stranger.Process.Pid))
	c.landline("server", "rotate-secret")
	select {
	case <-ended:
		t.Errorf("rotate-secret stopped process %d, which only a stale switchboard.pid named and no lock held", stranger.Process.Pid)
	case <-time.After(500 * time.Millisecond):
	}
}

func saysNoSwitchboardRuns(status string) bool {
	return strings.Contains(status, "no switchboard") && !strings.Contains(status, "started by landline server up")
}
