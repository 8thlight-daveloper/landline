package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/internal/cli/command"
	"github.com/8thlight-daveloper/landline/internal/model"
	"github.com/8thlight-daveloper/landline/tui/internal/agentsview"
	"github.com/8thlight-daveloper/landline/tui/internal/clean"
	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
	"github.com/8thlight-daveloper/landline/tui/internal/pacing"
	"github.com/8thlight-daveloper/landline/tui/internal/screen"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

var usage = strings.TrimRight(command.Usage, "\n")

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
	env := environment(stdout, stderr)
	if len(args) > 0 && args[0] == "version" {
		fmt.Fprint(stdout, bannerFor(stdout))
		fmt.Fprintln(stdout, command.VersionLine("landline", version, commit, date))
		return 0
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") && args[0] != "agents" {
		return command.Run(args, env)
	}
	opts, problem := parseArgs(args)
	if opts.help {
		fmt.Fprintln(stdout, usage)
		return 0
	}
	if problem != "" {
		return fail(stderr, problem, 2)
	}
	if err := pacing.CheckInterval(opts.interval); err != nil {
		return fail(stderr, withUsage("landline: "+err.Error()), 2)
	}
	if !opts.agents && !(isTerminal(os.Stdin) && isTerminal(stdout)) {
		return fail(stderr, usage, 2)
	}
	client, code := connect(stderr)
	if client == nil {
		return code
	}
	if opts.agents {
		return agents(client, opts.asJSON, stdout, stderr)
	}
	return interactive(opts.interval, client, stderr)
}

func environment(stdout, stderr io.Writer) command.Env {
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	return command.Env{
		Stdin: os.Stdin, Stdout: stdout, Stderr: stderr, Getenv: os.Getenv,
		ConfigDir: filepath.Join(home, ".config", "landline"), Home: home, Host: host,
		Banner: func() string { return command.BannerFor(os.Stderr) }, Version: version,
	}
}

func bannerFor(out io.Writer) string {
	if file, ok := out.(*os.File); ok {
		return command.BannerFor(file)
	}
	return ""
}

func isTerminal(out any) bool {
	file, ok := out.(*os.File)
	return ok && isatty.IsTerminal(file.Fd())
}

func connect(stderr io.Writer) (*hubclient.Client, int) {
	home, _ := os.UserHomeDir()
	address, credential, err := hubclient.DiscoverCredential(os.Getenv, home)
	if err != nil {
		return nil, fail(stderr, err.Error(), 1)
	}
	client := hubclient.NewWith(address, credential)
	client.RereadCredentialWith(func() (string, hubclient.Credential, error) {
		return hubclient.DiscoverCredential(os.Getenv, home)
	})
	return client, 0
}

func fail(stderr io.Writer, text string, code int) int {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = clean.Text(line)
	}
	fmt.Fprintln(stderr, strings.Join(lines, "\n"))
	return code
}

func agents(client *hubclient.Client, asJSON bool, stdout, stderr io.Writer) int {
	list, err := client.Agents()
	if err != nil {
		return fail(stderr, "landline: "+err.Error(), 1)
	}
	view := agentsview.Build(client.Address(), time.Now(), list)
	if asJSON {
		raw, _ := json.MarshalIndent(view, "", "  ")
		fmt.Fprintln(stdout, string(raw))
		return 0
	}
	fmt.Fprint(stdout, agentLines(view))
	return 0
}

func agentLines(view agentsview.View) string {
	var out strings.Builder
	for _, row := range view.Agents {
		line := fmt.Sprintf("%s\t%s\t%s\t%s", command.Printable(row.ID), command.Printable(row.Harness), command.Printable(row.Host), row.State)
		if attributes := rootAgent(row).Attributes(); attributes != "" {
			line += "\t" + command.Printable(attributes)
		}
		out.WriteString(line + "\n")
	}
	return out.String()
}

func rootAgent(row agentsview.Row) model.AgentInfo {
	return model.AgentInfo{
		Project: row.Project, Task: row.Task, TaskAt: parsedTaskAt(row.TaskAt), Folder: row.Folder, Branch: row.Branch, Worktree: row.Worktree,
		HarnessVersion: row.HarnessVersion, LandlineVersion: row.LandlineVersion,
		LandlineCommit: row.LandlineCommit, LandlineDate: row.LandlineDate,
		Model: row.Model, Effort: row.Effort,
	}
}

func parsedTaskAt(text string) time.Time {
	at, _ := time.Parse(time.RFC3339, text)
	return at
}

func interactive(interval time.Duration, client *hubclient.Client, stderr io.Writer) int {
	model := screen.New(client.Address(), interval, client.Agents, time.Now)
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fail(stderr, "landline: "+err.Error(), 1)
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
	if opts.asJSON && !opts.agents {
		return opts, usage
	}
	return opts, ""
}
