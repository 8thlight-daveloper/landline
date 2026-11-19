package command

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const serverUsage = `landline server <command>

  up [flags]        start the switchboard in the background
                    Flags: --addr, --log, --host, and the timing flags --keepalive D, --delivery-timeout D and
                    --grace D, --invite-lifetime D and --signature-window D, which take durations such as 200ms or 15s.
                    --grace is how long the switchboard keeps an agent after it was last seen, 2 minutes by default.
                    Without --addr it listens on the host and port saved in config.toml. Without a saved host it uses
                    127.0.0.1, and without a saved port the port in LANDLINE_PORT or 8787. It saves the address after it listens
  down              stop the switchboard that "up" started
  restart [flags]   stop that switchboard and start it again with the flags given. It keeps no agents, so each one registers again
  rotate-secret [flags]   replace the secret and restart the switchboard with the flags given. The old secret stops working
                    at once and every agent must register again
  invite --name N   make a code that lets another machine join as N, and print the join command for it
  machines          list the machines that joined, with a fingerprint of each key
  revoke N|--all    remove a machine, or every machine, and end its tokens and streams
  status            say whether the switchboard is running
  log               print the message log
  join <host:port> [--code C]   use a switchboard on another machine. With a code, it saves a key of its own. Without one,
                    it saves the secret from LANDLINE_SECRET or standard input
  install [--load]  start the switchboard at login: a launchd agent on macOS and a systemd user unit on Linux
`

func dataDirectory(env Env) string { return filepath.Join(env.Home, ".local", "share", "landline") }

func runServer(args []string, env Env) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, serverUsage)
		return 2
	}
	dataDir := dataDirectory(env)
	switch args[0] {
	case "up":
		return serverUp(args[1:], env, dataDir)
	case "down":
		return serverDown(env, dataDir)
	case "restart":
		return serverRestart(args[1:], env, dataDir)
	case "rotate-secret":
		return serverRotateSecret(args[1:], env, dataDir)
	case "status":
		return serverStatus(env, dataDir)
	case "invite":
		return serverInvite(args[1:], env)
	case "machines":
		return serverMachines(args[1:], env)
	case "revoke":
		return serverRevoke(args[1:], env)
	case "install":
		return serverInstall(args[1:], env, dataDir)
	case "log":
		return Run([]string{"log"}, env)
	case "join":
		return Run(append([]string{"join"}, args[1:]...), env)
	}
	fmt.Fprint(env.Stderr, serverUsage)
	return 2
}

func newSecret() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func ensureSecret(env Env, config Config) (Config, error) {
	if config.Secret != "" {
		return config, nil
	}
	secret, err := newSecret()
	if err != nil {
		return config, err
	}
	config.Secret = secret
	return config, SaveConfig(env, Config{Secret: secret})
}

func pidFile(dataDir string) string { return filepath.Join(dataDir, "switchboard.pid") }

func switchboardPID(dataDir string) (int, bool) {
	raw, err := os.ReadFile(pidFile(dataDir))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 || syscall.Kill(pid, 0) != nil {
		return 0, false
	}
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil || !strings.Contains(string(out), SwitchboardBinaryName) {
		return 0, false
	}
	return pid, true
}

func checkSwitchboardFlags(args []string) error {
	values, err := FlagValues(args, SwitchboardFlags...)
	if err == nil {
		_, err = TimingFrom(values)
	}
	return err
}

func listenTarget(addr string, config Config, getenv func(string) string) (string, int, error) {
	if addr == "" {
		return config.Listen(getenv)
	}
	host, rawPort, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, fmt.Errorf("the address %q needs a port, as in 127.0.0.1:8787. Pass --addr host:port", addr)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 || host == "" {
		return "", 0, fmt.Errorf("the address %q must be host:port with a port from 1 to 65535", addr)
	}
	return host, port, nil
}

func serverUp(args []string, env Env, dataDir string) int {
	values, err := FlagValues(args, SwitchboardFlags...)
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline server up:", err)
		return 2
	}
	if pid, running := switchboardPID(dataDir); running {
		fmt.Fprintf(env.Stdout, "the switchboard is already running (pid %d) at %s\n", pid, HubURL(env))
		return 0
	}
	config, err := ReadConfig(env)
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline server up:", err)
		return 1
	}
	host, port, err := listenTarget(values["--addr"], config, env.Getenv)
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline server up:", err)
		return 2
	}
	if _, err := TimingFrom(values); err != nil {
		fmt.Fprintln(env.Stderr, "landline server up:", err)
		return 2
	}
	switchboard, err := findBinary(SwitchboardBinaryName)
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline server up:", err)
		return 1
	}
	if _, err := ensureSecret(env, config); err != nil {
		fmt.Fprintln(env.Stderr, "landline server up:", err)
		return 1
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		fmt.Fprintln(env.Stderr, "landline server up:", err)
		return 1
	}
	outPath := filepath.Join(dataDir, "switchboard.out")
	os.Remove(outPath)
	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline server up:", err)
		return 1
	}
	defer out.Close()
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	if values["--addr"] == "" {
		args = append([]string{"--addr", addr}, args...)
	}
	child := exec.Command(switchboard, args...)
	child.Stdout, child.Stderr = out, out
	child.Stdin = nil
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		fmt.Fprintln(env.Stderr, "landline server up:", err)
		return 1
	}
	pid := child.Process.Pid
	os.WriteFile(pidFile(dataDir), []byte(strconv.Itoa(pid)), 0o600)
	exited := make(chan struct{})
	go func() {
		child.Wait()
		close(exited)
	}()
	deadline := time.Now().Add(20 * time.Second)
	for {
		raw, _ := os.ReadFile(outPath)
		if strings.Contains(string(raw), "listening on") {
			return upListening(env, Config{Host: host, Port: port}, pid, addr, outPath)
		}
		select {
		case <-exited:
			return upFailed(env, dataDir, outPath, values["--addr"] == "")
		default:
		}
		if time.Now().After(deadline) {
			return upFailed(env, dataDir, outPath, values["--addr"] == "")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func upListening(env Env, address Config, pid int, addr, outPath string) int {
	if err := SaveConfig(env, address); err != nil {
		fmt.Fprintf(env.Stderr, "landline server up: the switchboard is running (pid %d) at http://%s and its address was not saved: %v\n", pid, addr, err)
		return 1
	}
	fmt.Fprintf(env.Stdout, "the switchboard is running (pid %d) at http://%s. Its output is in %s\n", pid, addr, outPath)
	return 0
}

func upFailed(env Env, dataDir, outPath string, fromSavedAddress bool) int {
	raw, _ := os.ReadFile(outPath)
	if _, running := switchboardPID(dataDir); !running {
		os.Remove(pidFile(dataDir))
	}
	fmt.Fprintf(env.Stderr, "the switchboard did not start:\n%s", raw)
	if fromSavedAddress && strings.Contains(string(raw), "cannot listen on") {
		fmt.Fprintf(env.Stderr, "The address comes from %s. If it names a switchboard on another machine, edit host and port there to run one here, or pass --addr host:port.\n", configPath(env))
	}
	return 1
}

func serverDown(env Env, dataDir string) int {
	pid, running := switchboardPID(dataDir)
	if !running {
		os.Remove(pidFile(dataDir))
		fmt.Fprintln(env.Stdout, "no switchboard started by landline server up is running")
		return 0
	}
	syscall.Kill(pid, syscall.SIGTERM)
	for i := 0; i < 60; i++ {
		if syscall.Kill(pid, 0) != nil {
			os.Remove(pidFile(dataDir))
			fmt.Fprintln(env.Stdout, "the switchboard stopped")
			return 0
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Fprintf(env.Stderr, "the switchboard (pid %d) did not stop\n", pid)
	return 1
}

func serverRestart(args []string, env Env, dataDir string) int {
	if err := checkSwitchboardFlags(args); err != nil {
		fmt.Fprintln(env.Stderr, "landline server restart:", err)
		return 2
	}
	if _, err := findBinary(SwitchboardBinaryName); err != nil {
		fmt.Fprintln(env.Stderr, "landline server restart:", err)
		return 1
	}
	if code := serverDown(env, dataDir); code != 0 {
		return code
	}
	return serverUp(args, env, dataDir)
}

func serverStatus(env Env, dataDir string) int {
	if pid, running := switchboardPID(dataDir); running {
		fmt.Fprintf(env.Stdout, "the switchboard is running (pid %d) at %s\n", pid, HubURL(env))
		return 0
	}
	fmt.Fprintf(env.Stdout, "no switchboard started by landline server up is running. The saved address is %s\n", HubURL(env))
	return 0
}

func serverInstall(args []string, env Env, dataDir string) int {
	load := false
	for _, each := range args {
		if each != "--load" {
			fmt.Fprintf(env.Stderr, "landline server install: unknown flag %s\n", each)
			return 2
		}
		load = true
	}
	switchboard, err := findBinary(SwitchboardBinaryName)
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline server install:", err)
		return 1
	}
	if strings.Contains(switchboard, nixStoreMarker) {
		fmt.Fprintf(env.Stderr, "landline server install: %s is at a Nix store path that garbage collection can remove. Install it first with nix profile install\n", SwitchboardBinaryName)
		return 1
	}
	plan, err := PlanServiceInstall(runtime.GOOS, env.Home, switchboard, dataDir, os.Getuid())
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline server install:", err)
		return 1
	}
	config, err := ReadConfig(env)
	if err == nil {
		_, err = ensureSecret(env, config)
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline server install:", err)
		return 1
	}
	os.MkdirAll(dataDir, 0o700)
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0o755); err != nil {
		fmt.Fprintln(env.Stderr, "landline server install:", err)
		return 1
	}
	if err := os.WriteFile(plan.Path, []byte(plan.Content), 0o644); err != nil {
		fmt.Fprintln(env.Stderr, "landline server install:", err)
		return 1
	}
	fmt.Fprintf(env.Stdout, "wrote %s\n", plan.Path)
	if !load {
		fmt.Fprintf(env.Stdout, "To start it now and at every login, run:\n  %s\nOr run landline server install --load\n%s\n", strings.Join(plan.Load, " "), plan.Note)
		return 0
	}
	cmd := exec.Command(plan.Load[0], plan.Load[1:]...)
	cmd.Stdout, cmd.Stderr = env.Stdout, env.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(env.Stderr, "landline server install:", err)
		return 1
	}
	fmt.Fprintln(env.Stdout, plan.Note)
	return 0
}
