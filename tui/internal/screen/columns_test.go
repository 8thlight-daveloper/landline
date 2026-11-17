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
			ID: fmt.Sprintf("agent%02d", i), Harness: "claude", Host: "mac", Connected: true, Project: "lead",
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

const (
	projectColumnCap = 16
	taskColumnCap    = 24
	taskColumnMin    = 8
)

type columnBudget struct {
	title    string
	capWidth int
	minWidth int
}

func columnBudgets() []columnBudget {
	return []columnBudget{
		{"PROJECT", projectColumnCap, 6}, {"TASK", taskColumnCap, taskColumnMin}, {"HARNESS", 12, 6}, {"LANDLINE", 9, 5},
		{"FOLDER", 28, 10}, {"BRANCH", 22, 8}, {"MODEL", 18, 6}, {"EFFORT", 7, 5}, {"COMMIT", 13, 13},
	}
}

const (
	shapeAndIDWidth = 1 + 1 + 7
	gapWidth        = 2
)

type boundary struct {
	title      string
	firstShown int
	fullAt     int
}

func boundaries() []boundary {
	var out []boundary
	used := shapeAndIDWidth
	for _, c := range columnBudgets() {
		out = append(out, boundary{c.title, used + gapWidth + c.minWidth, used + gapWidth + c.capWidth})
		used += gapWidth + c.capWidth
	}
	return out
}

func sameColumns(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] == want[i] {
			continue
		}
		cut, found := strings.CutSuffix(got[i], "…")
		if !found || !strings.HasPrefix(want[i], cut) {
			return false
		}
	}
	return true
}

func TestColumnsAreAdmittedOnceTheirMinimumFitsInPriorityOrderWithTaskRightAfterProject(t *testing.T) {
	titles := []string{"ID"}
	for _, each := range boundaries() {
		before := append([]string{}, titles...)
		titles = append(titles, each.title)
		for width, want := range map[int][]string{each.firstShown - 1: before, each.firstShown: titles, each.fullAt: titles} {
			if got := headerTitles(rendered(shortAgents(3), width)); !sameColumns(got, want) {
				t.Errorf("width %d: got %v want %v", width, got, want)
			}
		}
		if each.fullAt > each.firstShown {
			if got := headerTitles(rendered(shortAgents(3), each.fullAt-1)); !sameColumns(got, titles) {
				t.Errorf("width %d: got %v want %v", each.fullAt-1, got, titles)
			}
		}
	}
	if got := headerTitles(rendered(shortAgents(3), 300)); !sameColumns(got, titles) {
		t.Errorf("width 300: got %v want %v", got, titles)
	}
	if got := headerTitles(rendered(shortAgents(3), 30)); !sameColumns(got, []string{"ID", "PROJECT"}) {
		t.Errorf("width 30: got %v", got)
	}
}

func TestTheColumnSetDoesNotDependOnTheRows(t *testing.T) {
	widths := []int{30, 240}
	for _, each := range boundaries() {
		widths = append(widths, each.firstShown-1, each.firstShown, each.fullAt)
	}
	for _, width := range widths {
		few := headerTitles(rendered(shortAgents(3), width))
		many := headerTitles(rendered(shortAgents(40), width))
		if strings.Join(few, " ") != strings.Join(many, " ") {
			t.Errorf("width %d: %v then %v", width, few, many)
		}
	}
}

func taskCellAt(t *testing.T, width int) string {
	t.Helper()
	agents := []hubclient.Agent{{ID: "agent00", Harness: "claude", Host: "mac", Connected: true, Project: "landline", Task: strings.Repeat("t", 80)}}
	row := rowLines(strings.Split(rendered(agents, width), "\n"))[0]
	start := strings.Index(row, "tt")
	end := strings.Index(row, "…") + len("…")
	if start < 0 || end <= start {
		t.Fatalf("width %d: no cut task in %q", width, row)
	}
	return row[start:end]
}

func TestTheTaskCellIsCutFromTheRightWithAnEllipsisAndGrowsWithTheWidthUpToItsCap(t *testing.T) {
	var task boundary
	for _, each := range boundaries() {
		if each.title == "TASK" {
			task = each
		}
	}
	for width := task.firstShown; width <= task.fullAt+3; width++ {
		want := min(width-(shapeAndIDWidth+gapWidth+projectColumnCap)-gapWidth, taskColumnCap)
		got := ansi.StringWidth(taskCellAt(t, width))
		if got != want {
			t.Errorf("width %d: the task cell is %d wide, want %d", width, got, want)
		}
	}
}

func TestAShortTaskIsShownWholeAnAgentWithNoTaskLeavesTheCellBlankAndNoLineIsWiderThanTheScreen(t *testing.T) {
	agents := []hubclient.Agent{
		{ID: "alpha", Harness: "claude", Host: "mac", Connected: true, Project: "landline", Task: "short task"},
		{ID: "bravo", Harness: "claude", Host: "mac", Connected: true, Project: "landline"},
	}
	for _, width := range []int{37, 60, 100, 140, 240} {
		view := rendered(agents, width)
		rows := rowLines(strings.Split(view, "\n"))
		if width >= 60 && !strings.Contains(rows[0], "short task") {
			t.Errorf("width %d: a short task is not shown whole\n%s", width, view)
		}
		for _, l := range strings.Split(view, "\n") {
			if ansi.StringWidth(l) > width {
				t.Errorf("width %d: %q is wider", width, l)
			}
		}
		if width < 60 {
			continue
		}
		header := []rune(ansi.Strip(strings.Split(view, "\n")[0]))
		row := []rune(ansi.Strip(rows[1]))
		from := runeIndex(header, "TASK")
		if cell := strings.TrimSpace(string(row[from:min(from+taskColumnMin, len(row))])); cell != "" {
			t.Errorf("width %d: the TASK cell of the row with no task is not blank: %q", width, string(row))
		}
	}
}

func runeIndex(text []rune, part string) int {
	return len([]rune(strings.SplitN(string(text), part, 2)[0]))
}

func TestARowWithATaskAndNoProjectShowsAnEmptyProjectCellThenTheTask(t *testing.T) {
	agents := []hubclient.Agent{{ID: "alpha", Harness: "claude", Host: "mac", Connected: true, Task: "docs only"}}
	view := rendered(agents, 140)
	header := []rune(ansi.Strip(strings.Split(view, "\n")[0]))
	row := []rune(ansi.Strip(rowLines(strings.Split(view, "\n"))[0]))
	project, task := runeIndex(header, "PROJECT"), runeIndex(header, "TASK")
	if project >= len(header) || task >= len(header) {
		t.Fatalf("the header lacks PROJECT or TASK: %q", string(header))
	}
	if got := strings.TrimSpace(string(row[project:task])); got != "" {
		t.Errorf("the PROJECT cell is not empty: %q in %q", got, string(row))
	}
	if !strings.HasPrefix(string(row[task:]), "docs only") {
		t.Errorf("the task is not under TASK: %q", string(row))
	}
}

func TestTheTaskSitsBetweenTheProjectAndTheHarness(t *testing.T) {
	agents := []hubclient.Agent{{ID: "alpha", Harness: "claude", Host: "mac", Connected: true, Project: "landline", Task: "write tests"}}
	row := rowLines(strings.Split(rendered(agents, 140), "\n"))[0]
	project, task, harness := strings.Index(row, "landline"), strings.Index(row, "write tests"), strings.Index(row, "claude")
	if !(0 < project && project < task && task < harness) {
		t.Fatalf("the order is wrong in %q", row)
	}
}

func TestATaskWithControlCharactersNeverReachesTheScreenRaw(t *testing.T) {
	agents := []hubclient.Agent{{ID: "alpha", Harness: "claude", Host: "mac", Connected: true, Project: "p", Task: "a\x1b[2Jb\nc\u202ed"}}
	view := rendered(agents, 140)
	if strings.Contains(view, "\x1b[2J") || strings.Contains(view, "\u202e") || strings.Contains(view, "a\nc") {
		t.Fatalf("raw text reached the screen: %q", view)
	}
}

func TestTruncatedCellsEndInAnEllipsisAndFolderKeepsItsEnd(t *testing.T) {
	agents := []hubclient.Agent{{
		ID: "alpha", Harness: "claude", Host: "mac", Connected: true, Project: "a-project-longer-than-sixteen",
		Folder: "/Users/someone/Projects/landline", Branch: "feature/a-branch-name-that-is-long",
	}}
	view := rendered(agents, 140)
	if !strings.Contains(view, ansi.Truncate("a-project-longer-than-sixteen", projectColumnCap, "…")) {
		t.Fatalf("project not cut from the right with an ellipsis\n%s", view)
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
		ID: "alpha", Harness: cjk, Host: "mac", Connected: true, Project: cjk, LandlineVersion: "日本語日本語日本語日本語",
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
		m, _ = send(m, snapshotMsg{err: errors.New("cannot reach the switchboard: refused"), at: clock})
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
