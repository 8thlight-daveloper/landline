package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/tui/internal/agentsview"
	"github.com/8thlight-daveloper/landline/tui/internal/clean"
	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
	"github.com/8thlight-daveloper/landline/tui/internal/pacing"
	"github.com/8thlight-daveloper/landline/tui/internal/screen"
	tea "github.com/charmbracelet/bubbletea"
)

const usage = "usage: landline-tui [--interval D]\n       landline-tui agents --json\n       landline-tui -h | --help"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type options struct {
	agents   bool
	asJSON   bool
	help     bool
	interval time.Duration
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, problem := parseArgs(args)
	if opts.help {
		fmt.Fprintln(stdout, usage)
		return 0
	}
	if problem != "" {
		return fail(stderr, problem, 2)
	}
	if err := pacing.CheckInterval(opts.interval); err != nil {
		return fail(stderr, "landline-tui: "+err.Error(), 2)
	}
	client, code := connect(stderr)
	if client == nil {
		return code
	}
	if opts.agents {
		return agents(client, stdout, stderr)
	}
	return interactive(opts.interval, client, stderr)
}

func connect(stderr io.Writer) (*hubclient.Client, int) {
	home, _ := os.UserHomeDir()
	address, secret, err := hubclient.Discover(os.Getenv, home)
	if err != nil {
		return nil, fail(stderr, err.Error(), 1)
	}
	return hubclient.New(address, secret), 0
}

func fail(stderr io.Writer, text string, code int) int {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = clean.Text(line)
	}
	fmt.Fprintln(stderr, strings.Join(lines, "\n"))
	return code
}

func agents(client *hubclient.Client, stdout, stderr io.Writer) int {
	list, err := client.Agents()
	if err != nil {
		return fail(stderr, "landline-tui: "+err.Error(), 1)
	}
	raw, _ := json.MarshalIndent(agentsview.Build(client.Address(), time.Now(), list), "", "  ")
	fmt.Fprintln(stdout, string(raw))
	return 0
}

func interactive(interval time.Duration, client *hubclient.Client, stderr io.Writer) int {
	model := screen.New(client.Address(), interval, client.Agents, time.Now)
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fail(stderr, "landline-tui: "+err.Error(), 1)
	}
	return 0
}

const badInterval = "the interval must be a duration such as 2s"

func withUsage(reason string) string { return reason + "\n" + usage }

func parseArgs(args []string) (options, string) {
	opts := options{interval: pacing.DefaultInterval}
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(strings.TrimLeft(args[i], "-"), "=")
		if !strings.HasPrefix(args[i], "-") {
			if args[i] != "agents" || opts.agents {
				return opts, usage
			}
			opts.agents = true
			continue
		}
		switch name {
		case "h", "help":
			opts.help = true
			return opts, ""
		case "json":
			if hasValue {
				return opts, usage
			}
			opts.asJSON = true
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
	if opts.agents != opts.asJSON {
		return opts, usage
	}
	return opts, ""
}
