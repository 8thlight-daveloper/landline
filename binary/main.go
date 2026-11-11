package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/8thlight-daveloper/landline/internal/cli/command"
	"github.com/8thlight-daveloper/landline/internal/hub/server"
	"github.com/8thlight-daveloper/landline/internal/log/store"
	"github.com/8thlight-daveloper/landline/internal/wake/codex"
)

func main() {
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	if len(os.Args) > 1 && os.Args[1] == "hub" {
		os.Exit(runHub(os.Args[2:], home, host))
	}
	os.Exit(command.Run(os.Args[1:], command.Env{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.Getenv,
		ConfigDir: filepath.Join(home, ".config", "landline"), Host: host,
	}))
}

func runHub(args []string, home, host string) int {
	addr, logPath := "127.0.0.1:8787", filepath.Join(home, ".local", "share", "landline", "log.jsonl")
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--addr":
			addr = args[i+1]
		case "--log":
			logPath = args[i+1]
		case "--host":
			host = args[i+1]
		default:
			fmt.Fprintf(os.Stderr, "landline hub: unknown flag %s\n", args[i])
			return 2
		}
	}
	log, err := store.Open(logPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 1
	}
	defer log.Close()
	hub, err := server.New(server.Config{
		Secret: os.Getenv("LANDLINE_SECRET"), Host: host,
		Wakers: map[string]server.Waker{"codex": codex.New()},
		Notify: func(line string) { fmt.Fprintln(os.Stderr, time.Now().Format(time.RFC3339), line) },
	}, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "landline hub listening on %s, host %q, log %s\n", addr, host, logPath)
	srv := &http.Server{Addr: addr, Handler: hub.Handler(), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, "landline hub:", err)
		return 1
	}
	return 0
}
