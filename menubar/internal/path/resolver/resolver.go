package resolver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	LoginTimeout = 5 * time.Second
	pipeWait     = time.Second
)

func Login(shell, fallback string, timeout time.Duration) string {
	if shell == "" {
		return fallback
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", `printf %s "$PATH"`)
	cmd.WaitDelay = pipeWait
	out, err := cmd.Output()
	if err != nil {
		return fallback
	}
	return Parse(string(out), fallback)
}

func Parse(output, fallback string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" || strings.ContainsFunc(last, unicode.IsControl) {
		return fallback
	}
	return last
}

func Find(name string, self string, path string) (string, bool) {
	var dirs []string
	if self != "" {
		dirs = append(dirs, filepath.Dir(self))
		if resolved, err := filepath.EvalSymlinks(self); err == nil {
			dirs = append(dirs, filepath.Dir(resolved))
		}
	}
	dirs = append(dirs, filepath.SplitList(path)...)
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if executable(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}
