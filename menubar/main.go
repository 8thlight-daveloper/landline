package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/internal/cli/command"
	"github.com/8thlight-daveloper/landline/menubar/internal/about"
	"github.com/8thlight-daveloper/landline/menubar/internal/buildinfo"
	"github.com/8thlight-daveloper/landline/menubar/internal/clean"
	"github.com/8thlight-daveloper/landline/menubar/internal/hubclient"
	"github.com/8thlight-daveloper/landline/menubar/internal/menu/session"
	"github.com/8thlight-daveloper/landline/menubar/internal/pacing"
	"github.com/8thlight-daveloper/landline/menubar/internal/path/resolver"
	"github.com/8thlight-daveloper/landline/menubar/internal/screen"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

const usage = `usage: landline-menubar [--interval D]
       landline-menubar menu --text [--interval D]
       landline-menubar version

landline-menubar shows the Landline agents in the macOS menu bar.
  menu --text   print the menu as text and exit
  version       print the version
  --interval D  how often to poll the switchboard, at least 500ms (default 5s)`

func init() { runtime.LockOSThread() }

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type options struct {
	help     bool
	version  bool
	menu     bool
	text     bool
	interval time.Duration
}

const badInterval = "the interval must be a duration such as 5s"

func run(args []string, stdout, stderr io.Writer) int {
	opts, problem := parseArgs(args)
	switch {
	case opts.help:
		fmt.Fprintln(stdout, usage)
		return 0
	case problem != "":
		return fail(stderr, problem, 2)
	case opts.version:
		fmt.Fprintln(stdout, command.VersionLine("landline-menubar", version, commit, date))
		return 0
	}
	if err := pacing.CheckInterval(opts.interval); err != nil {
		return fail(stderr, withUsage("landline-menubar: "+err.Error()), 2)
	}
	home, _ := os.UserHomeDir()
	address, secret, err := hubclient.Discover(os.Getenv, home)
	if err != nil {
		address = strings.TrimRight(os.Getenv("LANDLINE_URL"), "/")
	}
	client := hubclient.New(address, secret)
	client.RereadSecretWith(func() string {
		_, fresh, err := hubclient.Discover(os.Getenv, home)
		if err != nil {
			return ""
		}
		return fresh
	})
	app := buildinfo.Stamped(version, commit)
	self, _ := os.Executable()
	loginPath := resolver.Login(os.Getenv("SHELL"), os.Getenv("PATH"), resolver.LoginTimeout)
	find := func() (string, bool) { return resolver.Find("landline", self, loginPath) }
	state := session.New(address, app)
	if opts.text {
		_, found := find()
		state.Apply(session.Observe(client, found))
		fmt.Fprint(stdout, state.Menu().Text())
		return 0
	}
	if err != nil {
		state.Notify(clean.Text(err.Error()))
	}
	about.UseAccessoryPolicy()
	screen.Run(screen.Config{Source: client, Session: state, Interval: opts.interval, Find: find, Path: loginPath, App: app})
	return 0
}

func fail(stderr io.Writer, text string, code int) int {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = clean.Text(line)
	}
	fmt.Fprintln(stderr, strings.Join(lines, "\n"))
	return code
}

func withUsage(reason string) string { return reason + "\n" + usage }

func parseArgs(args []string) (options, string) {
	opts := options{interval: pacing.DefaultInterval}
	words := 0
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			words++
			switch {
			case words > 1:
				return opts, usage
			case arg == "version":
				opts.version = true
			case arg == "menu":
				opts.menu = true
			default:
				return opts, usage
			}
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch name {
		case "h", "help":
			opts.help = true
			return opts, ""
		case "text":
			if hasValue {
				return opts, usage
			}
			opts.text = true
		case "interval":
			if !hasValue {
				if i+1 >= len(args) {
					return opts, withUsage(badInterval)
				}
				i++
				value = args[i]
			}
			interval, err := time.ParseDuration(value)
			if err != nil {
				return opts, withUsage(badInterval)
			}
			opts.interval = interval
		default:
			return opts, usage
		}
	}
	if opts.menu != opts.text || (opts.version && (opts.text || len(args) != 1)) {
		return opts, usage
	}
	return opts, ""
}
