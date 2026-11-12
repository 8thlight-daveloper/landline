package command

import (
	"fmt"
	"path/filepath"
)

const TUIBinaryName = "landline-tui"

const tuiMissing = "landline-tui is not installed. Build it with: just install-tui\n"

type TUI struct {
	Executable string
	LookPath   func(name string) (string, error)
	IsFile     func(path string) bool
	Exec       func(path string, argv []string) error
}

func (t TUI) find() (string, bool) {
	if found, err := t.LookPath(TUIBinaryName); err == nil && found != "" {
		return found, true
	}
	if t.Executable != "" {
		beside := filepath.Join(filepath.Dir(t.Executable), TUIBinaryName)
		if t.IsFile(beside) {
			return beside, true
		}
	}
	return "", false
}

func RunTUI(args []string, env Env, tui TUI) int {
	path, found := tui.find()
	if !found {
		fmt.Fprint(env.Stderr, tuiMissing)
		return 1
	}
	if err := tui.Exec(path, append([]string{TUIBinaryName}, args...)); err != nil {
		fmt.Fprintln(env.Stderr, "landline:", err)
		return 1
	}
	return 0
}
