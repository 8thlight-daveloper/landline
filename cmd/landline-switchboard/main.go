package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		fmt.Fprintf(os.Stderr, "%s: %s\nusage: %s [--addr A] [--log PATH] [--host H] [--keepalive D] [--delivery-timeout D] [--grace D] [--invite-lifetime D] [--signature-window D]\n       %s version\n", name, command.Printable(err.Error()), name, name)
		return 2
	}
	saved, err := command.ReadConfig(env)
	if err != nil {
		say(err)
		return 1
	}
	savedHost, savedPort, err := saved.Listen(os.Getenv)
	if err != nil {
		say(err)
		return 1
	}
	addr := command.Or(values["--addr"], net.JoinHostPort(savedHost, strconv.Itoa(savedPort)))
	if _, err := command.TimingFrom(values); err != nil {
		say(err)
		return 1
	}
	settings, flagLines, err := command.SwitchboardSettings(env, values)
	if err != nil {
		say(err)
		return 1
	}
	for _, line := range flagLines {
		say(line)
	}
	logPath, host, timing := settings.Log, settings.Host, settings.Timing
	warnIfExposed(addr)
	dataDirectory := filepath.Dir(command.DefaultLogPath(env))
	machinesPath := filepath.Join(dataDirectory, "machines.json")
	lockPath := command.SwitchboardLockPath(env)
	oldMachinesPath := filepath.Join(filepath.Dir(logPath), "machines.json")
	lock, err := server.LockDataDirectory(lockPath)
	if err != nil {
		say(err)
		return 1
	}
	defer lock.Release()
	if err := server.CheckLogIsItsOwnFile(logPath, machinesPath, lockPath, oldMachinesPath); err != nil {
		say(err)
		return 1
	}
	notice, err := server.CarryOverMachines(machinesPath, oldMachinesPath)
	if err != nil {
		say(err)
		return 1
	}
	if notice != "" {
		say(notice)
	}
	log, err := store.Open(logPath)
	if err != nil {
		say(err)
		return 1
	}
	defer log.Close()
	secret := os.Getenv("LANDLINE_SECRET")
	if secret == "" {
		secret = saved.Secret
	}
	if secret == "" {
		say("there is no secret. Run landline server up, or set LANDLINE_SECRET")
		return 1
	}
	switchboard, err := server.New(server.Config{
		Secret: secret, Host: host,
		KeepaliveInterval: timing.Keepalive, DeliveryTimeout: timing.DeliveryTimeout, Grace: timing.Grace,
		InviteLifetime: timing.InviteLifetime, SignatureWindow: timing.SignatureWindow,
		MachinesPath: machinesPath,
		Notify:       command.NotifyToStderr,
		Build:        server.BuildResponse{Version: version, Commit: commit, Date: date},
	}, log)
	if err != nil {
		say(err)
		return 1
	}
	listener, resolved, err := listenOn(addr)
	if err != nil {
		say(fmt.Sprintf("cannot listen on %s: %v", addr, err))
		return command.ExitCannotListen
	}
	switchboard.Listening()
	fmt.Fprint(os.Stderr, command.BannerFor(os.Stderr))
	fmt.Fprintln(os.Stderr, command.ListeningLinePrefix+command.Printable(fmt.Sprintf("%s, host %q, log %s%s", listener.Addr(), host, logPath, asGiven(addr, listener.Addr().String(), resolved))))
	srv := &http.Server{Handler: switchboard.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second}
	if err := srv.Serve(listener); err != nil {
		say(err)
		return 1
	}
	return 0
}

func say(text any) {
	fmt.Fprintln(os.Stderr, name+": "+command.Printable(fmt.Sprint(text)))
}

func listenOn(addr string) (net.Listener, []net.IP, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || net.ParseIP(host) != nil {
		listener, err := net.Listen("tcp", addr)
		return listener, nil, err
	}
	resolved, err := net.LookupIP(host)
	if err != nil {
		return nil, nil, err
	}
	if len(resolved) == 0 {
		return nil, nil, fmt.Errorf("lookup %s: no addresses", host)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(resolved[0].String(), port))
	return listener, resolved, err
}

func asGiven(addr, bound string, resolved []net.IP) string {
	if addr == bound {
		return ""
	}
	line := ", address as given " + addr
	if len(resolved) == 0 {
		return line
	}
	names := make([]string, len(resolved))
	for i, ip := range resolved {
		names[i] = ip.String()
	}
	return line + ", which resolves to " + strings.Join(names, ", ") + ". It bound " + bound
}

func warnIfExposed(addr string) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		say(fmt.Sprintf("warning: %s is not a loopback address. The switchboard speaks plain HTTP, so tokens and messages cross the network in the clear. Prefer localhost and a tunnel.", addr))
	}
}
