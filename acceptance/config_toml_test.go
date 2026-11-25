package acceptance

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	unownedAddress   = "192.0.2.1"
	unresolvableHost = "tailnet-mac.invalid"
	joinedSecret     = "a-long-enough-secret-1"
)

func configDirOf(home string) string { return filepath.Join(home, ".config", "landline") }

func configTomlOf(home string) string { return filepath.Join(configDirOf(home), "config.toml") }

func writeConfigFile(t *testing.T, home, name, content string) {
	t.Helper()
	if err := tryWriteConfigFile(home, name, content); err != nil {
		t.Fatal(err)
	}
}

func tryWriteConfigFile(home, name, content string) error {
	if err := os.MkdirAll(configDirOf(home), 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDirOf(home), name), []byte(content), 0o600)
}

var (
	quotedLine   = regexp.MustCompile(`^([a-z_]+)\s*=\s*"([^"]*)"$`)
	integerLine  = regexp.MustCompile(`^([a-z_]+)\s*=\s*(-?[0-9]+)$`)
	secretValue  = regexp.MustCompile(`secret\s*=\s*"[^"]*"`)
	equalsSpaces = regexp.MustCompile(`\s*=\s*`)
)

func readTomlValues(t *testing.T, path string) (map[string]string, []string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v", err)
	}
	values := map[string]string{}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	for _, line := range lines {
		if found := quotedLine.FindStringSubmatch(line); found != nil && found[1] != "port" {
			values[found[1]] = found[2]
		}
		if found := integerLine.FindStringSubmatch(line); found != nil && found[1] == "port" {
			values[found[1]] = found[2]
		}
	}
	return values, lines
}

func scrub(text string) string { return secretValue.ReplaceAllString(text, `secret = "<redacted>"`) }

func normalized(line string) string { return equalsSpaces.ReplaceAllString(line, " = ") }

func hostAndPort(t *testing.T, address string) (string, string) {
	t.Helper()
	host, port, err := net.SplitHostPort(strings.TrimPrefix(address, "http://"))
	if err != nil {
		t.Fatalf("%q: %v", address, err)
	}
	return host, port
}

func tomlFor(host, port, secretText string) string {
	var lines []string
	if secretText != "" {
		lines = append(lines, `secret = "`+secretText+`"`)
	}
	if host != "" {
		lines = append(lines, `host = "`+host+`"`)
	}
	if port != "" {
		lines = append(lines, `port = `+port)
	}
	return strings.Join(lines, "\n") + "\n"
}

func exitCodeOf(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err == nil {
		return 0
	}
	return -1
}

func bindableOwnAddress(t *testing.T) string {
	t.Helper()
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() {
			if l, err := net.Listen("tcp", net.JoinHostPort(ip.IP.String(), "0")); err == nil {
				l.Close()
				return ip.IP.String()
			}
		}
	}
	t.Skip("this machine has no non-loopback IPv4 address that a listener can bind")
	return ""
}

func redacted(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		out[key] = value
		if key == "secret" {
			out[key] = "<" + strconv.Itoa(len(value)) + " characters>"
		}
	}
	return out
}

func otherOwnAddress() string {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() {
			return ip.IP.String()
		}
	}
	return ""
}

func listensAt(t *testing.T, host, port string) {
	t.Helper()
	connection, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		t.Errorf("nothing listens at %s: %v", net.JoinHostPort(host, port), err)
		return
	}
	connection.Close()
}

func serverEnv(t *testing.T) (string, string, []string) {
	t.Helper()
	bin, home := build(t), t.TempDir()
	env := isolatedEnv(home)
	t.Cleanup(func() { runIn(t, bin, env, home, "server", "down") })
	return bin, home, env
}

func withFreePort(t *testing.T, env []string) []string {
	t.Helper()
	return append(append([]string{}, env...), "LANDLINE_PORT="+freePort(t))
}

type clientCommand struct {
	name string
	args []string
}

var clientCommands = []clientCommand{{"agents, the screen reader", []string{"agents", "--json"}}, {"log, the command reader", []string{"log"}}}

func TestServerUpSavesConfigTomlWithTheSecretTheHostAndTheIntegerPortAndWritesNoConfigJSON(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	addr := freeAddr(t)
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", addr); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	info, statErr := os.Stat(configTomlOf(home))
	if statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config.toml %v %v", info, statErr)
	}
	values, _ := readTomlValues(t, configTomlOf(home))
	host, port := hostAndPort(t, addr)
	if values["host"] != host || values["port"] != port || len(values["secret"]) != 32 {
		t.Fatalf("config.toml holds %v, want a quoted host %s, an integer port %s, and a 32 character secret", redacted(values), host, port)
	}
	raw, _ := os.ReadFile(configTomlOf(home))
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "#") && !regexp.MustCompile(`^(secret|host|port)\s*=`).MatchString(trimmed) {
			t.Errorf("config.toml holds a key other than secret, host, and port, so it names a role or a url: %q", scrub(line))
		}
		if strings.Contains(line, "http") {
			t.Errorf("config.toml names a scheme: %q", scrub(line))
		}
	}
	if _, err := os.Stat(filepath.Join(configDirOf(home), "config.json")); err == nil {
		t.Error("config.json was written")
	}
	listensAt(t, host, port)
}

func TestServerUpWithNoFlagsListensOnTheSavedHostAndPort(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	host, port := hostAndPort(t, freeAddr(t))
	writeConfigFile(t, home, "config.toml", tomlFor(host, port, joinedSecret+joinedSecret))
	env = withFreePort(t, env)
	out, err := runIn(t, bin, env, home, "server", "up")
	if err != nil || !strings.Contains(out, "the switchboard is running") || !strings.Contains(out, "http://"+net.JoinHostPort(host, port)) {
		t.Fatalf("up: %v %s", err, out)
	}
	hubOut, _ := os.ReadFile(filepath.Join(home, ".local", "share", "landline", "switchboard.out"))
	if !strings.Contains(string(hubOut), "listening on "+net.JoinHostPort(host, port)) {
		t.Fatalf("switchboard.out: %s", hubOut)
	}
	listensAt(t, host, port)
}

func TestServerUpOnANonLoopbackAddressThisMachineOwnsStartsStopsAndStartsAgainFromTheSavedConfig(t *testing.T) {
	t.Parallel()
	own := bindableOwnAddress(t)
	bin, home, env := serverEnv(t)
	env = withFreePort(t, env)
	port := freePort(t)
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", net.JoinHostPort(own, port)); err != nil || !strings.Contains(out, "the switchboard is running") {
		t.Fatalf("first up: %v %s", err, out)
	}
	listensAt(t, own, port)
	if out, err := runIn(t, bin, env, home, "server", "down"); err != nil || !strings.Contains(out, "the switchboard stopped") {
		t.Fatalf("down: %v %s", err, out)
	}
	out, err := runIn(t, bin, env, home, "server", "up")
	if err != nil || !strings.Contains(out, "the switchboard is running") || strings.Contains(out, "joined") {
		t.Fatalf("up after down: %v %s", err, out)
	}
	listensAt(t, own, port)
	if out, err := runIn(t, bin, env, home, "server", "restart"); err != nil || !strings.Contains(out, "the switchboard is running") {
		t.Fatalf("restart: %v %s", err, out)
	}
	listensAt(t, own, port)
}

func TestServerUpOnAHostThisMachineDoesNotOwnFailsToBindNamesTheAddressAndSavesNoHostOrPort(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	port := freePort(t)
	out, err := runIn(t, bin, env, home, "server", "up", "--addr", net.JoinHostPort(unownedAddress, port))
	if exitCodeOf(err) != 1 || !strings.Contains(out, "cannot listen on "+unownedAddress+":"+port+":") {
		t.Fatalf("up: %v %s", err, out)
	}
	if status, _ := runIn(t, bin, env, home, "server", "status"); !saysNoSwitchboardRuns(status) {
		t.Errorf("status: %s", status)
	}
	if _, statErr := os.Stat(configTomlOf(home)); statErr == nil {
		if values, _ := readTomlValues(t, configTomlOf(home)); values["host"] != "" || values["port"] != "" {
			t.Errorf("a failed bind saved an address: %v", redacted(values))
		}
	}
}

func TestServerUpOnAPortThatIsTakenFailsToBindNamesTheAddressAndSavesNoHostOrPort(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	addr := holder.Addr().String()
	out, runErr := runIn(t, bin, env, home, "server", "up", "--addr", addr)
	if exitCodeOf(runErr) != 1 || !strings.Contains(out, "cannot listen on "+addr+":") {
		t.Fatalf("up: %v %s", runErr, out)
	}
	if status, _ := runIn(t, bin, env, home, "server", "status"); !saysNoSwitchboardRuns(status) {
		t.Errorf("status: %s", status)
	}
	if _, statErr := os.Stat(configTomlOf(home)); statErr == nil {
		if values, _ := readTomlValues(t, configTomlOf(home)); values["host"] != "" || values["port"] != "" {
			t.Errorf("a failed bind saved an address: %v", redacted(values))
		}
	}
}

func TestAFailedBindLeavesTheSavedHostAndPortUnchanged(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	host, port := hostAndPort(t, freeAddr(t))
	writeConfigFile(t, home, "config.toml", tomlFor(host, port, joinedSecret+joinedSecret))
	before, _ := os.ReadFile(configTomlOf(home))
	unowned := net.JoinHostPort(unownedAddress, freePort(t))
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", unowned); exitCodeOf(err) != 1 || !strings.Contains(out, "cannot listen on "+unowned) {
		t.Fatalf("up on an address the machine does not own: %v %s", err, out)
	}
	if after, _ := os.ReadFile(configTomlOf(home)); string(after) != string(before) {
		t.Errorf("config.toml changed:\n%s\nwas:\n%s", scrub(string(after)), scrub(string(before)))
	}
}

func TestAFailedRestartBindLeavesTheSavedHostAndPortUnchanged(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	addr := freeAddr(t)
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", addr); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	before, _ := os.ReadFile(configTomlOf(home))
	unowned := net.JoinHostPort(unownedAddress, freePort(t))
	if out, err := runIn(t, bin, env, home, "server", "restart", "--addr", unowned); exitCodeOf(err) != 1 || !strings.Contains(out, "cannot listen on "+unowned) {
		t.Fatalf("restart on an address the machine does not own: %v %s", err, out)
	}
	if after, _ := os.ReadFile(configTomlOf(home)); string(after) != string(before) {
		t.Errorf("config.toml changed:\n%s\nwas:\n%s", scrub(string(after)), scrub(string(before)))
	}
}

func TestTheSwitchboardShowsTheHostAsGivenTheAddressItBoundAndEveryOtherAddressTheNameResolvesTo(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	port := freePort(t)
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", "localhost:"+port); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	if values, _ := readTomlValues(t, configTomlOf(home)); values["host"] != "localhost" || values["port"] != port {
		t.Errorf("config.toml holds %v, want the host as given", redacted(values))
	}
	hubOut, _ := os.ReadFile(filepath.Join(home, ".local", "share", "landline", "switchboard.out"))
	var line string
	for _, each := range strings.Split(string(hubOut), "\n") {
		if strings.Contains(each, "listening on") {
			line = each
		}
	}
	if !strings.Contains(line, "localhost:"+port) {
		t.Fatalf("the listening line does not show the host as given:\n%s", hubOut)
	}
	ips, _ := net.LookupIP("localhost")
	if len(ips) == 0 {
		t.Fatal("localhost does not resolve on this machine")
	}
	first := net.JoinHostPort(ips[0].String(), port)
	if !strings.Contains(line, first) {
		t.Errorf("localhost resolves to %v in that order, and the listening line does not show %s, the first address, as the one bound:\n%s", ips, first, line)
	}
	for _, ip := range ips {
		if !strings.Contains(line, ip.String()) {
			t.Errorf("the listening line leaves out %s, which localhost resolves to:\n%s", ip, line)
		}
	}
}

func TestTheHostFlagIsTheAgentLabelAndNeverTheSavedHost(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	host, port := hostAndPort(t, freeAddr(t))
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", net.JoinHostPort(host, port), "--host", "label-machine"); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	if values, _ := readTomlValues(t, configTomlOf(home)); values["host"] != host || values["port"] != port {
		t.Errorf("config.toml holds %v, want host %s and port %s", redacted(values), host, port)
	}
	hubOut, _ := os.ReadFile(filepath.Join(home, ".local", "share", "landline", "switchboard.out"))
	if !strings.Contains(string(hubOut), `host "label-machine"`) {
		t.Errorf("--host no longer labels the switchboard:\n%s", hubOut)
	}
}

func TestServerJoinSavesTheHostThePortAndTheSecretWithNoSchemeWhateverFormTheAddressTakes(t *testing.T) {
	t.Parallel()
	bin := build(t)
	for _, form := range []string{unresolvableHost + ":8787", "http://" + unresolvableHost + ":8787"} {
		home := t.TempDir()
		env := append(isolatedEnv(home), "LANDLINE_SECRET="+joinedSecret)
		out, err := runIn(t, bin, env, home, "server", "join", form)
		if err != nil {
			t.Fatalf("join %s: %v %s", form, err, out)
		}
		info, statErr := os.Stat(configTomlOf(home))
		values, _ := readTomlValues(t, configTomlOf(home))
		if statErr != nil || info.Mode().Perm() != 0o600 || values["host"] != unresolvableHost || values["port"] != "8787" || values["secret"] != joinedSecret {
			t.Errorf("join %s saved %v %v %v", form, info, statErr, redacted(values))
		}
		raw, _ := os.ReadFile(configTomlOf(home))
		if strings.Contains(string(raw), "http") {
			t.Errorf("join %s saved a scheme:\n%s", form, scrub(string(raw)))
		}
	}
}

func TestServerJoinTakesTheSecretFromStandardInputWhenTheEnvironmentHasNone(t *testing.T) {
	t.Parallel()
	bin, home := build(t), t.TempDir()
	cmd := exec.Command(bin, "server", "join", unresolvableHost+":8787")
	cmd.Env, cmd.Dir, cmd.Stdin = isolatedEnv(home), home, strings.NewReader(joinedSecret+"\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("join: %v %s", err, out)
	}
	if values, _ := readTomlValues(t, configTomlOf(home)); values["secret"] != joinedSecret || values["host"] != unresolvableHost {
		t.Errorf("join saved %v", redacted(values))
	}
}

var onlyHTTP = regexp.MustCompile(`(?is)http\W+only|only\W+(speaks\W+)?http\b`)

func TestServerJoinRefusesAnHTTPSAddressSaysLandlineSpeaksHTTPOnlyAndSavesNothing(t *testing.T) {
	t.Parallel()
	bin, home := build(t), t.TempDir()
	env := append(isolatedEnv(home), "LANDLINE_SECRET="+joinedSecret)
	out, err := runIn(t, bin, env, home, "server", "join", "https://"+unresolvableHost+":8787")
	if err == nil || !onlyHTTP.MatchString(out) {
		t.Fatalf("join: %v %s", err, out)
	}
	for _, name := range []string{"config.toml", "config.json"} {
		if _, statErr := os.Stat(filepath.Join(configDirOf(home), name)); statErr == nil {
			t.Errorf("%s was written", name)
		}
	}
}

func TestServerUpOnAMachineThatJoinedAnotherSwitchboardFailsToBindAndSaysConfigTomlNamesSwitchboardElsewhere(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	joinEnv := append(append([]string{}, env...), "LANDLINE_SECRET="+joinedSecret)
	if out, err := runIn(t, bin, joinEnv, home, "server", "join", unresolvableHost+":8787"); err != nil {
		t.Fatalf("join: %v %s", err, out)
	}
	before, _ := os.ReadFile(configTomlOf(home))
	env = withFreePort(t, env)
	out, err := runIn(t, bin, env, home, "server", "up")
	if exitCodeOf(err) != 1 || !strings.Contains(out, "cannot listen on "+unresolvableHost+":8787") || !strings.Contains(out, "config.toml") || strings.Contains(out, "this machine joined the switchboard") {
		t.Fatalf("up after join: %v %s", err, out)
	}
	if after, _ := os.ReadFile(configTomlOf(home)); string(after) != string(before) {
		t.Errorf("the failed up changed config.toml:\n%s\nwas:\n%s", scrub(string(after)), scrub(string(before)))
	}
}

func TestRotateSecretChangesOnlyTheSecretLineKeepsCommentsBlankLinesAndUnknownKeysAndResetsTheModeTo0600(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	host, port := hostAndPort(t, freeAddr(t))
	old := strings.Repeat("a", 32)
	content := "# the switchboard on this machine\n\nsecret = \"" + old + "\"\nhost = \"" + host + "\"\nport = " + port + "\n# a note for later\nextra = \"kept\"\n"
	writeConfigFile(t, home, "config.toml", content)
	if err := os.Chmod(configTomlOf(home), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runIn(t, bin, env, home, "server", "rotate-secret"); err != nil || !strings.Contains(out, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret: %v %s", err, out)
	}
	raw, _ := os.ReadFile(configTomlOf(home))
	got := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	want := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("the line count changed:\n%s", scrub(string(raw)))
	}
	for i := range want {
		if i != 2 && normalized(got[i]) != normalized(want[i]) {
			t.Errorf("line %d is %q, want %q", i+1, got[i], want[i])
		}
	}
	if found := quotedLine.FindStringSubmatch(got[2]); found == nil || found[1] != "secret" || len(found[2]) != 32 || found[2] == old {
		t.Errorf("line 3 does not hold a new 32 character secret")
	}
	if info, _ := os.Stat(configTomlOf(home)); info == nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info)
	}
	listensAt(t, host, port)
}

func TestRotateSecretRefusesWhenTheSavedHostIsNotThisMachinesAndChangesNothing(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	env = withFreePort(t, env)
	writeConfigFile(t, home, "config.toml", tomlFor(unresolvableHost, "8787", joinedSecret+joinedSecret))
	before, _ := os.ReadFile(configTomlOf(home))
	out, err := runIn(t, bin, env, home, "server", "rotate-secret")
	if exitCodeOf(err) != 1 || strings.Contains(out, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret: %v %s", err, out)
	}
	if after, _ := os.ReadFile(configTomlOf(home)); string(after) != string(before) {
		t.Errorf("config.toml changed:\n%s\nwas:\n%s", scrub(string(after)), scrub(string(before)))
	}
	if status, _ := runIn(t, bin, env, home, "server", "status"); !saysNoSwitchboardRuns(status) {
		t.Errorf("a switchboard is running: %s", status)
	}
}

func TestRotateSecretOnAnAddressThisMachineOwnsThatIsNotLoopbackRotatesAndStartsOne(t *testing.T) {
	t.Parallel()
	own := bindableOwnAddress(t)
	bin, home, env := serverEnv(t)
	port := freePort(t)
	writeConfigFile(t, home, "config.toml", tomlFor(own, port, strings.Repeat("e", 32)))
	if out, err := runIn(t, bin, env, home, "server", "rotate-secret"); err != nil || !strings.Contains(out, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret: %v %s", err, out)
	}
	if values, _ := readTomlValues(t, configTomlOf(home)); len(values["secret"]) != 32 || values["secret"] == strings.Repeat("e", 32) {
		t.Errorf("the secret was not replaced: %v", redacted(values))
	}
	listensAt(t, own, port)
}

func ownNamedHost(t *testing.T) string {
	t.Helper()
	name, err := os.Hostname()
	if err != nil || name == "" {
		t.Skip("this machine has no host name")
	}
	own := map[string]bool{}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok {
			own[ip.IP.String()] = true
		}
	}
	ips, _ := net.LookupIP(name)
	resolvesHere := false
	for _, ip := range ips {
		resolvesHere = resolvesHere || own[ip.String()]
	}
	if !resolvesHere {
		t.Skip("this machine's host name does not resolve to one of its own addresses")
	}
	if l, err := net.Listen("tcp", net.JoinHostPort(name, "0")); err != nil {
		t.Skip("a listener cannot bind this machine's host name")
	} else {
		l.Close()
	}
	return name
}

func TestRotateSecretRunsWhenTheSavedHostIsANameThatResolvesToAnAddressOfThisMachine(t *testing.T) {
	t.Parallel()
	name := ownNamedHost(t)
	bin, home, env := serverEnv(t)
	old, port := strings.Repeat("f", 32), freePort(t)
	writeConfigFile(t, home, "config.toml", tomlFor(name, port, old))
	if out, err := runIn(t, bin, env, home, "server", "rotate-secret"); err != nil || !strings.Contains(out, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret: %v %s", err, out)
	}
	if values, _ := readTomlValues(t, configTomlOf(home)); len(values["secret"]) != 32 || values["secret"] == old {
		t.Errorf("the secret was not replaced: %v", redacted(values))
	}
	listensAt(t, name, port)
}

func TestServerUpChangesTheSavedHostAndPortInPlaceAndKeepsCommentsAndUnknownKeys(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	oldHost, oldPort := hostAndPort(t, freeAddr(t))
	_, newPort := hostAndPort(t, freeAddr(t))
	content := "# mine\nsecret = \"" + strings.Repeat("a", 32) + "\"\nhost = \"" + oldHost + "\"\nport = " + oldPort + "\n\n# last\nextra = \"kept\"\n"
	writeConfigFile(t, home, "config.toml", content)
	if err := os.Chmod(configTomlOf(home), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", "localhost:"+newPort); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	if info, _ := os.Stat(configTomlOf(home)); info == nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600 after up rewrote the file", info)
	}
	raw, _ := os.ReadFile(configTomlOf(home))
	var got []string
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		got = append(got, normalized(line))
	}
	want := []string{"# mine", `secret = "` + strings.Repeat("a", 32) + `"`, `host = "localhost"`, "port = " + newPort, "", "# last", `extra = "kept"`}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("config.toml:\n%s\nwant:\n%s", scrub(strings.Join(got, "\n")), scrub(strings.Join(want, "\n")))
	}
	listensAt(t, "localhost", newPort)
}

func TestServerUpAppendsAMissingKeyOnceAndKeepsTheOtherLines(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	_, port := hostAndPort(t, freeAddr(t))
	secretLine := `secret = "` + strings.Repeat("a", 32) + `"`
	writeConfigFile(t, home, "config.toml", "# only a secret\n"+secretLine+"\n")
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", "127.0.0.1:"+port); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	raw, _ := os.ReadFile(configTomlOf(home))
	var lines []string
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		lines = append(lines, normalized(line))
	}
	if len(lines) != 4 || lines[0] != "# only a secret" || lines[1] != secretLine {
		t.Fatalf("config.toml:\n%s", scrub(string(raw)))
	}
	if rest := lines[2] + "\n" + lines[3]; rest != `host = "127.0.0.1"`+"\n"+"port = "+port && rest != "port = "+port+"\n"+`host = "127.0.0.1"` {
		t.Fatalf("the appended lines are wrong:\n%s", scrub(string(raw)))
	}
	listensAt(t, "127.0.0.1", port)
}

func TestServerRestartWithAnAddressSavesTheNewPortAndListensThere(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", freeAddr(t)); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	_, port := hostAndPort(t, freeAddr(t))
	if out, err := runIn(t, bin, env, home, "server", "restart", "--addr", "127.0.0.1:"+port); err != nil {
		t.Fatalf("restart: %v %s", err, out)
	}
	if values, _ := readTomlValues(t, configTomlOf(home)); values["host"] != "127.0.0.1" || values["port"] != port {
		t.Errorf("config.toml holds %v, want host 127.0.0.1 and port %s", redacted(values), port)
	}
	listensAt(t, "127.0.0.1", port)
}

func TestServerUpWithNoConfigOrAPartialConfigSavesTheLoopbackDefaultAndTheLandlinePort(t *testing.T) {
	t.Parallel()
	for name, existing := range map[string]string{"no file": "", "only a secret": `secret = "` + strings.Repeat("a", 32) + `"` + "\n"} {
		bin, home, env := serverEnv(t)
		port := freePort(t)
		env = append(env, "LANDLINE_PORT="+port)
		if existing != "" {
			writeConfigFile(t, home, "config.toml", existing)
		}
		if out, err := runIn(t, bin, env, home, "server", "up"); err != nil {
			t.Fatalf("%s: up: %v %s", name, err, out)
		}
		values, _ := readTomlValues(t, configTomlOf(home))
		if values["host"] != "127.0.0.1" || values["port"] != port || len(values["secret"]) != 32 || (existing != "" && values["secret"] != strings.Repeat("a", 32)) {
			t.Errorf("%s: config.toml holds %v, want host 127.0.0.1, port %s, and the secret kept", name, redacted(values), port)
		}
		listensAt(t, "127.0.0.1", port)
		if other := otherOwnAddress(); other != "" {
			if connection, err := net.DialTimeout("tcp", net.JoinHostPort(other, port), 2*time.Second); err == nil {
				connection.Close()
				t.Errorf("%s: the switchboard also listens on %s, so the loopback default is not kept", name, other)
			}
		}
		runIn(t, bin, env, home, "server", "down")
	}
}

func TestAnIPv6HostIsSavedWithoutBracketsAndEveryClientReachesItWithBrackets(t *testing.T) {
	t.Parallel()
	l, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skip("this machine has no IPv6 loopback")
	}
	l.Close()
	bin, home, env := serverEnv(t)
	port := freePort(t)
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", "[::1]:"+port); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	if values, _ := readTomlValues(t, configTomlOf(home)); values["host"] != "::1" || values["port"] != port {
		t.Fatalf("config.toml holds %v, want host ::1 and port %s", redacted(values), port)
	}
	for _, command := range clientCommands {
		if result := runBin(t, bin, env, command.args...); result.code != 0 {
			t.Errorf("%s: %d %q %q", command.name, result.code, result.stdout, result.stderr)
		}
	}
	joinHome := t.TempDir()
	joinEnv := append(isolatedEnv(joinHome), "LANDLINE_SECRET="+joinedSecret)
	if out, err := runIn(t, bin, joinEnv, joinHome, "server", "join", "[::1]:"+port); err != nil {
		t.Fatalf("join: %v %s", err, out)
	}
	if values, _ := readTomlValues(t, configTomlOf(joinHome)); values["host"] != "::1" || values["port"] != port {
		t.Errorf("join saved %v, want host ::1 and port %s", redacted(values), port)
	}
}

func TestEveryClientReadsHostPortAndSecretFromConfigTomlWithCommentsBlankLinesAndUnknownKeys(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("alpha")
	host, port := hostAndPort(t, w.addr)
	for _, command := range clientCommands {
		home := t.TempDir()
		writeConfigFile(t, home, "config.toml", "# written by hand\n\nextra = \"ignored\"\nsecret = \""+secret+"\"\nhost = \""+host+"\"\n\n# the port\nport = "+port+"\n")
		if result := runBin(t, w.bin, isolatedEnv(home), command.args...); result.code != 0 || strings.Contains(result.stderr, "config.toml") {
			t.Errorf("%s: %d %q %q", command.name, result.code, result.stdout, result.stderr)
		}
	}
}

func TestEveryClientWithNoFileOrAFileWithoutHostAndPortReachesLoopbackAtTheLandlinePort(t *testing.T) {
	t.Parallel()
	w := start(t)
	_, port := hostAndPort(t, w.addr)
	for _, command := range clientCommands {
		for name, existing := range map[string]string{"no file": "", "a file with only a secret": tomlFor("", "", secret)} {
			home := t.TempDir()
			env := append(isolatedEnv(home), "LANDLINE_PORT="+port)
			if existing == "" {
				env = append(env, "LANDLINE_SECRET="+secret)
			} else {
				writeConfigFile(t, home, "config.toml", existing)
			}
			if result := runBin(t, w.bin, env, command.args...); result.code != 0 {
				t.Errorf("%s, %s: %d %q %q", command.name, name, result.code, result.stdout, result.stderr)
			}
		}
	}
}

func TestEveryClientWithNoFileAndNoSecretStillSaysThereIsNoSecret(t *testing.T) {
	t.Parallel()
	bin := build(t)
	for _, command := range clientCommands {
		home := t.TempDir()
		result := runBin(t, bin, append(isolatedEnv(home), "LANDLINE_PORT="+freePort(t)), command.args...)
		if combined := result.stdout + result.stderr; result.code == 0 || !strings.Contains(strings.ToLower(combined), "secret") {
			t.Errorf("%s: %d %q", command.name, result.code, combined)
		}
	}
}

func TestTheEnvironmentWinsOverConfigTomlForTheAddressAndForTheSecretInEveryClient(t *testing.T) {
	t.Parallel()
	w := start(t)
	host, port := hostAndPort(t, w.addr)
	for _, command := range clientCommands {
		home := t.TempDir()
		writeConfigFile(t, home, "config.toml", tomlFor(host, port, strings.Repeat("z", 32)))
		withSecret := append(isolatedEnv(home), "LANDLINE_SECRET="+secret)
		if result := runBin(t, w.bin, withSecret, command.args...); result.code != 0 {
			t.Errorf("%s: LANDLINE_SECRET did not win over the file: %d %q", command.name, result.code, result.stderr)
		}
		writeConfigFile(t, home, "config.toml", tomlFor(host, port, secret))
		withDeadURL := append(isolatedEnv(home), "LANDLINE_URL="+deadAddress(t))
		if result := runBin(t, w.bin, withDeadURL, command.args...); result.code == 0 {
			t.Errorf("%s: LANDLINE_URL did not win over the file: %q", command.name, result.stdout)
		}
	}
}

func TestEveryClientRefusesAMalformedConfigTomlNamingTheFileAndNeverEchoingTheLine(t *testing.T) {
	t.Parallel()
	bin := build(t)
	cases := []struct {
		name, content string
		line, echo    string
	}{
		{"a line that is not toml", "# first\nhost = \"127.0.0.1\"\nthis line holds hunter2 and is not toml\nport = %s\n", "line 3", "hunter2"},
		{"an unquoted string", "secret = \"" + joinedSecret + "\"\nhost = 127.0.0.1\nport = %s\n", "line 2", "host = 127.0.0.1"},
		{"a string port", "secret = \"" + joinedSecret + "\"\nhost = \"127.0.0.1\"\nport = \"%s\"\n", "line 3", `port = "`},
		{"an integer host", "secret = \"" + joinedSecret + "\"\nhost = 5\nport = %s\n", "line 2", "host = 5"},
		{"an integer secret", "secret = 12345\nhost = \"127.0.0.1\"\nport = %s\n", "line 1", "12345"},
	}
	for _, command := range clientCommands {
		for _, each := range cases {
			home := t.TempDir()
			writeConfigFile(t, home, "config.toml", strings.ReplaceAll(each.content, "%s", freePort(t)))
			result := runBin(t, bin, isolatedEnv(home), command.args...)
			combined := result.stdout + result.stderr
			if result.code == 0 || !strings.Contains(combined, "config.toml") || !strings.Contains(combined, each.line) || strings.Contains(withoutPaths(combined), each.echo) {
				t.Errorf("%s, %s: %d %q", command.name, each.name, result.code, combined)
			}
		}
	}
}

func TestEveryLineThatIsNotABlankACommentOrAKeyWithAStringOrIntegerIsRefusedAtItsLine(t *testing.T) {
	t.Parallel()
	bin := build(t)
	tokens := map[string]string{`port = 8787 # note`: "# note", `[a.b]`: "[a.b]", `list = [1, 2]`: "[1, 2]", `host = '127.0.0.1'`: "'127.0.0.1'", `port = 87.87`: "87.87"}
	for bad, token := range tokens {
		for _, command := range clientCommands {
			home := t.TempDir()
			writeConfigFile(t, home, "config.toml", "secret = \""+joinedSecret+"\"\nhost = \"127.0.0.1\"\n"+bad+"\n")
			result := runBin(t, bin, isolatedEnv(home), command.args...)
			if combined := result.stdout + result.stderr; result.code == 0 || !strings.Contains(combined, "config.toml") || !strings.Contains(combined, "line 3") || strings.Contains(combined, bad) || strings.Contains(combined, token) {
				t.Errorf("%s, %q: %d %q", command.name, bad, result.code, combined)
			}
		}
	}
}

func TestAnOldConfigJSONCarriesOverAsHostAndPortAndTheFirstWriteSavesConfigTomlAndRemovesIt(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	addr := freeAddr(t)
	env = withFreePort(t, env)
	old := strings.Repeat("b", 32)
	writeConfigFile(t, home, "config.json", `{"url":"http://`+addr+`","secret":"`+old+`"}`)
	if out, err := runIn(t, bin, env, home, "server", "up"); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	host, port := hostAndPort(t, addr)
	values, _ := readTomlValues(t, configTomlOf(home))
	if values["secret"] != old || values["host"] != host || values["port"] != port {
		t.Errorf("config.toml holds %v, want the old secret, host %s, and port %s", redacted(values), host, port)
	}
	if info, _ := os.Stat(configTomlOf(home)); info == nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info)
	}
	if _, err := os.Stat(filepath.Join(configDirOf(home), "config.json")); err == nil {
		t.Error("config.json was not removed after config.toml was saved")
	}
	listensAt(t, host, port)
}

func TestAnOldConfigJSONIsReadByEveryClientWhenNoConfigTomlExists(t *testing.T) {
	t.Parallel()
	w := start(t)
	for _, command := range clientCommands {
		home := t.TempDir()
		writeConfigFile(t, home, "config.json", `{"url":"`+w.url+`","secret":"`+secret+`"}`)
		if result := runBin(t, w.bin, isolatedEnv(home), command.args...); result.code != 0 {
			t.Errorf("%s: %d %q %q", command.name, result.code, result.stdout, result.stderr)
		}
		if _, err := os.Stat(configTomlOf(home)); err == nil {
			t.Errorf("%s: a read created config.toml", command.name)
		}
	}
}

func TestConfigTomlWinsInEveryClientWhenBothFilesExist(t *testing.T) {
	t.Parallel()
	w := start(t)
	host, port := hostAndPort(t, w.addr)
	for _, command := range clientCommands {
		home := t.TempDir()
		writeConfigFile(t, home, "config.toml", tomlFor(host, port, secret))
		writeConfigFile(t, home, "config.json", `{"url":"`+deadAddress(t)+`","secret":"`+strings.Repeat("c", 32)+`"}`)
		if result := runBin(t, w.bin, isolatedEnv(home), command.args...); result.code != 0 {
			t.Errorf("%s: %d %q %q", command.name, result.code, result.stdout, result.stderr)
		}
	}
}

func TestAnOldConfigJSONWithAnHTTPSURLIsAnErrorInEveryClientAndNoReadCreatesConfigToml(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.register("alpha")
	for _, command := range clientCommands {
		home := t.TempDir()
		writeConfigFile(t, home, "config.json", `{"url":"https://`+w.addr+`","secret":"`+secret+`"}`)
		if result := runBin(t, w.bin, isolatedEnv(home), command.args...); result.code == 0 {
			t.Errorf("%s: %d %q", command.name, result.code, result.stdout)
		}
		if _, err := os.Stat(configTomlOf(home)); err == nil {
			t.Errorf("%s: a read created config.toml", command.name)
		}
	}
}

func TestRotateSecretRefusesAnAddressLiteralThatThisMachineDoesNotOwn(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	env = withFreePort(t, env)
	writeConfigFile(t, home, "config.toml", tomlFor(unownedAddress, freePort(t), joinedSecret+joinedSecret))
	before, _ := os.ReadFile(configTomlOf(home))
	out, err := runIn(t, bin, env, home, "server", "rotate-secret")
	if exitCodeOf(err) != 1 || strings.Contains(out, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret: %v %s", err, out)
	}
	if after, _ := os.ReadFile(configTomlOf(home)); string(after) != string(before) {
		t.Errorf("config.toml changed:\n%s\nwas:\n%s", scrub(string(after)), scrub(string(before)))
	}
}

func TestARefusedRotateSecretLeavesARunningSwitchboardRunningAndTheSecretUnchanged(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	addr := freeAddr(t)
	host, port := hostAndPort(t, addr)
	if out, err := runIn(t, bin, env, home, "server", "up", "--addr", addr); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	values, _ := readTomlValues(t, configTomlOf(home))
	writeConfigFile(t, home, "config.toml", tomlFor(unownedAddress, port, values["secret"]))
	before, _ := os.ReadFile(configTomlOf(home))
	if out, err := runIn(t, bin, env, home, "server", "rotate-secret"); exitCodeOf(err) != 1 || strings.Contains(out, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret: %v %s", err, out)
	}
	if after, _ := os.ReadFile(configTomlOf(home)); string(after) != string(before) {
		t.Errorf("config.toml changed:\n%s\nwas:\n%s", scrub(string(after)), scrub(string(before)))
	}
	if status, _ := runIn(t, bin, env, home, "server", "status"); !strings.Contains(status, "is running") {
		t.Errorf("the refused rotation stopped the switchboard: %s", status)
	}
	listensAt(t, host, port)
}

func TestServerJoinSavesConfigTomlRemovesAnOldConfigJSONAndKeepsTheCommentsOfAnExistingConfigToml(t *testing.T) {
	t.Parallel()
	bin := build(t)
	fromJSON := t.TempDir()
	writeConfigFile(t, fromJSON, "config.json", `{"url":"http://`+unresolvableHost+`:9000","secret":"`+strings.Repeat("b", 32)+`"}`)
	env := append(isolatedEnv(fromJSON), "LANDLINE_SECRET="+joinedSecret)
	if out, err := runIn(t, bin, env, fromJSON, "server", "join", unresolvableHost+":8787"); err != nil {
		t.Fatalf("join: %v %s", err, out)
	}
	if values, _ := readTomlValues(t, configTomlOf(fromJSON)); values["host"] != unresolvableHost || values["port"] != "8787" || values["secret"] != joinedSecret {
		t.Errorf("join saved %v", redacted(values))
	}
	if _, err := os.Stat(filepath.Join(configDirOf(fromJSON), "config.json")); err == nil {
		t.Error("config.json was not removed after join saved config.toml")
	}
	withComments := t.TempDir()
	writeConfigFile(t, withComments, "config.toml", "# mine\nsecret = \""+strings.Repeat("a", 32)+"\"\nhost = \"127.0.0.1\"\nport = 9\n\n# last\nextra = \"kept\"\n")
	env = append(isolatedEnv(withComments), "LANDLINE_SECRET="+joinedSecret)
	if out, err := runIn(t, bin, env, withComments, "server", "join", unresolvableHost+":8787"); err != nil {
		t.Fatalf("join onto a file with comments: %v %s", err, out)
	}
	raw, _ := os.ReadFile(configTomlOf(withComments))
	var got []string
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		got = append(got, normalized(line))
	}
	want := []string{"# mine", `secret = "` + joinedSecret + `"`, `host = "` + unresolvableHost + `"`, "port = 8787", "", "# last", `extra = "kept"`}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("config.toml:\n%s\nwant:\n%s", scrub(strings.Join(got, "\n")), scrub(strings.Join(want, "\n")))
	}
}

func switchboardAccepts(t *testing.T, addr, secretText string) bool {
	t.Helper()
	request, _ := http.NewRequest("POST", "http://"+addr+"/v1/observers", strings.NewReader(`{"kind":"tui"}`))
	request.Header.Set("Authorization", "Bearer "+secretText)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func TestAnOldConfigJSONSecretThatConfigTomlCannotHoldStaysInConfigJSONAndUpNamesTheProblem(t *testing.T) {
	t.Parallel()
	for _, old := range []string{`alpha\bravo-0123456789abcdef`, `alpha"bravo-0123456789abcdef`} {
		bin, home, env := serverEnv(t)
		addr := freeAddr(t)
		raw, _ := json.Marshal(map[string]string{"url": "http://" + addr, "secret": old})
		writeConfigFile(t, home, "config.json", string(raw))
		out, _ := runIn(t, bin, env, home, "server", "up")
		if !strings.Contains(out, "config.toml") || !strings.Contains(out, "secret") {
			t.Errorf("up with the secret %q: %q, want a line that says the secret cannot be saved in config.toml", old, out)
		}
		if got := readFileOrFail(t, filepath.Join(configDirOf(home), "config.json")); got != string(raw) {
			t.Errorf("config.json changed or was removed: %q", got)
		}
		if exists(configTomlOf(home)) {
			for _, line := range strings.Split(readFileOrFail(t, configTomlOf(home)), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "secret") {
					t.Errorf("config.toml got a secret line for a value it cannot hold: %q", scrub(line))
				}
			}
		}
		if !switchboardAccepts(t, addr, old) {
			t.Errorf("the switchboard does not accept the exact secret %q of config.json", old)
		}
		runIn(t, bin, env, home, "server", "down")
	}
}

func TestAnOldConfigJSONHexSecretCarriesOverAndConfigJSONGoesOnlyOnceConfigTomlReadsItBack(t *testing.T) {
	t.Parallel()
	bin, home, env := serverEnv(t)
	addr := freeAddr(t)
	old := "0123456789abcdef0123456789abcdef"
	writeConfigFile(t, home, "config.json", `{"url":"http://`+addr+`","secret":"`+old+`"}`)
	if out, err := runIn(t, bin, env, home, "server", "up"); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	if values, _ := readTomlValues(t, configTomlOf(home)); values["secret"] != old {
		t.Errorf("config.toml does not hold the secret of config.json: %v", redacted(values))
	}
	if exists(filepath.Join(configDirOf(home), "config.json")) {
		t.Errorf("config.json stays though config.toml holds its secret")
	}
	if !switchboardAccepts(t, addr, old) {
		t.Errorf("the switchboard does not accept the carried-over secret")
	}
}
