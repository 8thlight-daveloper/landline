package agentd

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type pollResult struct {
	jobs []model.WakeJob
	err  error
}

type fakeHub struct {
	mu      sync.Mutex
	results []pollResult
	hosts   []string
	waits   []int
	acks    map[string]model.WakeAck
	ackErr  error
	onEmpty func()
}

func (f *fakeHub) Wakes(host string, waitSeconds int) ([]model.WakeJob, error) {
	f.mu.Lock()
	f.hosts = append(f.hosts, host)
	f.waits = append(f.waits, waitSeconds)
	if len(f.results) == 0 {
		f.mu.Unlock()
		f.onEmpty()
		return nil, nil
	}
	next := f.results[0]
	f.results = f.results[1:]
	f.mu.Unlock()
	return next.jobs, next.err
}

func (f *fakeHub) AckWake(id string, ack model.WakeAck) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.acks == nil {
		f.acks = map[string]model.WakeAck{}
	}
	f.acks[id] = ack
	return f.ackErr
}

type fakeWaker struct {
	calls []string
	fail  error
}

func (f *fakeWaker) Wake(target, text string) error {
	f.calls = append(f.calls, target+"|"+text)
	return f.fail
}

type notes struct{ lines []string }

func (n *notes) add(line string) { n.lines = append(n.lines, line) }

func (n *notes) has(part string) bool {
	for _, line := range n.lines {
		if strings.Contains(line, part) {
			return true
		}
	}
	return false
}

func runUntilEmpty(t *testing.T, hub *fakeHub, wakers map[string]Waker, n *notes, sleep func(context.Context, time.Duration)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub.onEmpty = cancel
	if err := run(ctx, hub, "linux", wakers, n.add, sleep); err != nil {
		t.Fatalf("run returned %v", err)
	}
}

func noSleep(context.Context, time.Duration) {}

func TestAJobWakesItsTargetAndIsAckedAsDone(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{results: []pollResult{{jobs: []model.WakeJob{{ID: "m-a-bravo", Agent: "bravo", Kind: "codex", Target: "thread-9", Text: "[landline] hi"}}}}}
	waker := &fakeWaker{}
	n := &notes{}
	runUntilEmpty(t, hub, map[string]Waker{"codex": waker}, n, noSleep)
	if len(waker.calls) != 1 || waker.calls[0] != "thread-9|[landline] hi" {
		t.Fatalf("calls %v", waker.calls)
	}
	if ack, ok := hub.acks["m-a-bravo"]; !ok || !ack.OK {
		t.Fatalf("acks %+v", hub.acks)
	}
	if !n.has("woke bravo") {
		t.Fatalf("notes %v", n.lines)
	}
	if hub.hosts[0] != "linux" || hub.waits[0] != 30 {
		t.Fatalf("polled %v %v", hub.hosts, hub.waits)
	}
}

func TestAFailedWakeIsAckedWithTheError(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{results: []pollResult{{jobs: []model.WakeJob{{ID: "j1", Agent: "bravo", Kind: "codex", Target: "t", Text: "[landline] hi"}}}}}
	n := &notes{}
	runUntilEmpty(t, hub, map[string]Waker{"codex": &fakeWaker{fail: errors.New("no such thread")}}, n, noSleep)
	if ack := hub.acks["j1"]; ack.OK || !strings.Contains(ack.Error, "no such thread") {
		t.Fatalf("ack %+v", ack)
	}
	if !n.has("failed") {
		t.Fatalf("notes %v", n.lines)
	}
}

func TestAnUnknownWakeKindIsAckedAsFailedNamingTheKind(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{results: []pollResult{{jobs: []model.WakeJob{{ID: "j1", Agent: "bravo", Kind: "pigeon", Target: "t", Text: "[landline] hi"}}}}}
	waker := &fakeWaker{}
	n := &notes{}
	runUntilEmpty(t, hub, map[string]Waker{"codex": waker}, n, noSleep)
	if ack := hub.acks["j1"]; ack.OK || !strings.Contains(ack.Error, "pigeon") {
		t.Fatalf("ack %+v", ack)
	}
	if len(waker.calls) != 0 {
		t.Fatalf("a waker ran: %v", waker.calls)
	}
}

func TestAFailedAckIsReported(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{ackErr: errors.New("hub down"), results: []pollResult{{jobs: []model.WakeJob{{ID: "j1", Agent: "bravo", Kind: "codex", Text: "[landline] hi"}}}}}
	n := &notes{}
	runUntilEmpty(t, hub, map[string]Waker{"codex": &fakeWaker{}}, n, noSleep)
	if !n.has("could not acknowledge") {
		t.Fatalf("notes %v", n.lines)
	}
}

func TestBackoffDoublesToTheCapAndResetsAfterASuccess(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	results := []pollResult{}
	for range 8 {
		results = append(results, pollResult{err: boom})
	}
	results = append(results, pollResult{}, pollResult{err: boom})
	hub := &fakeHub{results: results}
	n := &notes{}
	var slept []time.Duration
	runUntilEmpty(t, hub, map[string]Waker{}, n, func(_ context.Context, d time.Duration) { slept = append(slept, d) })
	want := []time.Duration{1, 2, 4, 8, 16, 30, 30, 30, 1}
	if len(slept) != len(want) {
		t.Fatalf("slept %v", slept)
	}
	for i, seconds := range want {
		if slept[i] != seconds*time.Second {
			t.Fatalf("slept %v, want %v seconds", slept, want)
		}
	}
	if !n.has("boom") {
		t.Fatalf("notes %v", n.lines)
	}
}

func TestACancelledContextEndsTheLoopWithNil(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	hub := &fakeHub{}
	if err := run(ctx, hub, "linux", nil, func(string) {}, noSleep); err != nil {
		t.Fatalf("got %v", err)
	}
	if len(hub.hosts) != 0 {
		t.Fatalf("polled after cancel: %v", hub.hosts)
	}
}

func TestCancellingDuringABackoffReturnsNil(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{results: []pollResult{{err: errors.New("boom")}}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, hub, "linux", nil, func(string) { cancel() })
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop")
	}
}
