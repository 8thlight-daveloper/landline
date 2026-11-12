package screen

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
	"github.com/8thlight-daveloper/landline/tui/internal/pacing"
	tea "github.com/charmbracelet/bubbletea"
)

var clock = time.Date(2026, 11, 12, 10, 30, 5, 0, time.UTC)

func newModel() Model {
	fetch := func() ([]hubclient.Agent, error) { return nil, nil }
	return New("http://hub:1", 2*time.Second, fetch, func() time.Time { return clock })
}

func send(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func key(text string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)} }

var sample = []hubclient.Agent{
	{Name: "alpha", Harness: "claude", Host: "mac", Connected: true, Label: "lead", Folder: "/work/app", Branch: "main", Model: "opus", Effort: "high", LandlineVersion: "0.9"},
	{Name: "bravo", Harness: "codex", Host: "mac"},
}

func TestViewShowsLoadingBeforeTheFirstPoll(t *testing.T) {
	if view := newModel().View(); !strings.Contains(view, "Loading") || !strings.Contains(view, "hub http://hub:1") {
		t.Fatal(view)
	}
}

func TestSnapshotShowsAgentsAndTheStatusLine(t *testing.T) {
	m, _ := send(newModel(), tea.WindowSizeMsg{Width: 200, Height: 20})
	m, cmd := send(m, snapshotMsg{agents: sample, at: clock, took: 10 * time.Millisecond})
	view := m.View()
	for _, want := range []string{"HANDLE", "alpha", "lead", "connected", "bravo", "disconnected", "/work/app", "opus", "hub http://hub:1", "refreshed 10:30:05"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in\n%s", want, view)
		}
	}
	if strings.Index(view, "alpha") > strings.Index(view, "bravo") {
		t.Error("connected agent must come first")
	}
	if cmd == nil {
		t.Error("a poll schedules the next one")
	}
}

func TestEmptyListSaysSo(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: nil, at: clock})
	if !strings.Contains(m.View(), "No agents are registered with this hub.") {
		t.Fatal(m.View())
	}
}

func TestFailedPollKeepsThePreviousDataAndShowsTheError(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: sample, at: clock})
	m, _ = send(m, tea.WindowSizeMsg{Width: 200, Height: 20})
	m, _ = send(m, snapshotMsg{err: errors.New("cannot reach the hub at http://hub:1: refused"), at: clock.Add(time.Minute)})
	view := m.View()
	if !strings.Contains(view, "alpha") || !strings.Contains(view, "error: cannot reach the hub") || !strings.Contains(view, "refreshed 10:30:05") {
		t.Fatal(view)
	}
	m, _ = send(m, snapshotMsg{agents: sample, at: clock.Add(2 * time.Minute)})
	if strings.Contains(m.View(), "error:") {
		t.Fatal("a good poll clears the error")
	}
}

func TestFailedFirstPollShowsTheErrorAndNoStaleList(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{err: errors.New("boom")})
	if view := m.View(); !strings.Contains(view, "error: boom") || strings.Contains(view, "Loading") {
		t.Fatal(view)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, msg := range []tea.KeyMsg{key("q"), {Type: tea.KeyCtrlC}} {
		_, cmd := send(newModel(), msg)
		if cmd == nil {
			t.Fatalf("%v gave no command", msg)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%v did not quit", msg)
		}
	}
}

func TestRefreshKeyFetchesNowUnlessOneIsRunning(t *testing.T) {
	calls := 0
	fetch := func() ([]hubclient.Agent, error) { calls++; return sample, nil }
	m := New("h", 2*time.Second, fetch, func() time.Time { return clock })
	m, cmd := send(m, key("r"))
	if cmd != nil {
		t.Fatal("a fetch is already running at start")
	}
	m, _ = send(m, snapshotMsg{agents: sample, at: clock})
	m, cmd = send(m, key("r"))
	if cmd == nil {
		t.Fatal("r must fetch")
	}
	msg, ok := cmd().(snapshotMsg)
	if !ok || calls != 1 || len(msg.agents) != 2 {
		t.Fatalf("calls %d msg %#v", calls, msg)
	}
	if !strings.Contains(m.View(), "refreshing...") {
		t.Fatalf("no marker in\n%s", m.View())
	}
	again, cmd := send(m, key("r"))
	if cmd != nil || !strings.Contains(again.View(), "already refreshing") {
		t.Fatalf("r while fetching must say so\n%s", again.View())
	}
	done, _ := send(again, snapshotMsg{agents: sample, at: clock})
	if strings.Contains(done.View(), "refreshing") {
		t.Fatal(done.View())
	}
}

func TestStaleTickIsIgnored(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: sample, at: clock})
	stale := tickMsg{generation: m.generation - 1}
	if _, cmd := send(m, stale); cmd != nil {
		t.Fatal("stale tick must not fetch")
	}
	if _, cmd := send(m, tickMsg{generation: m.generation}); cmd == nil {
		t.Fatal("current tick must fetch")
	}
}

func TestSlowRequestDoublesTheNextWait(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: sample, at: clock, took: 3 * time.Second})
	if m.nextWait != 6*time.Second {
		t.Fatalf("got %s", m.nextWait)
	}
	m, _ = send(m, snapshotMsg{agents: sample, at: clock, took: 100 * time.Millisecond})
	if m.nextWait != 2*time.Second {
		t.Fatalf("got %s", m.nextWait)
	}
}

func TestTinyWidthsTruncateWithoutPanicking(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: sample, at: clock})
	for _, width := range []int{-1, 0, 1, 2, 5, 12, 30, 60, 200} {
		m, _ = send(m, tea.WindowSizeMsg{Width: width, Height: 10})
		for _, line := range strings.Split(m.View(), "\n") {
			if got := lipglossWidth(line); got > max(width, 0) {
				t.Fatalf("width %d: line %q is %d wide", width, line, got)
			}
		}
	}
}

func TestNarrowWidthStillShowsTheHandle(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: sample, at: clock})
	m, _ = send(m, tea.WindowSizeMsg{Width: 40, Height: 10})
	if !strings.Contains(m.View(), "alpha") {
		t.Fatal(m.View())
	}
}

func agentsNamed(n int) []hubclient.Agent {
	out := []hubclient.Agent{}
	for i := range n {
		out = append(out, hubclient.Agent{Name: fmt.Sprintf("agent%02d", i), Harness: "claude", Host: "mac", Connected: true})
	}
	return out
}

func TestFortyAgentsInA24RowWindowShowHeaderPrefixMoreAndStatus(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: agentsNamed(40), at: clock})
	m, _ = send(m, tea.WindowSizeMsg{Width: 120, Height: 24})
	lines := strings.Split(m.View(), "\n")
	if len(lines) > 24 {
		t.Fatalf("%d lines", len(lines))
	}
	if !strings.Contains(lines[0], "HANDLE") {
		t.Fatalf("header %q", lines[0])
	}
	for i := range 19 {
		if !strings.Contains(lines[1+i], fmt.Sprintf("agent%02d", i)) {
			t.Fatalf("row %d is %q", i, lines[1+i])
		}
	}
	if lines[20] != "+21 more" {
		t.Fatalf("got %q", lines[20])
	}
	if !strings.Contains(lines[22], "hub http://hub:1") || !strings.Contains(lines[23], "q quit") {
		t.Fatalf("footer %q", lines[21:])
	}
	if strings.Contains(m.View(), "agent19") {
		t.Fatal("rows past the cut must not show")
	}
}

func TestHeightZeroOrUnknownIsUnlimited(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: agentsNamed(40), at: clock})
	for name, model := range map[string]Model{"unknown": m, "zero": func() Model { n, _ := send(m, tea.WindowSizeMsg{Width: 120, Height: 0}); return n }()} {
		view := model.View()
		if !strings.Contains(view, "agent39") || strings.Contains(view, "more") || len(strings.Split(view, "\n")) != 44 {
			t.Fatalf("%s: %d lines", name, len(strings.Split(view, "\n")))
		}
	}
}

func TestEveryHeightKeepsTheStatusLineWithinTheWindow(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: agentsNamed(40), at: clock})
	for height := 1; height <= 50; height++ {
		next, _ := send(m, tea.WindowSizeMsg{Width: 100, Height: height})
		view := next.View()
		if n := len(strings.Split(view, "\n")); n > height {
			t.Fatalf("height %d: %d lines", height, n)
		}
		if !strings.Contains(view, "hub http://hub:1") {
			t.Fatalf("height %d: no status line in\n%s", height, view)
		}
		if height >= 4 && !strings.Contains(view, "HANDLE") {
			t.Fatalf("height %d: no header", height)
		}
	}
}

func TestAListThatFitsShowsNoMoreLine(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: agentsNamed(5), at: clock})
	m, _ = send(m, tea.WindowSizeMsg{Width: 100, Height: 9})
	if view := m.View(); strings.Contains(view, "more") || !strings.Contains(view, "agent04") {
		t.Fatal(view)
	}
}

func TestHostileValuesNeverReachTheScreen(t *testing.T) {
	hostile := []hubclient.Agent{{
		Name: "alpha", Harness: "claude", Host: "mac", Connected: true,
		Label: "\x1b]0;pwned\x07\x1b[2J", Folder: "x\ny\u202eevil", Branch: "b\rc", Model: "\u009b31m", Effort: "\x9b31m",
	}}
	m, _ := send(newModel(), snapshotMsg{agents: hostile, at: clock})
	m, _ = send(m, tea.WindowSizeMsg{Width: 200, Height: 30})
	view := m.View()
	for _, bad := range []string{"\x1b", "\x07", "\r", "\u202e", "\u009b", "\x9b"} {
		if strings.Contains(view, bad) {
			t.Fatalf("%q in %q", bad, view)
		}
	}
	if n := len(strings.Split(view, "\n")); n != 1+1+3 {
		t.Fatalf("a cell broke the layout: %d lines in %q", n, view)
	}
	if !strings.Contains(view, "pwned") || !strings.Contains(view, "x y evil") {
		t.Fatal(view)
	}
}

func TestTheMeasuredRequestDurationFeedsTheNextWaitThroughTheModel(t *testing.T) {
	now := clock
	interval := 2 * time.Second
	fetch := func() ([]hubclient.Agent, error) { now = now.Add(3 * interval); return sample, nil }
	m := New("h", interval, fetch, func() time.Time { return now })
	msg := m.Init()()
	m, cmd := send(m, msg)
	if m.nextWait != 12*time.Second {
		t.Fatalf("got %s", m.nextWait)
	}
	if cmd == nil {
		t.Fatal("no tick scheduled")
	}
}

func TestADefaultIntervalModelWaitsTwoSecondsAfterAFastFetch(t *testing.T) {
	now := clock
	fetch := func() ([]hubclient.Agent, error) { now = now.Add(10 * time.Millisecond); return sample, nil }
	m := New("h", pacing.DefaultInterval, fetch, func() time.Time { return now })
	m, _ = send(m, m.Init()())
	if m.nextWait != 2*time.Second {
		t.Fatalf("got %s", m.nextWait)
	}
}

func TestHostileErrorAndHubNeverReachTheViewAndTheStatusStaysOneLine(t *testing.T) {
	hostile := errors.New("dial \x1b]0;pwned\x07 \u0085 ‮ line1\nline2\r  \x9b31m")
	m := New("http://h\x1b[2J‮:1", 2*time.Second, nil, func() time.Time { return clock })
	m, _ = send(m, tea.WindowSizeMsg{Width: 300, Height: 24})
	m, _ = send(m, snapshotMsg{err: hostile, at: clock})
	view := m.View()
	for _, bad := range []string{"\x1b]", "\x1b[2J", "\x07", "\r", "\u0085", "‮", " ", "\x9b"} {
		if strings.Contains(view, bad) {
			t.Fatalf("%q in %q", bad, view)
		}
	}
	lines := strings.Split(view, "\n")
	var status string
	for _, l := range lines {
		if strings.Contains(l, "hub ") && strings.Contains(l, "refreshed") {
			status = l
		}
	}
	if status == "" || !strings.Contains(status, "pwned") || !strings.Contains(status, "line1 line2") {
		t.Fatalf("status %q in %q", status, view)
	}
	if len(lines) != 1+1+2 {
		t.Fatalf("the error broke the layout: %d lines in %q", len(lines), view)
	}
}
