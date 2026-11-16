package command

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed skills/landline/SKILL.md
var landlineSkill []byte

const nixStoreMarker = "/nix/store/"

func installSkill(home string) (string, error) {
	dir := filepath.Join(home, ".claude", "skills", "landline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "SKILL.md")
	return path, os.WriteFile(path, landlineSkill, 0o644)
}

const setupUsage = "usage: landline setup claude|codex [--remove] [--print]"

func RemoveCommand(harness string) ([]string, error) {
	switch harness {
	case "claude":
		return []string{"claude", "mcp", "remove", "--scope", "user", ChannelServerName}, nil
	case "codex":
		return []string{"codex", "mcp", "remove", ChannelServerName}, nil
	}
	return nil, fmt.Errorf("landline setup knows claude and codex, and not %q", harness)
}

func removeSkill(home string) (string, bool, error) {
	dir := filepath.Join(home, ".claude", "skills", "landline")
	path := filepath.Join(dir, "SKILL.md")
	installed, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return path, true, removeEmptyDir(dir)
	}
	if err != nil {
		return path, false, err
	}
	if !bytes.Equal(installed, landlineSkill) {
		return path, false, nil
	}
	if err := os.Remove(path); err != nil {
		return path, false, err
	}
	return path, true, removeEmptyDir(dir)
}

func removeEmptyDir(dir string) error {
	err := os.Remove(dir)
	if os.IsNotExist(err) || isDirectoryNotEmpty(err) {
		return nil
	}
	return err
}

func isDirectoryNotEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST)
}

func runSetupRemove(harness string, print bool, env Env) int {
	argv, err := RemoveCommand(harness)
	if err != nil {
		fmt.Fprintln(env.Stderr, setupUsage)
		return 2
	}
	if print {
		fmt.Fprintln(env.Stdout, strings.Join(argv, " "))
		return 0
	}
	status := runHarnessRemove(argv, env)
	if harness != "claude" {
		return status
	}
	if env.Home == "" {
		fmt.Fprintln(env.Stderr, "landline setup: there is no home folder to look for the skill in")
		return 1
	}
	path, removed, err := removeSkill(env.Home)
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline setup: the skill was not removed:", err)
		return 1
	}
	if !removed {
		fmt.Fprintf(env.Stdout, "kept %s, which differs from the skill that setup installs\n", path)
	}
	return status
}

func runHarnessRemove(argv []string, env Env) int {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = env.Stdin, env.Stdout, env.Stderr
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	fmt.Fprintf(env.Stderr, "landline setup: %s: %v\n", argv[0], err)
	return 1
}

func runSetup(args []string, env Env) int {
	print := false
	remove := false
	var harness string
	for _, each := range args {
		switch {
		case each == "--print":
			print = true
		case each == "--remove":
			remove = true
		case harness == "" && !strings.HasPrefix(each, "-"):
			harness = each
		default:
			fmt.Fprintln(env.Stderr, setupUsage)
			return 2
		}
	}
	if remove {
		return runSetupRemove(harness, print, env)
	}
	binary, err := findBinary(MCPBinaryName)
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline setup:", err)
		return 1
	}
	argv, err := SetupCommand(harness, binary)
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline setup:", err)
		return 2
	}
	if strings.Contains(binary, nixStoreMarker) {
		fmt.Fprintf(env.Stderr, "landline setup: warning: %s is at a Nix store path that garbage collection can remove. Install it with nix profile install and run setup again from there\n", MCPBinaryName)
	}
	if print {
		fmt.Fprintln(env.Stdout, strings.Join(argv, " "))
		return 0
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = env.Stdin, env.Stdout, env.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(env.Stderr, "landline setup:", err)
		return 1
	}
	if harness == "claude" {
		var path string
		if env.Home == "" {
			err = fmt.Errorf("there is no home folder")
		} else {
			path, err = installSkill(env.Home)
		}
		if err != nil {
			fmt.Fprintln(env.Stderr, "landline setup: the MCP server is added, but the skill was not installed:", err)
			return 1
		}
		fmt.Fprintln(env.Stdout, "installed the /landline skill at", path)
	}
	return 0
}

func runClaude(args []string, env Env) int {
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
	launch, err := ClaudeLaunch(rest)
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline claude:", err)
		return 2
	}
	if print {
		fmt.Fprintln(env.Stdout, strings.Join(append(append([]string{}, launch.Env...), launch.Argv...), " "))
		return 0
	}
	path, err := exec.LookPath(launch.Argv[0])
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline claude: claude is not on your PATH")
		return 1
	}
	kept := []string{}
	for _, pair := range os.Environ() {
		if !strings.HasPrefix(pair, "LANDLINE_SECRET=") {
			kept = append(kept, pair)
		}
	}
	fmt.Fprintln(env.Stderr, "Choose \"I am using this for local development\" on the confirmation screen.")
	if err := syscall.Exec(path, launch.Argv, append(kept, launch.Env...)); err != nil {
		fmt.Fprintln(env.Stderr, "landline claude:", err)
		return 1
	}
	return 0
}
