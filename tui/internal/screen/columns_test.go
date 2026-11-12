package screen

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func headerTitles(view string) []string {
	return strings.Fields(strings.Split(view, "\n")[0])
}

func shortAgents(n int) []hubclient.Agent {
	out := []hubclient.Agent{}
	for i := range n {
		out = append(out, hubclient.Agent{
			Name: fmt.Sprintf("agent%02d", i), Harness: "claude", Host: "mac", Connected: true, Label: "lead",
			LandlineVersion: "0.9.12", Folder: "/work/app", Branch: "main", Model: "opus", Effort: "high",
		})
	}
	return out
}

func rendered(agents []hubclient.Agent, width int) string {
	m, _ := send(newModel(), snapshotMsg{agents: agents, at: clock})
	m, _ = send(m, tea.WindowSizeMsg{Width: width, Height: 100})
	return m.View()
}

func TestColumnsAreDroppedFromTheLowestPriorityFirst(t *testing.T) {
	want := map[int]string{
		30:  "HANDLE LABEL STATE",
		50:  "HANDLE LABEL HARNESS STATE",
		70:  "HANDLE LABEL HARNESS STATE LANDLINE",
		90:  "HANDLE LABEL HARNESS STATE LANDLINE FOLDER",
		120: "HANDLE LABEL HARNESS STATE LANDLINE FOLDER BRANCH",
		160: "HANDLE LABEL HARNESS STATE LANDLINE FOLDER BRANCH MODEL EFFORT",
	}
	for width, titles := range want {
		got := strings.Join(headerTitles(rendered(shortAgents(3), width)), " ")
		if got != titles {
			t.Errorf("width %d: got %q want %q", width, got, titles)
		}
	}
}

func TestTheColumnSetDoesNotDependOnTheRows(t *testing.T) {
	for _, width := range []int{30, 50, 70, 90, 120, 160} {
		few := headerTitles(rendered(shortAgents(3), width))
		many := headerTitles(rendered(shortAgents(40), width))
		if strings.Join(few, " ") != strings.Join(many, " ") {
			t.Errorf("width %d: %v then %v", width, few, many)
		}
	}
}

func TestTruncatedCellsEndInAnEllipsisAndFolderKeepsItsEnd(t *testing.T) {
	agents := []hubclient.Agent{{
		Name: "alpha", Harness: "claude", Host: "mac", Connected: true, Label: "a-label-longer-than-sixteen",
		Folder: "/Users/someone/Projects/landline", Branch: "feature/a-branch-name-that-is-long",
	}}
	view := rendered(agents, 120)
	if !strings.Contains(view, "a-label-longer-…") {
		t.Fatalf("label not cut from the right with an ellipsis\n%s", view)
	}
	if !strings.Contains(view, "…s/someone/Projects/landline") || strings.Contains(view, "/Users/someone/Pro") {
		t.Fatalf("folder not cut from the left\n%s", view)
	}
	if !strings.Contains(view, "feature/a-branch-name…") {
		t.Fatalf("branch not cut from the right\n%s", view)
	}
}

func TestWideCharactersAreWidthAccountedInEveryColumn(t *testing.T) {
	cjk := "日本語のとても長い文字列日本語のとても長い文字列日本語"
	agents := []hubclient.Agent{{
		Name: "alpha", Harness: cjk, Host: "mac", Connected: true, Label: cjk, LandlineVersion: "日本語日本語日本語日本語",
		Folder: cjk, Branch: "🚀🚀🚀🚀🚀" + cjk, Model: "🚀" + cjk, Effort: "高高高高高高高高",
	}}
	m, _ := send(newModel(), snapshotMsg{agents: agents, at: clock})
	for _, width := range []int{20, 30, 40, 60, 80, 100, 120, 160, 200} {
		m, _ = send(m, tea.WindowSizeMsg{Width: width, Height: 20})
		for _, l := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(l); got > width {
				t.Fatalf("width %d: %q is %d wide", width, l, got)
			}
		}
	}
}

func TestAFailedPollOverAStaleListKeepsTheErrorFirstAndVisible(t *testing.T) {
	hub := "http://a-very-long-hub-name.internal.example.test:18787"
	for _, width := range []int{40, 60, 80} {
		m := New(hub, 2*time.Second, nil, func() time.Time { return clock })
		m, _ = send(m, tea.WindowSizeMsg{Width: width, Height: 20})
		m, _ = send(m, snapshotMsg{agents: sample, at: clock})
		m, _ = send(m, snapshotMsg{err: errors.New("cannot reach the hub: refused"), at: clock})
		lines := strings.Split(m.View(), "\n")
		status := lines[len(lines)-2]
		if !strings.HasPrefix(status, "error: cannot reach") {
			t.Fatalf("width %d: status %q", width, status)
		}
		if ansi.StringWidth(status) > width {
			t.Fatalf("width %d: status %q too wide", width, status)
		}
		if !strings.Contains(lines[0], "(stale)") {
			t.Fatalf("width %d: header %q is not marked stale", width, lines[0])
		}
	}
}

func TestAGoodPollRemovesTheStaleMark(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: sample, at: clock})
	m, _ = send(m, snapshotMsg{err: errors.New("boom"), at: clock})
	m, _ = send(m, snapshotMsg{agents: sample, at: clock})
	if strings.Contains(m.View(), "stale") {
		t.Fatal(m.View())
	}
}

func TestTheErrorIsShortenedLastNotTheHubAndTime(t *testing.T) {
	m := New("http://hub:1", 2*time.Second, nil, func() time.Time { return clock })
	m, _ = send(m, tea.WindowSizeMsg{Width: 60, Height: 20})
	m, _ = send(m, snapshotMsg{err: errors.New(strings.Repeat("boom ", 20)), at: clock})
	lines := strings.Split(m.View(), "\n")
	status := lines[len(lines)-2]
	if !strings.HasPrefix(status, "error: boom boom") || strings.Contains(status, "refreshed") {
		t.Fatalf("status %q", status)
	}
}

func TestTheFirstPollShowsLoadingAndTheRefreshMarker(t *testing.T) {
	view := newModel().View()
	if !strings.Contains(view, "Loading the agent list...") || !strings.Contains(view, "refreshing...") {
		t.Fatal(view)
	}
}

func TestTheRefreshKeyShowsTheMarkerWhileTheFetchRuns(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: sample, at: clock})
	if strings.Contains(m.View(), "refreshing") {
		t.Fatal(m.View())
	}
	m, _ = send(m, key("r"))
	if !strings.Contains(m.View(), "refreshing...") {
		t.Fatal(m.View())
	}
}

func TestTheRefreshedTimeShowsSeconds(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: sample, at: clock})
	if !strings.Contains(m.View(), "refreshed 10:30:05") {
		t.Fatal(m.View())
	}
}
