package codex

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

const wakeTimeout = 15 * time.Second

type Waker struct {
	run func(name string, args ...string) ([]byte, error)
}

func New() Waker { return withTimeout(wakeTimeout) }

func withTimeout(limit time.Duration) Waker {
	return Waker{run: func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), limit)
		defer cancel()
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}}
}

func NewWith(run func(name string, args ...string) ([]byte, error)) Waker { return Waker{run: run} }

func (w Waker) Wake(thread, text string) error {
	if strings.TrimSpace(thread) == "" {
		return fmt.Errorf("a codex wake needs a thread id or name")
	}
	if !strings.HasPrefix(text, "[landline] "+model.Marker+"\n") {
		return fmt.Errorf("refusing to wake with text that lacks the landline marker")
	}
	out, err := w.run("codex", "queue", "--thread", thread, "--message", text)
	if err != nil {
		return fmt.Errorf("codex queue: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
