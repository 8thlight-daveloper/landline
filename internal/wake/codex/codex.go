package codex

import (
	"fmt"
	"os/exec"
	"strings"
)

type Waker struct {
	run func(name string, args ...string) ([]byte, error)
}

func New() Waker {
	return Waker{run: func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}}
}

func NewWith(run func(name string, args ...string) ([]byte, error)) Waker { return Waker{run: run} }

func (w Waker) Wake(thread, text string) error {
	if strings.TrimSpace(thread) == "" {
		return fmt.Errorf("a codex wake needs a thread id or name")
	}
	if !strings.HasPrefix(text, "[landline] ") {
		return fmt.Errorf("refusing to wake with text that lacks the landline marker")
	}
	out, err := w.run("codex", "queue", "--thread", thread, "--message", text)
	if err != nil {
		return fmt.Errorf("codex queue: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
