package command

import (
	"bytes"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

const savedSecret = "saved-secret-saved-secret-0001"

func configFor(rawURL string) Config {
	parsed, _ := url.Parse(rawURL)
	port, _ := strconv.Atoi(parsed.Port())
	return Config{Host: parsed.Hostname(), Port: port, Secret: savedSecret}
}

func rotate(t *testing.T, savedURL string, environment map[string]string, args ...string) (run, string) {
	t.Helper()
	dir := t.TempDir()
	if err := SaveConfig(Env{ConfigDir: dir}, configFor(savedURL)); err != nil {
		t.Fatal(err)
	}
	var r run
	env := Env{
		Stdin: strings.NewReader(""), Stdout: &r.out, Stderr: &r.err, ConfigDir: dir, Home: t.TempDir(),
		Getenv: func(key string) string { return environment[key] },
	}
	r.code = Run(append([]string{"server", "rotate-secret"}, args...), env)
	raw, err := os.ReadFile(configPath(Env{ConfigDir: dir}))
	if err != nil {
		t.Fatal(err)
	}
	return r, string(raw)
}

func TestRotateSecretRefusesASavedHostThatIsNotThisMachinesAndChangesNothing(t *testing.T) {
	t.Setenv("PATH", "")
	r, file := rotate(t, "http://10.0.0.5:8787", nil)
	if r.code != 1 || !strings.Contains(r.err.String(), "the saved switchboard host 10.0.0.5 is not an address of this machine") || r.out.Len() != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", r.code, r.out.String(), r.err.String())
	}
	if !strings.Contains(file, savedSecret) {
		t.Fatalf("the saved secret changed: %s", file)
	}
}

func TestRotateSecretRefusesWhenTheEnvironmentSetsTheSecretAndChangesNothing(t *testing.T) {
	t.Setenv("PATH", "")
	r, file := rotate(t, "http://127.0.0.1:8787", map[string]string{"LANDLINE_SECRET": "environment-secret-0000000"})
	if r.code != 1 || !strings.Contains(r.err.String(), "LANDLINE_SECRET is set in the environment") || r.out.Len() != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", r.code, r.out.String(), r.err.String())
	}
	if !strings.Contains(file, savedSecret) {
		t.Fatalf("the saved secret changed: %s", file)
	}
	if strings.Contains(r.err.String(), "environment-secret") || strings.Contains(r.err.String(), savedSecret) {
		t.Fatalf("the message names a secret: %q", r.err.String())
	}
}

func TestRotateSecretIgnoresTheSavedSecretWhenCheckingTheEnvironment(t *testing.T) {
	t.Setenv("PATH", "")
	r, file := rotate(t, "http://127.0.0.1:8787", nil)
	if r.code != 1 || strings.Contains(r.err.String(), "LANDLINE_SECRET") || !strings.Contains(r.err.String(), SwitchboardBinaryName+" is not installed") {
		t.Fatalf("code %d, stderr %q", r.code, r.err.String())
	}
	if !strings.Contains(file, savedSecret) {
		t.Fatalf("the saved secret changed: %s", file)
	}
}

func TestRotateSecretWithABadFlagExitsTwoAndChangesNothing(t *testing.T) {
	t.Setenv("PATH", "")
	r, file := rotate(t, "http://127.0.0.1:8787", nil, "--no-such-flag", "x")
	if r.code != 2 || !strings.Contains(file, savedSecret) {
		t.Fatalf("code %d, stderr %q, file %s", r.code, r.err.String(), file)
	}
}

func TestProcessEnvironmentSurvivesALaterWrapOfTheConfig(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	env := Env{Stdout: &out, ConfigDir: t.TempDir(), Getenv: func(string) string { return "" }}
	if err := SaveConfig(env, configFor("http://127.0.0.1:8787")); err != nil {
		t.Fatal(err)
	}
	wrapped := withConfig(withConfig(env))
	if wrapped.Getenv("LANDLINE_SECRET") != savedSecret || wrapped.processEnvironment("LANDLINE_SECRET") != "" {
		t.Fatalf("saved %q, process %q", wrapped.Getenv("LANDLINE_SECRET"), wrapped.processEnvironment("LANDLINE_SECRET"))
	}
}

const stubSwitchboard = `#!/usr/bin/env perl
use strict;
use warnings;
use Fcntl qw(:flock O_RDWR O_CREAT);
use File::Path qw(make_path);
$| = 1;
sub record { my ($name, $text) = @_; open(my $out, ">", "$ENV{STUB_RECORD}/$name") or die; print $out $text; close $out; }
record("pid", "$$\n");
record("args", join("", map { "$_\n" } @ARGV));
open(my $config, "<", $ENV{STUB_CONFIG}) or die; local $/; record("config-at-start", <$config> // ""); close $config;
my $mode = $ENV{STUB_MODE} // "";
if ($mode eq "fail") { print "stub refuses to start\n"; exit 1; }
make_path($ENV{STUB_DATA}, { mode => 0700 });
my $lockPath = "$ENV{STUB_DATA}/switchboard.lock";
sysopen(my $lock, $lockPath, O_RDWR | O_CREAT, 0600) or die "landline-switchboard: $lockPath: $!\n";
if (!flock($lock, LOCK_EX | LOCK_NB)) { print "landline-switchboard: another switchboard is running on this machine and holds $lockPath\n"; exit 1; }
truncate($lock, 0); syswrite($lock, "$$");
if ($mode ne "hang") { print "landline-switchboard: listening on stub\n"; }
if ($mode eq "stubborn") { $SIG{TERM} = "IGNORE"; sleep 5; exit 0; }
$SIG{TERM} = sub { exit 0 };
sleep 30;
exit 0;
`

type rotation struct {
	env    Env
	out    *bytes.Buffer
	err    *bytes.Buffer
	record string
}

func newRotation(t *testing.T, savedURL string) *rotation {
	t.Helper()
	bin, record, home, dir := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, SwitchboardBinaryName), []byte(stubSwitchboard), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STUB_RECORD", record)
	t.Setenv("STUB_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("STUB_MODE", "")
	t.Setenv("STUB_DATA", dataDirectory(Env{Home: home}))
	t.Setenv("HOME", home)
	if err := SaveConfig(Env{ConfigDir: dir}, configFor(savedURL)); err != nil {
		t.Fatal(err)
	}
	r := &rotation{out: &bytes.Buffer{}, err: &bytes.Buffer{}, record: record}
	r.env = Env{
		Stdin: strings.NewReader(""), Stdout: r.out, Stderr: r.err, ConfigDir: dir, Home: home,
		Getenv: func(string) string { return "" },
	}
	t.Cleanup(func() {
		Run([]string{"server", "down"}, Env{Stdout: io.Discard, Stderr: io.Discard, ConfigDir: dir, Home: home})
	})
	return r
}

func (r *rotation) run(args ...string) int {
	r.out.Reset()
	r.err.Reset()
	return Run(append([]string{"server"}, args...), r.env)
}

func (r *rotation) pid(t *testing.T) int {
	t.Helper()
	pid, running := switchboardPID(dataDirectory(r.env))
	if !running {
		t.Fatal("no switchboard is running")
	}
	return pid
}

func (r *rotation) running() bool {
	_, running := switchboardPID(dataDirectory(r.env))
	return running
}

func (r *rotation) config() Config {
	config, err := ReadConfig(r.env)
	if err != nil {
		panic(err)
	}
	return config
}

func (r *rotation) address() string {
	config := r.config()
	return net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
}

func (r *rotation) recorded(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(r.record, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func listening(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener.Addr().String()
}

func TestRotateSecretStopsSavesAndStartsAndPrintsOneLineWithNoSecret(t *testing.T) {
	address := listening(t)
	r := newRotation(t, "http://"+address)
	if code := r.run("up", "--addr", address); code != 0 {
		t.Fatalf("up: %d %s", code, r.err.String())
	}
	before := r.pid(t)
	if code := r.run("rotate-secret", "--grace", "2m"); code != 0 {
		t.Fatalf("rotate: %d %s", code, r.err.String())
	}
	if r.out.String() != rotatedMessage+"\n" || r.err.Len() != 0 {
		t.Fatalf("stdout %q, stderr %q", r.out.String(), r.err.String())
	}
	config := r.config()
	if config.Secret == savedSecret || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(config.Secret) || r.address() != address {
		t.Fatalf("config after rotation: %+v", config)
	}
	if strings.Contains(r.out.String()+r.err.String(), config.Secret) || strings.Contains(r.out.String()+r.err.String(), savedSecret) {
		t.Fatal("the output names a secret")
	}
	if info, err := os.Stat(configPath(r.env)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode: %v %v", info, err)
	}
	if after := r.pid(t); after == before {
		t.Fatalf("the switchboard pid stayed %d", after)
	}
	if !strings.Contains(r.recorded(t, "config-at-start"), config.Secret) {
		t.Fatal("the new switchboard started before the new secret was saved")
	}
	if args := strings.Fields(r.recorded(t, "args")); !reflect.DeepEqual(args, []string{"--addr", address, "--grace", "2m"}) {
		t.Fatalf("switchboard arguments: %v", args)
	}
}

func TestRotateSecretWithNoSwitchboardRunningStartsOne(t *testing.T) {
	r := newRotation(t, "http://127.0.0.1:48612")
	if code := r.run("rotate-secret"); code != 0 || r.out.String() != rotatedMessage+"\n" || !r.running() {
		t.Fatalf("code %d, stdout %q, stderr %q, running %v", code, r.out.String(), r.err.String(), r.running())
	}
}

func TestRotateSecretKeepsTheSavedAddressWhenNoAddrFlagIsGiven(t *testing.T) {
	r := newRotation(t, "http://127.0.0.1:48613")
	r.env.Getenv = func(key string) string {
		if key == "LANDLINE_PORT" {
			return "48614"
		}
		return ""
	}
	if code := r.run("rotate-secret"); code != 0 {
		t.Fatalf("rotate: %d %s", code, r.err.String())
	}
	if r.address() != "127.0.0.1:48613" || !reflect.DeepEqual(strings.Fields(r.recorded(t, "args")), []string{"--addr", "127.0.0.1:48613"}) {
		t.Fatalf("config %+v, arguments %q", r.config(), r.recorded(t, "args"))
	}
}

func TestRotateSecretRefusesWhenSomethingElseAnswersAtTheSavedAddressAndChangesNothing(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	r := newRotation(t, "http://"+listener.Addr().String())
	if code := r.run("rotate-secret"); code != 1 || !strings.Contains(r.err.String(), "something answers at "+listener.Addr().String()) || r.out.Len() != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, r.out.String(), r.err.String())
	}
	if r.config().Secret != savedSecret {
		t.Fatal("the saved secret changed")
	}
}

func TestRotateSecretChecksTheSavedHostBeforeTheEnvironmentAndLeavesARunningSwitchboardAlone(t *testing.T) {
	r := newRotation(t, "http://127.0.0.1:48615")
	if code := r.run("up", "--addr", "127.0.0.1:48615"); code != 0 {
		t.Fatalf("up: %d %s", code, r.err.String())
	}
	before := r.pid(t)
	environment := func(string) string { return "set-in-the-environment" }
	if err := SaveConfig(r.env, configFor("http://10.0.0.5:8787")); err != nil {
		t.Fatal(err)
	}
	r.env.Getenv = environment
	if code := r.run("rotate-secret"); code != 1 || !strings.Contains(r.err.String(), "not an address of this machine") || strings.Contains(r.err.String(), "LANDLINE_SECRET") {
		t.Fatalf("code %d, stderr %q", code, r.err.String())
	}
	if r.pid(t) != before || r.config().Secret != savedSecret {
		t.Fatal("a refusal changed the switchboard or the secret")
	}
}

func TestRotateSecretWhoseStartFailsLeavesTheNewSecretSavedAndSaysToRunServerUp(t *testing.T) {
	r := newRotation(t, "http://127.0.0.1:48616")
	if code := r.run("up", "--addr", "127.0.0.1:48616"); code != 0 {
		t.Fatalf("up: %d %s", code, r.err.String())
	}
	t.Setenv("STUB_MODE", "fail")
	code := r.run("rotate-secret")
	if code != 1 || !strings.Contains(r.err.String(), "the new secret is saved and the switchboard is stopped. Run landline server up to start it") || r.out.Len() != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, r.out.String(), r.err.String())
	}
	if r.config().Secret == savedSecret || r.running() {
		t.Fatalf("secret changed %v, running %v", r.config().Secret != savedSecret, r.running())
	}
	if strings.Contains(r.err.String(), r.config().Secret) {
		t.Fatal("the message names the new secret")
	}
}

func TestRotateSecretWhoseSaveFailsLeavesTheSwitchboardStoppedAndTheOldSecretInTheFile(t *testing.T) {
	r := newRotation(t, "http://127.0.0.1:48617")
	if code := r.run("up", "--addr", "127.0.0.1:48617"); code != 0 {
		t.Fatalf("up: %d %s", code, r.err.String())
	}
	if err := os.Chmod(r.env.ConfigDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(r.env.ConfigDir, 0o700) })
	if code := r.run("rotate-secret"); code != 1 || !strings.Contains(r.err.String(), "keeps the old secret") || !strings.Contains(r.err.String(), "stopped") || r.out.Len() != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, r.out.String(), r.err.String())
	}
	if r.config().Secret != savedSecret || r.running() {
		t.Fatalf("the secret changed or the switchboard runs: %v", r.running())
	}
}

func TestRotateSecretRefusesWhenSomethingElseAnswersAtTheSavedAddressEvenWithAnotherAddrFlag(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	spare, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	spareAddress := spare.Addr().String()
	spare.Close()
	r := newRotation(t, "http://"+listener.Addr().String())
	if code := r.run("rotate-secret", "--addr", spareAddress); code != 1 || !strings.Contains(r.err.String(), "something answers at "+listener.Addr().String()) || r.out.Len() != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, r.out.String(), r.err.String())
	}
	if r.config().Secret != savedSecret || r.running() {
		t.Fatal("the secret changed or a switchboard started")
	}
}

func TestRotateSecretStopsANewSwitchboardThatIsAliveButNeverSaysItListens(t *testing.T) {
	r := newRotation(t, "http://127.0.0.1:48618")
	t.Setenv("STUB_MODE", "hang")
	code := r.run("rotate-secret")
	if code != 1 || !strings.Contains(r.err.String(), "the new secret is saved and the switchboard is stopped") || r.out.Len() != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, r.out.String(), r.err.String())
	}
	pid, err := strconv.Atoi(strings.TrimSpace(r.recorded(t, "pid")))
	if err != nil {
		t.Fatal(err)
	}
	if syscall.Kill(pid, 0) == nil {
		t.Fatalf("the new switchboard, pid %d, is still alive", pid)
	}
}

func TestRotateSecretRefusesWhenSomethingElseAnswersAtTheAddrFlagAndTheSavedAddressIsFree(t *testing.T) {
	occupied := listening(t)
	r := newRotation(t, "http://127.0.0.1:48619")
	if code := r.run("rotate-secret", "--addr", occupied); code != 1 || !strings.Contains(r.err.String(), "something answers at "+occupied) || r.out.Len() != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, r.out.String(), r.err.String())
	}
	if r.config().Secret != savedSecret || r.running() {
		t.Fatal("the secret changed or a switchboard started")
	}
}

func TestRotateSecretWhoseOldSwitchboardWillNotStopFailsAndKeepsTheOldSecret(t *testing.T) {
	r := newRotation(t, "http://127.0.0.1:48620")
	t.Setenv("STUB_MODE", "stubborn")
	if code := r.run("up", "--addr", "127.0.0.1:48620"); code != 0 {
		t.Fatalf("up: %d %s", code, r.err.String())
	}
	if code := r.run("rotate-secret"); code != 1 || !strings.Contains(r.err.String(), "did not stop") || r.out.Len() != 0 {
		t.Fatalf("code %d, stdout %q, stderr %q", code, r.out.String(), r.err.String())
	}
	if r.config().Secret != savedSecret {
		t.Fatal("the secret changed while the old switchboard still ran")
	}
}

func TestRotateSecretRefusesAnAddrFlagWithNoPortBeforeStoppingAnything(t *testing.T) {
	r := newRotation(t, "http://127.0.0.1:48621")
	if code := r.run("up", "--addr", "127.0.0.1:48621"); code != 0 {
		t.Fatalf("up: %d %s", code, r.err.String())
	}
	before := r.pid(t)
	if code := r.run("rotate-secret", "--addr", "localhost"); code == 0 || !strings.Contains(r.err.String(), "needs a port") {
		t.Fatalf("code %d, stderr %q", code, r.err.String())
	}
	if r.pid(t) != before || r.config().Secret != savedSecret {
		t.Fatal("a refusal changed the switchboard or the secret")
	}
}

func TestAHostIsThisMachinesWhenItIsLocalhostALoopbackIPOrResolvesToAnInterfaceAddress(t *testing.T) {
	t.Parallel()
	for host, want := range map[string]bool{
		"127.0.0.1": true, "127.9.9.9": true, "localhost": true, "LOCALHOST": true, "::1": true,
		"192.0.2.1": false, "tailnet-mac.invalid": false, "": false,
	} {
		if got := IsThisMachines(host); got != want {
			t.Errorf("IsThisMachines(%q) = %v, want %v", host, got, want)
		}
	}
	var own string
	addrs, _ := net.InterfaceAddrs()
	for _, each := range addrs {
		if ip, ok := each.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() {
			own = ip.IP.String()
		}
	}
	if own != "" && !IsThisMachines(own) {
		t.Errorf("IsThisMachines(%q) is false for an address of an interface", own)
	}
}
