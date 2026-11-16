package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/8thlight-daveloper/landline/menubar/internal/clean"
)

const (
	Limit         = 30 * time.Second
	NotFound      = "landline not found. Run just install"
	failedPrefix  = "Failed: "
	stderrCap     = 4096
	pipeWait      = 2 * time.Second
	maxFailedRune = 200
)

type Runner struct {
	Binary string
	Path   string
	Limit  time.Duration
	busy   sync.Mutex
}

func New(binary, path string) *Runner {
	return &Runner{Binary: binary, Path: path, Limit: Limit}
}

var ErrBusy = errors.New("another command is running")

func (r *Runner) Run(args []string) (failure string, err error) {
	if !r.busy.TryLock() {
		return "", ErrBusy
	}
	defer r.busy.Unlock()
	if r.Binary == "" {
		return failedPrefix + NotFound, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.Limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Env = WithPath(os.Environ(), r.Path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = pipeWait
	stderr := &capped{limit: stderrCap}
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if errors.Is(runErr, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success() {
		runErr = nil
	}
	switch {
	case ctx.Err() != nil:
		return fmt.Sprintf("%stimed out after %s", failedPrefix, r.Limit), nil
	case runErr != nil:
		return Failure(stderr.String(), runErr), nil
	}
	return "", nil
}

func Failure(stderr string, err error) string {
	first := FirstLine(stderr)
	if first == "" {
		first = err.Error()
	}
	runes := []rune(clean.Text(first))
	if len(runes) > maxFailedRune {
		runes = append(runes[:maxFailedRune-1], '…')
	}
	return failedPrefix + string(runes)
}

func FirstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func WithPath(env []string, path string) []string {
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, "PATH=") {
			out = append(out, entry)
		}
	}
	if path == "" {
		return out
	}
	return append(out, "PATH="+path)
}

type capped struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func (c *capped) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := c.limit - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

func (c *capped) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}
