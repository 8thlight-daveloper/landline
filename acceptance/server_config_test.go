package acceptance

import (
	"bytes"
	"context"
	"errors"
	"github.com/8thlight-daveloper/landline/testsupport/clock/sleeper"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const switchboardExitWait = 10 * time.Second

type configuredHome struct {
	t    *testing.T
	bin  string
	home string
	env  []string
	addr string
	host string
	port string
}

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func newConfiguredHome(t *testing.T, table string) *configuredHome {
	t.Helper()
	c := &configuredHome{t: t, bin: build(t), home: resolvedTempDir(t)}
	c.env = isolatedEnv(c.home)
	c.addr = freeAddr(t)
	c.host, c.port = hostAndPort(t, c.addr)
	c.writeTable(table)
	t.Cleanup(func() { runIn(t, c.bin, c.env, c.home, "server", "down") })
	return c
}

func (c *configuredHome) topLevel() string {
	return "secret = \"" + secret + "\"\nhost = \"" + c.host + "\"\nport = " + c.port + "\n"
}

func (c *configuredHome) writeTable(table string) {
	c.t.Helper()
	text := c.topLevel()
	if table != "" {
		text += "\n[server]\n" + table
	}
	c.writeConfig(text)
}

func (c *configuredHome) writeConfig(text string) {
	c.t.Helper()
	writeConfigFile(c.t, c.home, "config.toml", text)
}

func (c *configuredHome) configText() string {
	c.t.Helper()
	return readFileOrFail(c.t, configTomlOf(c.home))
}

func (c *configuredHome) path(relative string) string { return filepath.Join(c.home, relative) }

func (c *configuredHome) dataFile(name string) string {
	return filepath.Join(c.home, ".local", "share", "landline", name)
}

func (c *configuredHome) landline(args ...string) binResult {
	c.t.Helper()
	cmd := exec.Command(c.bin, args...)
	cmd.Env, cmd.Dir = c.env, c.home
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	result := binResult{stdout: out.String(), stderr: errb.String()}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		result.code = exit.ExitCode()
	case err != nil:
		c.t.Fatalf("landline %v: %v", args, err)
	}
	return result
}

func (c *configuredHome) mustLandline(args ...string) binResult {
	c.t.Helper()
	result := c.landline(args...)
	if result.code != 0 {
		c.t.Fatalf("landline %v: %d %s %s", args, result.code, safe(result.stdout), safe(result.stderr))
	}
	return result
}

type switchboardProcess struct {
	stderr *syncBuffer
	exited chan struct{}
	stop   func()
}

func (c *configuredHome) startSwitchboard(args ...string) *switchboardProcess {
	c.t.Helper()
	cmd := exec.Command(switchboardOf(c.bin), args...)
	cmd.Env, cmd.Dir = c.env, c.home
	p := &switchboardProcess{stderr: &syncBuffer{}, exited: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = p.stderr, p.stderr
	if err := cmd.Start(); err != nil {
		c.t.Fatal(err)
	}
	go func() { cmd.Wait(); close(p.exited) }()
	p.stop = func() { cmd.Process.Kill(); <-p.exited }
	c.t.Cleanup(p.stop)
	deadline := time.Now().Add(switchboardExitWait)
	for time.Now().Before(deadline) {
		select {
		case <-p.exited:
			c.t.Fatalf("%s %v exited before it listened: %s", switchboardName, args, safe(p.stderr.String()))
		default:
		}
		if answers(c.addr) {
			return p
		}
		time.Sleep(50 * time.Millisecond)
	}
	c.t.Fatalf("%s %v did not listen at %s: %s", switchboardName, args, c.addr, safe(p.stderr.String()))
	return nil
}

func (c *configuredHome) switchboardExit(args ...string) binResult {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), switchboardExitWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, switchboardOf(c.bin), args...)
	cmd.Env, cmd.Dir = c.env, c.home
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ctx.Err() != nil {
		c.t.Fatalf("%s %v kept running, want it to stop at start: %s", switchboardName, args, safe(errb.String()))
	}
	result := binResult{stdout: out.String(), stderr: errb.String()}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		result.code = exit.ExitCode()
	}
	return result
}

func (c *configuredHome) asHost() *hostRig {
	c.t.Helper()
	values, _ := readTomlValues(c.t, configTomlOf(c.home))
	return &hostRig{t: c.t, bin: c.bin, home: c.home, env: c.env, addr: c.addr, host: c.host, port: c.port, secret: values["secret"], url: "http://" + c.addr, began: time.Now()}
}

func (c *configuredHome) switchboardOut() string {
	raw, _ := os.ReadFile(c.dataFile("switchboard.out"))
	return string(raw)
}

func answers(address string) bool {
	connection, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
	if err != nil {
		return false
	}
	connection.Close()
	return true
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func serverPart(text string) string {
	if at := strings.Index(text, "\n[server]\n"); at >= 0 {
		return text[at:]
	}
	return ""
}

func linesNaming(text string, words ...string) []string {
	var found []string
	for _, line := range strings.Split(text, "\n") {
		all := true
		for _, word := range words {
			all = all && strings.Contains(line, word)
		}
		if all {
			found = append(found, line)
		}
	}
	return found
}

func flagLine(text, flag string) []string { return linesNaming(text, flag, "config.toml") }

func lineIndex(text, part string) int {
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, part) {
			return i
		}
	}
	return -1
}

func errorAtLine(text, number string) bool {
	return regexp.MustCompile(`config\.toml line ` + number + `\b`).MatchString(text)
}

type fileState struct {
	content string
	mode    os.FileMode
	changed time.Time
}

func stateOf(t *testing.T, path string) fileState {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return fileState{content: readFileOrFail(t, path), mode: info.Mode(), changed: info.ModTime()}
}

func machineNames(t *testing.T, path string) []string {
	t.Helper()
	var names []string
	for _, entry := range storedEntries(t, readFileOrFail(t, path)) {
		name, _ := entry["name"].(string)
		key, _ := entry["public_key"].(string)
		names = append(names, name+" "+key)
	}
	return names
}

func (c *configuredHome) moveMachinesBeside(logDir string) string {
	c.t.Helper()
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		c.t.Fatal(err)
	}
	old := filepath.Join(logDir, "machines.json")
	if err := os.Rename(c.dataFile("machines.json"), old); err != nil {
		c.t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		c.t.Fatal(err)
	}
	return old
}

func (c *configuredHome) joinedMachineBesideAnOldLog(logDir string) (*hostRig, *machineRig, string) {
	c.t.Helper()
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	h := c.asHost()
	m := h.machine("linux-box")
	c.mustLandline("server", "down")
	return h, m, c.moveMachinesBeside(logDir)
}

func (m *machineRig) registers(id string) bool {
	m.t.Helper()
	_, err := m.run("register", "--id", id, "--harness", "claude-code", "--host", "mac")
	return err == nil
}

func TestTheSwitchboardRunWithNoArgumentsTakesItsLogFromTheServerTableWithTildeAsTheHomeDirectory(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "log = \"~/table-logs/log.jsonl\"\n")
	c.startSwitchboard()
	if !exists(c.path("table-logs/log.jsonl")) {
		t.Errorf("the switchboard run with no arguments did not open the log that [server] names")
	}
	if exists(c.dataFile("log.jsonl")) {
		t.Errorf("the switchboard opened the default log although [server] names another")
	}
}

func TestEveryStartPathWithNoFlagsWritesTheAbsoluteLogTheServerTableNames(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	tableLog := c.path("abs-logs/log.jsonl")
	c.writeTable("log = \"" + tableLog + "\"\n")
	steps := []struct {
		name  string
		start func()
	}{
		{"server up", func() { c.mustLandline("server", "up") }},
		{"server restart", func() { c.mustLandline("server", "restart") }},
		{"the binary with no arguments, as the service runs it", func() {
			c.mustLandline("server", "down")
			c.startSwitchboard()
		}},
	}
	for _, step := range steps {
		os.RemoveAll(filepath.Dir(tableLog))
		step.start()
		if !exists(tableLog) {
			t.Errorf("%s did not open the log that [server] names", step.name)
		}
		if exists(c.dataFile("log.jsonl")) {
			t.Errorf("%s opened the default log although [server] names another", step.name)
		}
	}
}

func TestTheKeepaliveDeliveryTimeoutAndGraceOfTheServerTableReachTheSwitchboard(t *testing.T) {
	t.Parallel()
	config := t.TempDir()
	writeConfigFile(t, config, "config.toml", "[server]\nkeepalive = \"200ms\"\ndelivery-timeout = \"500ms\"\ngrace = \"1s\"\n")
	w := startHub(t, build(t), config, "")
	w.register("delta")
	registered := time.Now()
	w.register("alpha", "bravo", "charlie")
	stream := w.openRawLines("bravo")
	stream.waitFor(t, ": keepalive", time.Second)
	stream.waitFor(t, ": keepalive", time.Second)
	w.openStream("charlie", false)
	w.waitState("charlie", "connected")
	started := time.Now()
	status, body := w.sendHTTP("alpha", "charlie", "nobody reports this")
	if elapsed := time.Since(started); status != 504 || !strings.Contains(body, "no delivery report") || elapsed > 3*time.Second {
		t.Errorf("a send with no report answered %d after %s, want 504 after the 500ms of [server]", status, elapsed)
	}
	sleeper.Until(registered.Add(1600 * time.Millisecond))
	if status := w.get("/v1/agents", w.token("delta")); status != 401 {
		t.Errorf("an agent silent past the 1s grace of [server] still answers %d", status)
	}
}

func TestTheSignatureWindowAndInviteLifetimeOfTheServerTableHoldOnEveryStartPath(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "signature-window = \"3s\"\ninvite-lifetime = \"2s\"\n")
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	h := c.asHost()
	m := h.machine("linux-box")
	windowHolds := func(path string) {
		t.Helper()
		if status := h.signedStatus(m, time.Now().Unix(), "GET", "/v1/build", nil); status != 200 {
			t.Errorf("after %s, a request timed now answered %d", path, status)
		}
		if status := h.signedStatus(m, time.Now().Unix()+10, "GET", "/v1/build", nil); !refused(status) {
			t.Errorf("after %s, a time 10 seconds ahead answered %d, want a refusal under the 3s window of [server]", path, status)
		}
	}
	windowHolds("server up")
	expired := h.invite("slow-box")
	time.Sleep(3 * time.Second)
	if out, err := h.newMachineHome().joinWith(expired); err == nil || !strings.Contains(out, invalidCode) {
		t.Errorf("a 3 second old code under the 2s invite lifetime of [server]: %v %q", err, h.clean(out))
	}
	h.restartAndSettle()
	windowHolds("server restart")
	if out, err := h.run("server", "rotate-secret"); err != nil {
		t.Fatalf("rotate-secret: %v %s", err, h.clean(out))
	}
	waitPastTheStartPeriod()
	windowHolds("server rotate-secret")
	if out, err := h.run("server", "down"); err != nil {
		t.Fatalf("down: %v %s", err, h.clean(out))
	}
	c.startSwitchboard()
	waitPastTheStartPeriod()
	windowHolds("the binary with no arguments")
}

func TestAServerTableThatOmitsAKeyLeavesTheDefaultForThatKey(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "invite-lifetime = \"5m\"\n")
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	if !exists(c.dataFile("log.jsonl")) {
		t.Errorf("a [server] table with no log did not leave the log at %s", c.dataFile("log.jsonl"))
	}
	h := c.asHost()
	m := h.machine("linux-box")
	if status := h.signedStatus(m, time.Now().Unix()+4, "GET", "/v1/build", nil); status != 200 {
		t.Errorf("a [server] table with no signature-window refused a time 4 seconds ahead (%d), want the default window of 5 seconds", status)
	}
	if status := h.signedStatus(m, time.Now().Unix()+8, "GET", "/v1/build", nil); !refused(status) {
		t.Errorf("a [server] table with no signature-window accepted a time 8 seconds ahead (%d), want the default window of 5 seconds", status)
	}
}

var badServerValues = map[string]struct{ line, echo string }{
	"a relative log":                {`log = "relative-zebra/log.jsonl"`, "relative-zebra"},
	"a log under another user":      {`log = "~zebra/log.jsonl"`, "~zebra"},
	"an empty log":                  {`log = ""`, `log = ""`},
	"a log that is a number":        {`log = 5`, "log = 5"},
	"a keepalive of zero":           {`keepalive = "0s"`, `"0s"`},
	"a negative delivery timeout":   {`delivery-timeout = "-5s"`, "-5s"},
	"a grace that is no duration":   {`grace = "zebra"`, "zebra"},
	"an invite lifetime number":     {`invite-lifetime = 300`, "300"},
	"an empty signature window":     {`signature-window = ""`, `signature-window = ""`},
	"a signature window of 0s":      {`signature-window = "0s"`, `"0s"`},
	"a signature window of 500ms":   {`signature-window = "500ms"`, "500ms"},
	"a signature window of 6s":      {`signature-window = "6s"`, `"6s"`},
	"a signature window of 60s":     {`signature-window = "60s"`, "60s"},
	"a duration without a unit":     {`keepalive = "15"`, `"15"`},
	"a second server table":         {"[server]", ""},
	"a key given twice in [server]": {"# chosen by hand\nkeepalive = \"15s\"", ""},
}

func (c *configuredHome) repeats(output, echo string) bool {
	return echo != "" && strings.Contains(withoutPaths(output), echo)
}

func TestABadValueInTheServerTableStopsTheBinaryAndUpWithExitOneNamingTheFileAndLine(t *testing.T) {
	t.Parallel()
	for name, each := range badServerValues {
		c := newConfiguredHome(t, "")
		table := "keepalive = \"15s\"\n" + each.line + "\n"
		wantLine := "7"
		if strings.Contains(each.line, "\n") {
			wantLine = "8"
		}
		c.writeTable(table)
		written := c.configText()
		result := c.switchboardExit()
		if result.code != 1 || !errorAtLine(result.stderr, wantLine) || c.repeats(result.stdout+result.stderr, each.echo) {
			t.Errorf("%s, the binary: %d %q, want 1 and an error at config.toml line %s that does not repeat the line", name, result.code, result.stderr, wantLine)
		}
		result = c.landline("server", "up")
		combined := result.stdout + result.stderr
		if result.code != 1 || !errorAtLine(combined, wantLine) || c.repeats(combined, each.echo) {
			t.Errorf("%s, server up: %d %q, want 1 and an error at config.toml line %s that does not repeat the line", name, result.code, combined, wantLine)
		}
		if answers(c.addr) || exists(c.dataFile("switchboard.pid")) || c.configText() != written {
			t.Errorf("%s: a refused start left a switchboard, a pid file, or a changed config.toml", name)
		}
	}
}

func TestABadValueInTheServerTableStopsRestartAndRotateSecretBeforeTheyStopTheRunningSwitchboard(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.mustLandline("server", "up")
	pid := readFileOrFail(t, c.dataFile("switchboard.pid"))
	c.writeTable("# chosen by hand\nkeepalive = \"zebra\"\n")
	written := c.configText()
	for _, command := range [][]string{{"server", "restart"}, {"server", "rotate-secret"}} {
		result := c.landline(command...)
		combined := result.stdout + result.stderr
		if result.code != 1 || !errorAtLine(combined, "7") || strings.Contains(combined, "zebra") {
			t.Errorf("%v: %d %q, want 1 and an error at config.toml line 7 that does not repeat the line", command, result.code, combined)
		}
		if !answers(c.addr) || readFileOrFail(t, c.dataFile("switchboard.pid")) != pid {
			t.Errorf("%v stopped or replaced the running switchboard", command)
		}
		if c.configText() != written {
			t.Errorf("%v changed config.toml, so the secret may have changed", command)
		}
	}
}

func TestAFlagOverridesTheServerTableForOneRunAndIsNeverWrittenToConfigToml(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "log = \"~/table-logs/log.jsonl\"\nkeepalive = \"15s\"\n")
	table := serverPart(c.configText())
	c.mustLandline("server", "up", "--log", c.path("flag-logs/log.jsonl"), "--keepalive", "200ms")
	if !exists(c.path("flag-logs/log.jsonl")) || exists(c.path("table-logs/log.jsonl")) {
		t.Errorf("up --log did not override the log of [server] for its run")
	}
	text := c.configText()
	if serverPart(text) != table || strings.Contains(text, "flag-logs") || strings.Contains(text, "200ms") {
		t.Errorf("up wrote a flag's value to config.toml:\n%s", scrub(text))
	}
	c.mustLandline("server", "restart")
	if !exists(c.path("table-logs/log.jsonl")) {
		t.Errorf("restart with no flags kept the last run's --log instead of the log of [server]")
	}
	c.mustLandline("server", "down")
	os.RemoveAll(c.path("table-logs"))
	c.startSwitchboard("--log", c.path("binary-logs/log.jsonl"))
	if !exists(c.path("binary-logs/log.jsonl")) || exists(c.path("table-logs/log.jsonl")) {
		t.Errorf("the binary's --log did not override the log of [server]")
	}
	if text := c.configText(); serverPart(text) != table || strings.Contains(text, "binary-logs") {
		t.Errorf("the binary wrote a flag's value to config.toml:\n%s", scrub(text))
	}
}

func TestUpAndRestartPrintOneLineOnStandardOutputBeforeTheResultForEachFlagThatDiffersInMeaning(t *testing.T) {
	t.Parallel()
	hostname, err := os.Hostname()
	if err != nil {
		t.Skip("this machine has no host name")
	}
	c := newConfiguredHome(t, "log = \"~/table-logs/log.jsonl\"\nkeepalive = \"15s\"\ninvite-lifetime = \"5m\"\n")
	result := c.mustLandline("server", "up", "--keepalive", "200ms", "--invite-lifetime", "300s", "--log", "table-logs/log.jsonl",
		"--grace", "120s", "--delivery-timeout", "5s", "--host", hostname)
	for flag, want := range map[string]int{"--keepalive": 1, "--delivery-timeout": 1, "--invite-lifetime": 0, "--log": 0, "--grace": 0, "--host": 0} {
		if got := len(flagLine(result.stdout, flag)); got != want {
			t.Errorf("up printed %d lines that name %s and config.toml, want %d:\n%s", got, flag, want, result.stdout)
		}
	}
	for _, line := range flagLine(result.stdout, "--keepalive") {
		if !strings.Contains(line, "restart") {
			t.Errorf("the line does not say that restart uses the value in config.toml: %q", line)
		}
	}
	if warning, running := lineIndex(result.stdout, "--keepalive"), lineIndex(result.stdout, "the switchboard is running"); warning < 0 || running < warning {
		t.Errorf("the flag line is not before the result line on standard output:\n%s", result.stdout)
	}
	if len(flagLine(result.stderr, "--keepalive")) != 0 {
		t.Errorf("up printed the flag line on standard error: %q", result.stderr)
	}
	result = c.mustLandline("server", "restart", "--host", "label-elsewhere")
	if len(flagLine(result.stdout, "--host")) != 1 || len(flagLine(result.stdout, "--keepalive")) != 0 {
		t.Errorf("restart --host label-elsewhere: want one line for --host and none for flags it was not given:\n%s", result.stdout)
	}
}

func TestAFlagWithNoServerTableGetsTheLineOnlyWhenItDiffersFromTheDefault(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	result := c.mustLandline("server", "up", "--delivery-timeout", "5s", "--grace", "2m", "--log", c.dataFile("log.jsonl"))
	if len(flagLine(result.stdout, "--delivery-timeout")) != 1 || len(flagLine(result.stdout, "--grace")) != 0 || len(flagLine(result.stdout, "--log")) != 0 {
		t.Errorf("want one line for --delivery-timeout 5s and none for the default --grace 2m and the default log:\n%s", result.stdout)
	}
}

func TestTheSwitchboardPrintsTheFlagLineOnStandardError(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "keepalive = \"15s\"\n")
	p := c.startSwitchboard("--keepalive", "200ms", "--grace", "2m")
	eventually(t, "the flag line for --keepalive", func() bool { return len(flagLine(p.stderr.String(), "--keepalive")) == 1 })
	if len(flagLine(p.stderr.String(), "--grace")) != 0 {
		t.Errorf("the switchboard printed a line for --grace 2m, which is the default:\n%s", p.stderr.String())
	}
}

func TestMachinesJSONLivesInTheDataDirectoryWhateverTheLogFlagOrTheTableSays(t *testing.T) {
	t.Parallel()
	for _, how := range []string{"flag", "table"} {
		c := newConfiguredHome(t, "")
		custom := c.path("custom-logs/log.jsonl")
		if how == "table" {
			c.writeTable("log = \"~/custom-logs/log.jsonl\"\n")
			c.mustLandline("server", "up")
		} else {
			c.mustLandline("server", "up", "--log", custom)
		}
		waitPastTheStartPeriod()
		h := c.asHost()
		if strings.Contains(listedMachines(t, h), "linux-box") {
			t.Fatalf("%s: a fresh switchboard lists a machine", how)
		}
		h.machine("linux-box")
		if !exists(c.dataFile("machines.json")) || len(machineNames(t, c.dataFile("machines.json"))) != 1 {
			t.Errorf("%s: machines.json is not at %s with the one machine", how, c.dataFile("machines.json"))
		}
		if exists(filepath.Join(filepath.Dir(custom), "machines.json")) {
			t.Errorf("%s: machines.json was written next to the custom log", how)
		}
	}
}

func TestAMachinesJSONBesideTheFlagLogIsCopiedToTheDataDirectoryAtStartAndTheOldFileIsNeverChanged(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	customDir := c.path("custom-logs")
	h, m, old := c.joinedMachineBesideAnOldLog(customDir)
	before := stateOf(t, old)
	wantNames := machineNames(t, old)
	c.mustLandline("server", "up", "--log", filepath.Join(customDir, "log.jsonl"))
	waitPastTheStartPeriod()
	if !exists(c.dataFile("machines.json")) {
		t.Fatalf("the start did not copy %s to %s", old, c.dataFile("machines.json"))
	}
	if got := machineNames(t, c.dataFile("machines.json")); strings.Join(got, ",") != strings.Join(wantNames, ",") {
		t.Errorf("the copy holds %v, want the entries of the old file %v", got, wantNames)
	}
	if len(linesNaming(c.switchboardOut(), old, c.dataFile("machines.json"))) == 0 {
		t.Errorf("no line names both %s and %s:\n%s", old, c.dataFile("machines.json"), c.switchboardOut())
	}
	if !m.registers("alpha") {
		t.Errorf("the machine that joined before the move no longer registers")
	}
	h.machine("second-box")
	if len(machineNames(t, c.dataFile("machines.json"))) != 2 {
		t.Errorf("the second machine was not saved in %s", c.dataFile("machines.json"))
	}
	c.mustLandline("server", "down")
	c.mustLandline("server", "up", "--log", filepath.Join(customDir, "log.jsonl"))
	waitPastTheStartPeriod()
	if len(linesNaming(c.switchboardOut(), old)) == 0 {
		t.Errorf("a start with both files did not name the old file %s:\n%s", old, c.switchboardOut())
	}
	if listed := listedMachines(t, h); !strings.Contains(listed, "linux-box") || !strings.Contains(listed, "second-box") {
		t.Errorf("the second start did not read the data directory's machines.json: %s", listed)
	}
	if after := stateOf(t, old); after != before {
		t.Errorf("the old machines.json was changed: mode %v to %v, time %v to %v, content changed %v", before.mode, after.mode, before.changed, after.changed, before.content != after.content)
	}
}

func TestAMachinesJSONBesideTheTableLogIsCopiedWhenUpRunsWithNoFlags(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	_, m, old := c.joinedMachineBesideAnOldLog(c.path("custom-logs"))
	before := stateOf(t, old)
	c.writeTable("log = \"~/custom-logs/log.jsonl\"\n")
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	if !exists(c.dataFile("machines.json")) || strings.Join(machineNames(t, c.dataFile("machines.json")), ",") != strings.Join(machineNames(t, old), ",") {
		t.Errorf("up with no flags did not copy the machines.json beside the log of [server]")
	}
	if !m.registers("alpha") {
		t.Errorf("the machine no longer registers")
	}
	if stateOf(t, old) != before {
		t.Errorf("the old machines.json was changed")
	}
}

func TestWhenBothFilesExistTheDataDirectoryOneWinsAndEachStartNamesTheOldOne(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	h := c.asHost()
	oldBox := h.machine("old-box")
	c.mustLandline("server", "down")
	customDir := c.path("custom-logs")
	if err := os.MkdirAll(customDir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(customDir, "machines.json")
	if err := os.WriteFile(old, []byte(readFileOrFail(t, c.dataFile("machines.json"))), 0o600); err != nil {
		t.Fatal(err)
	}
	before := stateOf(t, old)
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	if out, err := h.run("server", "revoke", "old-box"); err != nil {
		t.Fatalf("revoke: %v %s", err, h.clean(out))
	}
	newBox := h.machine("new-box")
	c.mustLandline("server", "down")
	c.mustLandline("server", "up", "--log", filepath.Join(customDir, "log.jsonl"))
	waitPastTheStartPeriod()
	if listed := listedMachines(t, h); !strings.Contains(listed, "new-box") || strings.Contains(listed, "old-box") {
		t.Errorf("with both files, the switchboard did not read only the data directory's: %s", listed)
	}
	if oldBox.registers("alpha") || !newBox.registers("bravo") {
		t.Errorf("with both files, the old file's machine registers or the new file's does not")
	}
	if len(linesNaming(c.switchboardOut(), old)) == 0 {
		t.Errorf("the start did not name the old file %s:\n%s", old, c.switchboardOut())
	}
	if stateOf(t, old) != before {
		t.Errorf("the old machines.json was changed")
	}
}

func TestAFailedCopyOfAnOldMachinesJSONExitsOneNamingTheNewFileAndWritesNothing(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes into a folder without write permission")
	}
	c := newConfiguredHome(t, "")
	customDir := c.path("custom-logs")
	_, _, old := c.joinedMachineBesideAnOldLog(customDir)
	before := stateOf(t, old)
	dataDir := filepath.Dir(c.dataFile("machines.json"))
	entries, _ := os.ReadDir(dataDir)
	if err := os.Chmod(dataDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dataDir, 0o700) })
	result := c.switchboardExit("--log", filepath.Join(customDir, "log.jsonl"))
	if result.code != 1 || !strings.Contains(result.stderr, c.dataFile("machines.json")) {
		t.Errorf("a failed copy: %d %q, want 1 and a message that names %s", result.code, result.stderr, c.dataFile("machines.json"))
	}
	if after, _ := os.ReadDir(dataDir); len(after) != len(entries) || exists(c.dataFile("machines.json")) {
		t.Errorf("a failed copy wrote into the data directory")
	}
	if stateOf(t, old) != before {
		t.Errorf("a failed copy changed the old machines.json")
	}
}

func TestAnOldMachinesJSONThatCannotBeReadStopsTheStartAndIsNotCopied(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	customDir := c.path("custom-logs")
	if err := os.MkdirAll(customDir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(customDir, "machines.json")
	if err := os.WriteFile(old, []byte("this is not a list of machines\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := c.switchboardExit("--log", filepath.Join(customDir, "log.jsonl"))
	if result.code != 1 || exists(c.dataFile("machines.json")) {
		t.Errorf("an unreadable old machines.json: %d %q, new file written %v", result.code, result.stderr, exists(c.dataFile("machines.json")))
	}
	if readFileOrFail(t, old) != "this is not a list of machines\n" {
		t.Errorf("the old file was changed")
	}
}

func TestRotateSecretStopStartAndTheServiceKeepTheMachinesOfASwitchboardWhoseLogIsInTheServerTable(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "# by hand\nlog = \"~/table-logs/log.jsonl\"\n")
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	h := c.asHost()
	m := h.machine("linux-box")
	m.registerAgent("alpha")
	table := serverPart(c.configText())
	if out, err := h.run("server", "rotate-secret"); err != nil || !strings.Contains(out, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret with no flags, as the menu runs it: %v %s", err, h.clean(out))
	}
	if serverPart(c.configText()) != table {
		t.Errorf("the rotation changed the [server] table:\n%s", scrub(c.configText()))
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") || !m.registers("bravo") {
		t.Errorf("after a rotation with no flags, the joined machine is locked out")
	}
	c.mustLandline("server", "down")
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	if !m.registers("charlie") {
		t.Errorf("after the menu's Stop then Start, the joined machine is locked out")
	}
	c.mustLandline("server", "down")
	c.startSwitchboard()
	waitPastTheStartPeriod()
	if !m.registers("delta") {
		t.Errorf("under the binary run with no arguments, as the service runs it, the joined machine is locked out")
	}
}

func TestRotateSecretAfterAOneRunLogFlagKeepsTheRunningSwitchboardOnItsLogAndKeepsTheMachines(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "log = \"~/table-logs/log.jsonl\"\n")
	c.mustLandline("server", "up", "--log", c.path("flag-logs/log.jsonl"))
	waitPastTheStartPeriod()
	h := c.asHost()
	m := h.machine("linux-box")
	pid := readFileOrFail(t, c.dataFile("switchboard.lock"))
	if out, err := h.run("server", "rotate-secret"); err != nil {
		t.Fatalf("rotate-secret: %v %s", err, h.clean(out))
	}
	if exists(c.path("table-logs/log.jsonl")) || readFileOrFail(t, c.dataFile("switchboard.lock")) != pid {
		t.Errorf("rotate-secret replaced the running switchboard instead of reloading its secret")
	}
	if !m.registers("alpha") {
		t.Errorf("after the rotation, the joined machine is locked out")
	}
}

func TestServerUpWritesMissingTopLevelKeysAboveTheServerTableAndKeepsItsLines(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	table := "[server]\n# where the log goes\nlog = \"~/table-logs/log.jsonl\"\n"
	c.writeConfig(table)
	c.mustLandline("server", "up", "--addr", c.addr)
	text := c.configText()
	if !strings.HasSuffix(text, "\n"+table) {
		t.Errorf("up did not keep the [server] table at the end of the file:\n%s", scrub(text))
	}
	values, lines := readTomlValues(t, configTomlOf(c.home))
	tableAt := -1
	for i, line := range lines {
		if line == "[server]" {
			tableAt = i
		}
	}
	for _, key := range []string{"secret", "host", "port"} {
		at := -1
		for i, line := range lines {
			if strings.HasPrefix(normalized(line), key+" = ") {
				at = i
			}
		}
		if at < 0 || at > tableAt {
			t.Errorf("%s is not written above the [server] line:\n%s", key, scrub(text))
		}
	}
	if values["host"] != c.host || values["port"] != c.port {
		t.Errorf("config.toml holds %v", redacted(values))
	}
	if info, err := os.Stat(configTomlOf(c.home)); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("config.toml mode: %v %v", info, err)
	}
	if !exists(c.path("table-logs/log.jsonl")) {
		t.Errorf("up did not start the switchboard on the log of [server]")
	}
	if result := c.landline("log"); result.code != 0 {
		t.Errorf("a client could not read the saved keys: %+v", result)
	}
}

func TestEveryClientReadsTheTopLevelKeysAboveAServerTableAndNeverAKeyInsideATable(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("alpha")
	host, port := hostAndPort(t, w.addr)
	content := "secret = \"" + secret + "\"\nhost = \"" + host + "\"\nport = " + port + "\n\n[server]\n# by hand\n" +
		"log = \"~/table-logs/log.jsonl\"\nkeepalive = \"15s\"\nhost = \"" + unownedAddress + "\"\nport = 1\nsecret = \"wrong-secret-wrong-secret\"\n" +
		"\n[elsewhere]\nhost = \"" + unownedAddress + "\"\n"
	for _, command := range clientCommands {
		home := t.TempDir()
		writeConfigFile(t, home, "config.toml", content)
		if result := runBin(t, w.bin, isolatedEnv(home), command.args...); result.code != 0 {
			t.Errorf("%s: %d %q %q", command.name, result.code, result.stdout, result.stderr)
		}
	}
}

func TestEveryClientRefusesASecondServerTableOrAKeyGivenTwiceInItAtItsLine(t *testing.T) {
	t.Parallel()
	bin := build(t)
	top := "secret = \"" + joinedSecret + "\"\nhost = \"127.0.0.1\"\nport = %s\n[server]\n"
	cases := []struct{ name, content, line, echo string }{
		{"a second [server] line", top + "keepalive = \"15s\"\n[elsewhere]\n[server]\n", "7", ""},
		{"a key given twice", top + "grace = \"2m\"\n# again\ngrace = \"9m\"\n", "7", "9m"},
		{"a dotted table name", top + "[a.b]\n", "5", "a.b"},
	}
	for _, command := range clientCommands {
		for _, each := range cases {
			home := t.TempDir()
			writeConfigFile(t, home, "config.toml", strings.ReplaceAll(each.content, "%s", freePort(t)))
			result := runBin(t, bin, isolatedEnv(home), command.args...)
			combined := result.stdout + result.stderr
			if result.code == 0 || !errorAtLine(combined, each.line) || (each.echo != "" && strings.Contains(withoutPaths(combined), each.echo)) {
				t.Errorf("%s, %s: %d %q", command.name, each.name, result.code, combined)
			}
		}
	}
}

func TestMenuTextReadsAConfigWithAServerTableAndTreatsASecondServerTableAsUnreadable(t *testing.T) {
	t.Parallel()
	w := start(t)
	host, port := hostAndPort(t, w.addr)
	good := tomlFor(host, port, secret) + "\n[server]\nlog = \"~/table-logs/log.jsonl\"\nhost = \"" + unownedAddress + "\"\nport = 1\n"
	rig := newMenubarRig(t, defaultFlags(t), "", landlineNextToBinary)
	writeConfigFile(t, rig.home, "config.toml", good)
	if got := rig.menu(t); !strings.Contains(got, "Running at "+w.url) {
		t.Errorf("the menu did not read the top-level keys above a [server] table:\n%s", got)
	}
	rig = newMenubarRig(t, defaultFlags(t), "", landlineNextToBinary)
	writeConfigFile(t, rig.home, "config.toml", good+"[server]\n")
	if got := rig.menu(t); !strings.HasPrefix(got, "icon: jack dimmed\n") || strings.Contains(got, "Running at") {
		t.Errorf("the menu read a file with a second [server] line:\n%s", got)
	}
}

func TestASignatureWindowFlagOutsideOneToFiveSecondsStopsTheBinaryWithExitOneBeforeItListens(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"0s", "500ms", "6s", "60s"} {
		c := newConfiguredHome(t, "")
		result := c.switchboardExit("--signature-window", bad)
		if result.code != 1 || !strings.Contains(result.stderr, "signature-window") {
			t.Errorf("--signature-window %s: %d %q, want 1 and an error that names signature-window", bad, result.code, result.stderr)
		}
		if answers(c.addr) {
			t.Errorf("--signature-window %s: something listens at %s", bad, c.addr)
		}
		up := c.landline("server", "up", "--signature-window", bad)
		if up.code == 0 || answers(c.addr) || exists(c.dataFile("switchboard.pid")) {
			t.Errorf("server up --signature-window %s: %d, want a refusal that starts no switchboard", bad, up.code)
		}
	}
}

func TestTheBinaryExitsThreeWhenItCannotListenAndOneWhenItFailsToStartForAnotherReason(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { taken.Close() })
	if result := c.switchboardExit("--addr", taken.Addr().String()); result.code != 3 {
		t.Errorf("an address another process holds: %d %q, want 3", result.code, result.stderr)
	}
	blocker := c.path("a-file")
	if err := os.WriteFile(blocker, []byte("not a folder\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := c.switchboardExit("--log", filepath.Join(blocker, "log.jsonl")); result.code != 1 {
		t.Errorf("a log that cannot be opened: %d %q, want 1", result.code, result.stderr)
	}
	c.writeTable("signature-window = \"6s\"\n")
	if result := c.switchboardExit(); result.code != 1 {
		t.Errorf("a signature-window of 6s in [server]: %d %q, want 1", result.code, result.stderr)
	}
}
