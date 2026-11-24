package acceptance

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func bearerStatus(t *testing.T, addr, method, path, bearer string, body string) (int, string) {
	t.Helper()
	request, _ := http.NewRequest(method, "http://"+addr+path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+bearer)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return -1, err.Error()
	}
	defer response.Body.Close()
	var raw strings.Builder
	buffer := make([]byte, 4096)
	for {
		n, err := response.Body.Read(buffer)
		raw.Write(buffer[:n])
		if err != nil {
			break
		}
	}
	return response.StatusCode, raw.String()
}

func (c *configuredHome) savedSecret() string {
	c.t.Helper()
	values, _ := readTomlValues(c.t, configTomlOf(c.home))
	return values["secret"]
}

func (c *configuredHome) acceptsOnMachines(secretText string) bool {
	status, _ := bearerStatus(c.t, c.addr, "GET", "/v1/machines", secretText, "")
	return status == http.StatusOK
}

func (c *configuredHome) lockedPid() int {
	c.t.Helper()
	pid, err := strconv.Atoi(strings.TrimSpace(readFileOrFail(c.t, c.dataFile("switchboard.lock"))))
	if err != nil {
		c.t.Fatalf("switchboard.lock does not hold a process id: %v", err)
	}
	return pid
}

func (c *configuredHome) agentToken(id string) string {
	c.t.Helper()
	status, raw := bearerStatus(c.t, c.addr, "POST", "/v1/register", c.savedSecret(), `{"id":"`+id+`","harness":"claude-code","host":"mac"}`)
	var answer struct{ Token string }
	if status != http.StatusOK || json.Unmarshal([]byte(raw), &answer) != nil || answer.Token == "" {
		c.t.Fatalf("register %s: %d %s", id, status, raw)
	}
	return answer.Token
}

func TestRotateSecretReloadsTheRunningSwitchboardWithoutAStartPeriodAndEndsEveryAgentToken(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	h := c.asHost()
	m := h.machine("linux-box")
	old := c.savedSecret()
	token := c.agentToken("alpha")
	pid := c.lockedPid()

	result := c.landline("server", "rotate-secret")
	if result.code != 0 || !strings.Contains(result.stdout, "rotated the switchboard secret. Every agent must register again.") {
		t.Fatalf("rotate-secret: %+v", result)
	}
	if got := c.lockedPid(); got != pid {
		t.Errorf("the switchboard was replaced: pid %d before, %d after", pid, got)
	}
	fresh := c.savedSecret()
	if fresh == old || !c.acceptsOnMachines(fresh) || c.acceptsOnMachines(old) {
		t.Errorf("after the rotation: new secret differs %v, accepted %v; old secret accepted %v", fresh != old, c.acceptsOnMachines(fresh), c.acceptsOnMachines(old))
	}
	if status, _ := bearerStatus(t, c.addr, "GET", "/v1/agents", token, ""); status != http.StatusUnauthorized {
		t.Errorf("an agent token from before the rotation answered %d, want 401", status)
	}
	if status := h.signedStatus(m, time.Now().Unix(), "GET", "/v1/build", nil); status != http.StatusOK {
		t.Errorf("right after the rotation, one signed request from a joined machine answered %d, so a start period began or its key stopped working", status)
	}
	if strings.Contains(result.stdout+result.stderr, old) || strings.Contains(result.stdout+result.stderr, fresh) {
		t.Errorf("rotate-secret printed a secret")
	}
}

func TestRotateSecretWithNoSwitchboardRunningSavesStartsOneAndSucceeds(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	old := c.savedSecret()
	result := c.landline("server", "rotate-secret")
	if result.code != 0 || !strings.Contains(result.stdout, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret: %+v", result)
	}
	fresh := c.savedSecret()
	if fresh == old || !c.acceptsOnMachines(fresh) || c.acceptsOnMachines(old) {
		t.Errorf("after the rotation: new secret differs %v and is accepted %v, old accepted %v", fresh != old, c.acceptsOnMachines(fresh), c.acceptsOnMachines(old))
	}
}

func startInGroup(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
	})
}

func TestRotateSecretUnderASupervisorThatRestartsTheSwitchboardReportsSuccessOnlyWhenTheNewSecretWorks(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	supervisor := exec.Command("sh", "-c", `while :; do "$0"; sleep 0.2; done`, switchboardOf(c.bin))
	supervisor.Env, supervisor.Dir = c.env, c.home
	startInGroup(t, supervisor)
	eventually(t, "the supervised switchboard listens", func() bool { return answers(c.addr) })
	old := c.savedSecret()
	result := c.landline("server", "rotate-secret")
	time.Sleep(time.Second)
	fresh := c.savedSecret()
	oldWorks, freshWorks := c.acceptsOnMachines(old), c.acceptsOnMachines(fresh)
	if result.code == 0 && (oldWorks || !freshWorks) {
		t.Errorf("rotate-secret reported success, and then the old secret works %v and the new one works %v: %+v", oldWorks, freshWorks, result)
	}
	if result.code != 0 {
		t.Errorf("rotate-secret under a supervisor: %+v, want a reload that succeeds", result)
	}
}

func TestRotateSecretExitsOneAndSaysSoWhenTheSwitchboardKeepsTheOldSecret(t *testing.T) {
	t.Parallel()
	cases := map[string]func(c *configuredHome){
		"its environment sets LANDLINE_SECRET": func(c *configuredHome) {
			c.env = withEnv(c.env, "LANDLINE_SECRET="+c.savedSecret())
			c.startSwitchboard()
			c.env = withEnv(c.env, "LANDLINE_SECRET=")
		},
		"the process in the lock file ignores SIGHUP": func(c *configuredHome) {
			wrapper := exec.Command("sh", "-c", `trap "" HUP; "$0" & child=$!; while [ ! -s "$1" ]; do sleep 0.05; done; sleep 0.3; printf %s $$ > "$1"; wait $child`,
				switchboardOf(c.bin), c.dataFile("switchboard.lock"))
			wrapper.Env, wrapper.Dir = c.env, c.home
			startInGroup(c.t, wrapper)
			eventually(c.t, "the wrapped switchboard listens", func() bool { return answers(c.addr) })
			eventually(c.t, "the wrapper names itself in the lock file", func() bool {
				return strings.TrimSpace(readFileOrFail(c.t, c.dataFile("switchboard.lock"))) == strconv.Itoa(wrapper.Process.Pid)
			})
		},
	}
	for name, start := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newConfiguredHome(t, "")
			start(c)
			old := c.savedSecret()
			began := time.Now()
			result := c.landline("server", "rotate-secret")
			said := result.stdout + result.stderr
			fresh := c.savedSecret()
			if result.code != 1 || strings.Contains(said, "rotated the switchboard secret") {
				t.Errorf("rotate-secret: %d %q, want 1 and no success line", result.code, said)
			}
			for _, want := range []string{"config.toml", c.addr} {
				if !strings.Contains(said, want) {
					t.Errorf("the message does not name %q: %q", want, said)
				}
			}
			if strings.Contains(said, old) || strings.Contains(said, fresh) {
				t.Errorf("rotate-secret printed a secret")
			}
			if fresh == old || len(fresh) == 0 {
				t.Errorf("config.toml does not hold a new secret")
			}
			if took := time.Since(began); took > 10*time.Second {
				t.Errorf("rotate-secret took %v, want about 5 seconds at most", took)
			}
		})
	}
}

func TestASwitchboardThatGetsSIGHUPWithAnUnreadableConfigOrNoSecretKeepsTheOldSecretAndRuns(t *testing.T) {
	t.Parallel()
	cases := map[string]func(c *configuredHome){
		"an unreadable config.toml": func(c *configuredHome) {
			if err := os.Chmod(configTomlOf(c.home), 0o000); err != nil {
				t.Fatal(err)
			}
			c.t.Cleanup(func() { os.Chmod(configTomlOf(c.home), 0o600) })
		},
		"a config.toml with no secret": func(c *configuredHome) {
			c.writeConfig("host = \"" + c.host + "\"\nport = " + c.port + "\n")
		},
	}
	for name, spoil := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newConfiguredHome(t, "")
			c.mustLandline("server", "up")
			old := c.savedSecret()
			pid := c.lockedPid()
			before := c.switchboardOut()
			spoil(c)
			if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
				t.Fatal(err)
			}
			time.Sleep(500 * time.Millisecond)
			if syscall.Kill(pid, 0) != nil || !answers(c.addr) {
				t.Fatalf("the switchboard stopped on SIGHUP")
			}
			if !c.acceptsOnMachines(old) {
				t.Errorf("the switchboard no longer accepts the secret it had")
			}
			added := strings.TrimPrefix(c.switchboardOut(), before)
			if lines := strings.Split(strings.TrimSpace(added), "\n"); strings.TrimSpace(added) == "" || len(lines) != 1 {
				t.Errorf("the switchboard printed %q on SIGHUP, want one line", added)
			}
		})
	}
}

func TestAStatusProbeHoldingASharedLockWhenTheSwitchboardStartsDoesNotMakeTheStartFail(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	if err := os.MkdirAll(c.dataDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	probe, err := os.OpenFile(filepath.Join(c.dataDir(), "switchboard.lock"), os.O_RDONLY|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	fd := int(probe.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_SH); err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(300 * time.Millisecond)
		syscall.Flock(fd, syscall.LOCK_UN)
	}()
	defer func() { <-released }()
	c.startSwitchboard()
}

func TestAReloadEndsObserverAndMachineAgentTokensAndKeepsTheSignaturesItRemembers(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	h := c.asHost()
	m := h.machine("linux-box")
	status, raw := bearerStatus(t, c.addr, "POST", "/v1/observers", c.savedSecret(), `{"kind":"tui"}`)
	var observer struct{ Token string }
	if status != http.StatusOK || json.Unmarshal([]byte(raw), &observer) != nil || observer.Token == "" {
		t.Fatalf("observer registration: %d %s", status, raw)
	}
	m.registerAgent("alpha")
	machineAgent := m.agentToken("alpha")
	signedBuild := map[string]string{"Authorization": machineAuth(m.key(), m.name, time.Now().Unix(), "GET", "/v1/build", nil)}
	if status := h.statusOnly("GET", "/v1/build", nil, signedBuild); status != http.StatusOK {
		t.Fatalf("the signed request before the rotation: %d", status)
	}
	if result := c.landline("server", "rotate-secret"); result.code != 0 {
		t.Fatalf("rotate-secret: %+v", result)
	}
	for name, token := range map[string]string{"an observer token": observer.Token, "the token of an agent registered with a machine key": machineAgent} {
		if status, _ := bearerStatus(t, c.addr, "GET", "/v1/agents", token, ""); status != http.StatusUnauthorized {
			t.Errorf("%s from before the rotation answered %d, want 401", name, status)
		}
	}
	if status := h.statusOnly("GET", "/v1/build", nil, signedBuild); !refused(status) {
		t.Errorf("a copy of a signed request accepted before the rotation, sent again inside the window after it, answered %d, so the reload forgot the signatures it remembers", status)
	}
}

func TestAReloadReadsOnlyTheSecretAndKeepsTheSignatureWindowItStartedWith(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.mustLandline("server", "up")
	waitPastTheStartPeriod()
	h := c.asHost()
	m := h.machine("linux-box")
	c.writeTable("signature-window = \"1s\"\n")
	if result := c.landline("server", "rotate-secret"); result.code != 0 {
		t.Fatalf("rotate-secret: %+v", result)
	}
	if status := h.signedStatus(m, time.Now().Unix()+3, "GET", "/v1/build", nil); status != http.StatusOK {
		t.Errorf("after a reload, a request timed 3 seconds ahead answered %d, so the switchboard took the 1s signature-window from config.toml instead of keeping its 5s", status)
	}
}

func deadPid(t *testing.T) int {
	t.Helper()
	gone := exec.Command("true")
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	return gone.Process.Pid
}

func TestRotateSecretWhoseSignalCannotBeSentSaysSoAtOnce(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	pid := deadPid(t)
	lockPath := c.writeDataFile("switchboard.lock", strconv.Itoa(pid))
	lock, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	old := c.savedSecret()
	began := time.Now()
	result := c.landline("server", "rotate-secret")
	took := time.Since(began)
	said := result.stdout + result.stderr
	c.checkTheNewSecretStaysSaved(old, said)
	if result.code != 1 || strings.Contains(said, "rotated the switchboard secret") || !strings.Contains(said, strconv.Itoa(pid)) {
		t.Errorf("rotate-secret with a locked pid that cannot be signalled: %d %q, want 1 and a message that names process %d", result.code, said, pid)
	}
	if took > 3*time.Second || strings.Contains(said, "still uses the old one") {
		t.Errorf("rotate-secret waited %v and said %q instead of saying at once that the signal failed", took, said)
	}
}

func TestRotateSecretWhoseStartFailsExitsAtOnceWithTheStartsFailure(t *testing.T) {
	t.Parallel()
	c := newConfiguredHome(t, "")
	c.writeTable("log = \"" + filepath.Join(c.blocker(), "log.jsonl") + "\"\n")
	old := c.savedSecret()
	began := time.Now()
	result := c.landline("server", "rotate-secret")
	took := time.Since(began)
	said := result.stdout + result.stderr
	c.checkTheNewSecretStaysSaved(old, said)
	if !strings.Contains(said, "saved") {
		t.Errorf("rotate-secret printed only what up printed, with no line that the new secret is saved: %q", said)
	}
	if result.code != 1 || !strings.Contains(said, "did not start") {
		t.Errorf("rotate-secret whose start fails: %d %q, want 1 and the start's failure", result.code, said)
	}
	if took > 3*time.Second || strings.Contains(said, "still uses the old one") {
		t.Errorf("rotate-secret waited %v and said %q instead of the start's failure alone", took, said)
	}
}

func (c *configuredHome) checkTheNewSecretStaysSaved(old, said string) {
	c.t.Helper()
	if fresh := c.savedSecret(); fresh == "" || fresh == old {
		c.t.Errorf("after the failed rotation config.toml does not hold a new secret: empty %v, same as the old %v", fresh == "", fresh == old)
	}
	if !strings.Contains(said, "config.toml") {
		c.t.Errorf("the message does not say that the new secret is saved in config.toml: %q", said)
	}
}
