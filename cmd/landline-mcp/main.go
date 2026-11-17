package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/internal/cli/command"
	mcp "github.com/8thlight-daveloper/landline/internal/mcp/server"
	"github.com/8thlight-daveloper/landline/internal/wake/codex"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

const name = "landline-mcp"

func main() {
	os.Exit(run(os.Args[1:]))
}

func gitRunner(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
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
	channel, as := false, ""
	overrides := command.Identity{LandlineVersion: version, LandlineCommit: commit, LandlineDate: date}
	targets := map[string]*string{
		"--as": &as, "--project": &overrides.Project, "--model": &overrides.Model, "--effort": &overrides.Effort,
		"--folder": &overrides.Folder, "--branch": &overrides.Branch,
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--channel" {
			channel = true
			continue
		}
		target, known := targets[args[i]]
		if !known || i+1 >= len(args) {
			fmt.Fprintf(os.Stderr, "%s: unknown or incomplete flag %s\n", name, args[i])
			return 2
		}
		*target = args[i+1]
		i++
	}
	options := mcp.Options{Channel: channel, Waker: codex.New(), Notify: command.NotifyToStderr}
	if as == "" {
		as = os.Getenv("LANDLINE_AS")
	}
	if as != "" {
		c, err := command.Connect(as, env)
		if err != nil {
			fmt.Fprintln(os.Stderr, name+":", err)
			return 1
		}
		if err := mcp.ServeWith(os.Stdin, os.Stdout, c, options); err != nil {
			fmt.Fprintln(os.Stderr, name+":", err)
			return 1
		}
		return 0
	}
	dir, _ := os.Getwd()
	session := command.NewSession(env, func() command.Identity { return command.DetectIdentity(env, dir, gitRunner) }, overrides)
	defer session.Close()
	options.OnClient = session.SetClient
	options.Project = session.Project
	options.ID = session.ID
	options.OnReady = func() {
		if _, err := session.Agents(); err != nil {
			command.NotifyToStderr(name + ": " + err.Error())
		}
	}
	if err := mcp.ServeWith(os.Stdin, os.Stdout, session, options); err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		return 1
	}
	return 0
}
