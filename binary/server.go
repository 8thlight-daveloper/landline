package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/8thlight-daveloper/landline/internal/cli/command"
)

const serverUsage = `landline server <command>

  up [flags]        start the hub in the background
  run [flags]       run the hub in this terminal
                    Flags for up and run: --addr, --log, --state, --host, and the timing flags --keepalive D,
                    --delivery-timeout D and --retention D, which take durations such as 200ms or 15s.
                    Without --addr the hub listens on 127.0.0.1:8787, or on the port in LANDLINE_PORT
  down              stop the hub that "up" started
  restart [flags]   stop that hub and start it again with the flags given, so every agent reconnects
  status            say whether the hub is running
  log               print the hub's message log
  join <address>    use a hub on another machine, with its secret from LANDLINE_SECRET or standard input
  install [--load]  start the hub at login: a launchd agent on macOS and a systemd user unit on Linux
`

func runServer(args []string, env command.Env, home, host string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, serverUsage)
		return 2
	}
	dataDir := filepath.Join(home, ".local", "share", "landline")
	switch args[0] {
	case "run":
		return runHub(args[1:], env, home, host)
	case "up":
		return serverUp(args[1:], env, dataDir)
	case "down":
		return serverDown(dataDir)
	case "restart":
		return serverRestart(args[1:], env, dataDir)
	case "status":
		return serverStatus(env, dataDir)
	case "install":
		return serverInstall(args[1:], env, home, dataDir)
	case "log":
		return command.Run([]string{"log"}, env)
	case "join":
		return command.Run(append([]string{"join"}, args[1:]...), env)
	}
	fmt.Fprint(os.Stderr, serverUsage)
	return 2
}

func isLocalURL(text string) bool {
	parsed, err := url.Parse(text)
	if err != nil {
		return false
	}
	switch name := parsed.Hostname(); {
	case name == "localhost", name == "::1", strings.HasPrefix(name, "127."):
		return true
	}
	return false
}

func newSecret() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func ensureLocalConfig(env command.Env, addr string) error {
	config := command.LoadConfig(env)
	if config.URL != "" && !isLocalURL(config.URL) {
		return fmt.Errorf("this machine joined the hub at %s. Edit or remove %s to run a hub here instead", config.URL, filepath.Join(env.ConfigDir, "config.json"))
	}
	if config.Secret == "" {
		secret, err := newSecret()
		if err != nil {
			return err
		}
		config.Secret = secret
	}
	config.URL = "http://" + addr
	return command.SaveConfig(env, config)
}

func pidFile(dataDir string) string { return filepath.Join(dataDir, "hub.pid") }

func hubPID(dataDir string) (int, bool) {
	raw, err := os.ReadFile(pidFile(dataDir))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 || syscall.Kill(pid, 0) != nil {
		return 0, false
	}
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil || !strings.Contains(string(out), "landline") || !strings.Contains(string(out), "server run") {
		return 0, false
	}
	return pid, true
}

var hubFlags = []string{"--addr", "--log", "--state", "--host", "--keepalive", "--delivery-timeout", "--retention"}

func serverUp(args []string, env command.Env, dataDir string) int {
	values, err := flagValues(args, hubFlags...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline server up:", err)
		return 2
	}
	if pid, running := hubPID(dataDir); running {
		fmt.Printf("the hub is already running (pid %d) at %s\n", pid, command.HubURL(env))
		return 0
	}
	defaultAddr, err := command.DefaultAddr(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline server up:", err)
		return 2
	}
	if _, err := timingFrom(values); err != nil {
		fmt.Fprintln(os.Stderr, "landline server up:", err)
		return 2
	}
	addr := or(values["--addr"], defaultAddr)
	if err := ensureLocalConfig(env, addr); err != nil {
		fmt.Fprintln(os.Stderr, "landline server up:", err)
		return 1
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "landline server up:", err)
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline server up:", err)
		return 1
	}
	outPath := filepath.Join(dataDir, "hub.out")
	os.Remove(outPath)
	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline server up:", err)
		return 1
	}
	defer out.Close()
	child := exec.Command(self, append([]string{"server", "run"}, args...)...)
	child.Stdout, child.Stderr = out, out
	child.Stdin = nil
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "landline server up:", err)
		return 1
	}
	pid := child.Process.Pid
	os.WriteFile(pidFile(dataDir), []byte(strconv.Itoa(pid)), 0o600)
	child.Process.Release()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if raw, _ := os.ReadFile(outPath); strings.Contains(string(raw), "listening on") {
			fmt.Printf("the hub is running (pid %d) at %s. Its output is in %s\n", pid, "http://"+addr, outPath)
			return 0
		}
		time.Sleep(50 * time.Millisecond)
	}
	raw, _ := os.ReadFile(outPath)
	fmt.Fprintf(os.Stderr, "the hub did not start:\n%s", raw)
	return 1
}

func serverDown(dataDir string) int {
	pid, running := hubPID(dataDir)
	if !running {
		os.Remove(pidFile(dataDir))
		fmt.Println("no hub started by landline server up is running")
		return 0
	}
	syscall.Kill(pid, syscall.SIGTERM)
	for i := 0; i < 60; i++ {
		if syscall.Kill(pid, 0) != nil {
			os.Remove(pidFile(dataDir))
			fmt.Println("the hub stopped")
			return 0
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "the hub (pid %d) did not stop\n", pid)
	return 1
}

func serverRestart(args []string, env command.Env, dataDir string) int {
	values, err := flagValues(args, hubFlags...)
	if err == nil {
		_, err = timingFrom(values)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline server restart:", err)
		return 2
	}
	if code := serverDown(dataDir); code != 0 {
		return code
	}
	return serverUp(args, env, dataDir)
}

func serverStatus(env command.Env, dataDir string) int {
	if pid, running := hubPID(dataDir); running {
		fmt.Printf("the hub is running (pid %d) at %s\n", pid, command.HubURL(env))
		return 0
	}
	fmt.Printf("no hub started by landline server up is running. The saved address is %s\n", command.HubURL(env))
	return 0
}

func serverInstall(args []string, env command.Env, home, dataDir string) int {
	load := false
	for _, each := range args {
		if each != "--load" {
			fmt.Fprintf(os.Stderr, "landline server install: unknown flag %s\n", each)
			return 2
		}
		load = true
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline server install:", err)
		return 1
	}
	if strings.Contains(self, "/nix/store/") {
		fmt.Fprintln(os.Stderr, "landline server install: this binary is at a Nix store path that garbage collection can remove. Install it first with nix profile install, and run the installed landline")
		return 1
	}
	plan, err := command.PlanServiceInstall(runtime.GOOS, home, self, dataDir, os.Getuid())
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline server install:", err)
		return 1
	}
	if err := ensureLocalConfig(env, "127.0.0.1:8787"); err != nil {
		fmt.Fprintln(os.Stderr, "landline server install:", err)
		return 1
	}
	os.MkdirAll(dataDir, 0o700)
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "landline server install:", err)
		return 1
	}
	if err := os.WriteFile(plan.Path, []byte(plan.Content), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "landline server install:", err)
		return 1
	}
	fmt.Printf("wrote %s\n", plan.Path)
	if !load {
		fmt.Printf("To start it now and at every login, run:\n  %s\nOr run landline server install --load\n%s\n", strings.Join(plan.Load, " "), plan.Note)
		return 0
	}
	cmd := exec.Command(plan.Load[0], plan.Load[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "landline server install:", err)
		return 1
	}
	fmt.Println(plan.Note)
	return 0
}
