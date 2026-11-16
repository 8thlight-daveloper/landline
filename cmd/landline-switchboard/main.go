package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/8thlight-daveloper/landline/internal/cli/command"
	"github.com/8thlight-daveloper/landline/internal/log/store"
	"github.com/8thlight-daveloper/landline/internal/switchboard/server"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

const name = "landline-switchboard"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Println(command.VersionLine(name, version, commit, date))
		return 0
	}
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	env := command.Env{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.Getenv,
		ConfigDir: filepath.Join(home, ".config", "landline"), Home: home, Host: host,
	}
	values, err := command.FlagValues(args, command.SwitchboardFlags...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\nusage: %s [--addr A] [--log PATH] [--host H] [--keepalive D] [--delivery-timeout D] [--grace D]\n       %s version\n", name, err, name, name)
		return 2
	}
	defaultAddr, err := command.DefaultAddr(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		return 2
	}
	addr := command.Or(values["--addr"], defaultAddr)
	timing, err := command.TimingFrom(values)
	if err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		return 2
	}
	logPath := command.Or(values["--log"], filepath.Join(home, ".local", "share", "landline", "log.jsonl"))
	host = command.Or(values["--host"], host)
	warnIfExposed(addr)
	log, err := store.Open(logPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		return 1
	}
	defer log.Close()
	secret := os.Getenv("LANDLINE_SECRET")
	if secret == "" {
		secret = command.LoadConfig(env).Secret
	}
	if secret == "" {
		fmt.Fprintln(os.Stderr, name+": there is no secret. Run landline server up, or set LANDLINE_SECRET")
		return 1
	}
	switchboard, err := server.New(server.Config{
		Secret: secret, Host: host,
		KeepaliveInterval: timing.Keepalive, DeliveryTimeout: timing.DeliveryTimeout, Grace: timing.Grace,
		Notify: command.NotifyToStderr,
		Build:  server.BuildResponse{Version: version, Commit: commit, Date: date},
	}, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		return 1
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		return 1
	}
	fmt.Fprint(os.Stderr, command.BannerFor(os.Stderr))
	fmt.Fprintf(os.Stderr, "%s listening on %s, host %q, log %s\n", name, listener.Addr(), host, logPath)
	srv := &http.Server{Handler: switchboard.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second}
	if err := srv.Serve(listener); err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		return 1
	}
	return 0
}

func warnIfExposed(addr string) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		fmt.Fprintf(os.Stderr, "%s: warning: %s is not a loopback address. The switchboard speaks plain HTTP, so tokens and messages cross the network in the clear. Prefer localhost and a tunnel.\n", name, addr)
	}
}
