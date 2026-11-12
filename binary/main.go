package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/8thlight-daveloper/landline/internal/cli/command"
	"github.com/8thlight-daveloper/landline/internal/hub/server"
	"github.com/8thlight-daveloper/landline/internal/log/store"
	mcp "github.com/8thlight-daveloper/landline/internal/mcp/server"
	"github.com/8thlight-daveloper/landline/internal/wake/codex"
)

// version is set at build time from VERSION. A plain go build reports dev.
var version = "dev"

func main() {
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	env := command.Env{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.Getenv,
		ConfigDir: filepath.Join(home, ".config", "landline"), Host: host,
		Banner: func() string { return bannerFor(os.Stderr) },
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hub":
			os.Exit(runHub(os.Args[2:], env, home, host))
		case "server":
			os.Exit(runServer(os.Args[2:], env, home, host))
		case "setup":
			os.Exit(runSetup(os.Args[2:]))
		case "claude":
			os.Exit(runClaude(os.Args[2:]))
		case "mcp":
			os.Exit(runMCP(os.Args[2:], env))
		case "version":
			fmt.Print(bannerFor(os.Stdout))
			fmt.Println("landline", version)
			return
		}
	}
	os.Exit(command.Run(os.Args[1:], env))
}

func flagValues(args []string, allowed ...string) (map[string]string, error) {
	values := map[string]string{}
	for i := 0; i < len(args); i += 2 {
		known := false
		for _, name := range allowed {
			known = known || args[i] == name
		}
		if !known || i+1 >= len(args) {
			return nil, fmt.Errorf("unknown or incomplete flag %s", args[i])
		}
		values[args[i]] = args[i+1]
	}
	return values, nil
}

func warnIfExposed(addr string) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		fmt.Fprintf(os.Stderr, "landline hub: warning: %s is not a loopback address. The hub speaks plain HTTP, so tokens and messages cross the network in the clear. Prefer localhost and a tunnel.\n", addr)
	}
}

func bannerFor(out *os.File) string {
	info, err := out.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return ""
	}
	color := os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	return command.Banner(color)
}

func notifyToStderr(line string) {
	plain := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf) {
			return ' '
		}
		return r
	}, line)
	fmt.Fprintln(os.Stderr, time.Now().Format(time.RFC3339), plain)
}

func runHub(args []string, env command.Env, home, host string) int {
	values, err := flagValues(args, "--addr", "--log", "--host", "--state", "--keepalive", "--delivery-timeout", "--retention")
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 2
	}
	defaultAddr, err := command.DefaultAddr(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 2
	}
	addr := or(values["--addr"], defaultAddr)
	timing, err := timingFrom(values)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 2
	}
	logPath := or(values["--log"], filepath.Join(home, ".local", "share", "landline", "log.jsonl"))
	statePath := or(values["--state"], filepath.Join(filepath.Dir(logPath), "state.json"))
	host = or(values["--host"], host)
	warnIfExposed(addr)
	log, err := store.Open(logPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 1
	}
	defer log.Close()
	secret := os.Getenv("LANDLINE_SECRET")
	if secret == "" {
		secret = command.LoadConfig(env).Secret
	}
	if secret == "" {
		fmt.Fprintln(os.Stderr, "landline hub: there is no hub secret. Run landline server up, or set LANDLINE_SECRET")
		return 1
	}
	hub, err := server.New(server.Config{
		Secret: secret, Host: host, StatePath: statePath,
		KeepaliveInterval: timing.keepalive, DeliveryTimeout: timing.deliveryTimeout, DisconnectedRetention: timing.retention,
		Notify: notifyToStderr,
	}, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 1
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 1
	}
	fmt.Fprint(os.Stderr, bannerFor(os.Stderr))
	fmt.Fprintf(os.Stderr, "landline hub listening on %s, host %q, log %s\n", listener.Addr(), host, logPath)
	srv := &http.Server{Handler: hub.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second}
	if err := srv.Serve(listener); err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 1
	}
	return 0
}

type hubTiming struct {
	keepalive, deliveryTimeout, retention time.Duration
}

func timingFrom(values map[string]string) (hubTiming, error) {
	var out hubTiming
	for flag, target := range map[string]*time.Duration{"--keepalive": &out.keepalive, "--delivery-timeout": &out.deliveryTimeout, "--retention": &out.retention} {
		raw, given := values[flag]
		if !given {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return hubTiming{}, fmt.Errorf("%s needs a positive duration such as 200ms or 15s, not %q", flag, raw)
		}
		*target = d
	}
	return out, nil
}

func gitRunner(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

func runMCP(args []string, env command.Env) int {
	channel, as := false, ""
	overrides := command.Identity{}
	targets := map[string]*string{
		"--as": &as, "--label": &overrides.Label, "--model": &overrides.Model, "--effort": &overrides.Effort,
		"--folder": &overrides.Folder, "--branch": &overrides.Branch,
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--channel" {
			channel = true
			continue
		}
		target, known := targets[args[i]]
		if !known || i+1 >= len(args) {
			fmt.Fprintf(os.Stderr, "landline mcp: unknown or incomplete flag %s\n", args[i])
			return 2
		}
		*target = args[i+1]
		i++
	}
	options := mcp.Options{Channel: channel, Waker: codex.New(), Notify: notifyToStderr}
	if as == "" {
		as = os.Getenv("LANDLINE_AS")
	}
	if as != "" {
		c, err := command.Connect(as, env)
		if err != nil {
			fmt.Fprintln(os.Stderr, "landline mcp:", err)
			return 1
		}
		if err := mcp.ServeWith(os.Stdin, os.Stdout, c, options); err != nil {
			fmt.Fprintln(os.Stderr, "landline mcp:", err)
			return 1
		}
		return 0
	}
	dir, _ := os.Getwd()
	session := command.NewSession(env, func() command.Identity { return command.DetectIdentity(env, dir, gitRunner) }, overrides)
	defer session.Close()
	options.OnClient = session.SetClient
	options.OnReady = func() {
		if _, err := session.Agents(); err != nil {
			notifyToStderr("landline mcp: " + err.Error())
		}
	}
	if err := mcp.ServeWith(os.Stdin, os.Stdout, session, options); err != nil {
		fmt.Fprintln(os.Stderr, "landline mcp:", err)
		return 1
	}
	return 0
}

func or(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
