package command

import (
	"bytes"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestRotateSecretTakesNoFlagAndAnyFlagExitsTwoAndChangesNothing(t *testing.T) {
	t.Setenv("PATH", "")
	for _, args := range [][]string{
		{"--no-such-flag", "x"}, {"--addr", "127.0.0.1:8787"}, {"--grace", "2m"}, {"--keepalive", "15s"},
		{"--delivery-timeout", "20s"}, {"--invite-lifetime", "5m"}, {"--signature-window", "5s"},
		{"--log", "/tmp/landline-log.jsonl"}, {"--host", "alpha-host"},
	} {
		r, file := rotate(t, "http://127.0.0.1:8787", nil, args...)
		if r.code != 2 || !strings.Contains(file, savedSecret) || r.out.Len() != 0 {
			t.Errorf("rotate-secret %v: code %d, stdout %q, stderr %q, secret kept %v", args, r.code, r.out.String(), r.err.String(), strings.Contains(file, savedSecret))
		}
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
