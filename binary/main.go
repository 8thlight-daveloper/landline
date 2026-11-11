package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/8thlight-daveloper/landline/internal/cli/command"
	client "github.com/8thlight-daveloper/landline/internal/client/http"
	"github.com/8thlight-daveloper/landline/internal/daemon/agentd"
	"github.com/8thlight-daveloper/landline/internal/hub/server"
	"github.com/8thlight-daveloper/landline/internal/log/store"
	mcp "github.com/8thlight-daveloper/landline/internal/mcp/server"
	"github.com/8thlight-daveloper/landline/internal/wake/codex"
)

const version = "0.1.0"

func main() {
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	env := command.Env{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.Getenv,
		ConfigDir: filepath.Join(home, ".config", "landline"), Host: host,
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hub":
			os.Exit(runHub(os.Args[2:], home, host))
		case "agentd":
			os.Exit(runDaemon(os.Args[2:], host))
		case "mcp":
			os.Exit(runMCP(os.Args[2:], env))
		case "version":
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

func notifyToStderr(line string) {
	plain := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf) {
			return ' '
		}
		return r
	}, line)
	fmt.Fprintln(os.Stderr, time.Now().Format(time.RFC3339), plain)
}

func runHub(args []string, home, host string) int {
	values, err := flagValues(args, "--addr", "--log", "--host", "--state", "--window")
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 2
	}
	addr := or(values["--addr"], "127.0.0.1:8787")
	logPath := or(values["--log"], filepath.Join(home, ".local", "share", "landline", "log.jsonl"))
	statePath := or(values["--state"], filepath.Join(filepath.Dir(logPath), "state.json"))
	host = or(values["--host"], host)
	window := time.Duration(0)
	if raw := values["--window"]; raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			fmt.Fprintln(os.Stderr, "landline hub: --window must be a duration such as 90s")
			return 2
		}
		window = parsed
	}
	warnIfExposed(addr)
	log, err := store.Open(logPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 1
	}
	defer log.Close()
	hub, err := server.New(server.Config{
		Secret: os.Getenv("LANDLINE_SECRET"), Host: host, StatePath: statePath, Window: window,
		Wakers: map[string]server.Waker{"codex": codex.New()},
		Notify: notifyToStderr,
	}, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "landline hub listening on %s, host %q, log %s\n", addr, host, logPath)
	srv := &http.Server{Addr: addr, Handler: hub.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 1
	}
	return 0
}

func runDaemon(args []string, host string) int {
	values, err := flagValues(args, "--host")
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline agentd:", err)
		return 2
	}
	url, secret := os.Getenv("LANDLINE_URL"), os.Getenv("LANDLINE_SECRET")
	if url == "" || secret == "" {
		fmt.Fprintln(os.Stderr, "landline agentd: set LANDLINE_URL and LANDLINE_SECRET")
		return 2
	}
	host = or(values["--host"], host)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(os.Stderr, "landline agentd waking agents on host %q for the hub at %s\n", host, url)
	err = agentd.Run(ctx, client.New(url, secret), host, map[string]agentd.Waker{"codex": codex.New()}, notifyToStderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline agentd:", err)
		return 1
	}
	return 0
}

func runMCP(args []string, env command.Env) int {
	values, err := flagValues(args, "--as")
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline mcp:", err)
		return 2
	}
	c, err := command.Connect(or(values["--as"], os.Getenv("LANDLINE_AS")), env)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline mcp:", err)
		return 1
	}
	if err := mcp.Serve(os.Stdin, os.Stdout, c); err != nil {
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
