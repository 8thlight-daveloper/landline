package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/8thlight-daveloper/landline/internal/cli/command"
)

//go:embed skills/landline/SKILL.md
var landlineSkill []byte

// installSkill writes the /landline skill where Claude Code looks for personal skills.
func installSkill(home string) (string, error) {
	dir := filepath.Join(home, ".claude", "skills", "landline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "SKILL.md")
	return path, os.WriteFile(path, landlineSkill, 0o644)
}

func runSetup(args []string) int {
	print := false
	var harness string
	for _, each := range args {
		switch {
		case each == "--print":
			print = true
		case harness == "" && !strings.HasPrefix(each, "-"):
			harness = each
		default:
			fmt.Fprintln(os.Stderr, "usage: landline setup claude|codex [--print]")
			return 2
		}
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline setup:", err)
		return 1
	}
	argv, err := command.SetupCommand(harness, self)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline setup:", err)
		return 2
	}
	if strings.Contains(self, "/nix/store/") {
		fmt.Fprintln(os.Stderr, "landline setup: warning: this binary is at a Nix store path that garbage collection can remove. Install it with nix profile install and run setup again from there")
	}
	if print {
		fmt.Println(strings.Join(argv, " "))
		return 0
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "landline setup:", err)
		return 1
	}
	if harness == "claude" {
		home, err := os.UserHomeDir()
		if err == nil {
			var path string
			if path, err = installSkill(home); err == nil {
				fmt.Println("installed the /landline skill at", path)
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "landline setup: the MCP server is added, but the skill was not installed:", err)
			return 1
		}
	}
	return 0
}

func runClaude(args []string) int {
	print := false
	rest := args[:0:0]
	for i, each := range args {
		if each == "--print" {
			print = true
			continue
		}
		if each == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		rest = append(rest, each)
	}
	launch, err := command.ClaudeLaunch(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline claude:", err)
		return 2
	}
	if print {
		fmt.Println(strings.Join(append(append([]string{}, launch.Env...), launch.Argv...), " "))
		return 0
	}
	path, err := exec.LookPath(launch.Argv[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "landline claude: claude is not on your PATH")
		return 1
	}
	env := []string{}
	for _, pair := range os.Environ() {
		if !strings.HasPrefix(pair, "LANDLINE_SECRET=") {
			env = append(env, pair)
		}
	}
	fmt.Fprintln(os.Stderr, "Choose \"I am using this for local development\" on the confirmation screen.")
	if err := syscall.Exec(path, launch.Argv, append(env, launch.Env...)); err != nil {
		fmt.Fprintln(os.Stderr, "landline claude:", err)
		return 1
	}
	return 0
}
