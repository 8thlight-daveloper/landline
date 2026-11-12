package screen

import (
	"fmt"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
	tea "github.com/charmbracelet/bubbletea"
)

func longAgents() []hubclient.Agent {
	long := strings.Repeat("x", 40)
	out := []hubclient.Agent{}
	for _, name := range []string{"amber-falcon-1", "amber-falcon-2"} {
		out = append(out, hubclient.Agent{
			Name: name, Harness: "claude", Host: "mac", Connected: true, Label: "review-" + long,
			LandlineVersion: "0.9.12", Folder: "/work/" + long, Branch: "feature/" + long, Model: "model-" + long, Effort: "high",
		})
	}
	return out
}

func TestHandlesAreNeverTruncatedAtEightyColumns(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: longAgents(), at: clock})
	m, _ = send(m, tea.WindowSizeMsg{Width: 80, Height: 20})
	view := m.View()
	for _, want := range []string{"amber-falcon-1", "amber-falcon-2", "connected"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in\n%s", want, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if lipglossWidth(line) > 80 {
			t.Fatalf("line %q is wider than 80", line)
		}
	}
}

func TestNarrowTerminalsNeverPanicOrOverflowWithLongHandles(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: longAgents(), at: clock})
	for _, width := range []int{20, 40} {
		next, _ := send(m, tea.WindowSizeMsg{Width: width, Height: 20})
		for _, line := range strings.Split(next.View(), "\n") {
			if got := lipglossWidth(line); got > width {
				t.Fatalf("width %d: line %q is %d wide", width, line, got)
			}
		}
	}
}

func TestWhenHandleAndStateCannotBothFitTheHandleStaysWholeAndTheStateGoes(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: longAgents(), at: clock})
	m, _ = send(m, tea.WindowSizeMsg{Width: 28, Height: 20})
	if view := m.View(); !strings.Contains(view, "amber-falcon-1") || !strings.Contains(view, "connected") {
		t.Fatal(view)
	}
	m, _ = send(m, tea.WindowSizeMsg{Width: 20, Height: 20})
	if view := m.View(); !strings.Contains(view, "amber-falcon-1") || strings.Contains(view, "connected") {
		t.Fatal(view)
	}
}

func TestAHandleWiderThanTheScreenKeepsItsTail(t *testing.T) {
	agents := []hubclient.Agent{{Name: "amber-falcon-5fe1", Harness: "claude", Host: "mac", Connected: true}}
	m, _ := send(newModel(), snapshotMsg{agents: agents, at: clock})
	m, _ = send(m, tea.WindowSizeMsg{Width: 10, Height: 20})
	view := m.View()
	if !strings.Contains(view, "…lcon-5fe1") && !strings.Contains(view, "…con-5fe1") {
		t.Fatal(view)
	}
	if strings.Contains(view, "amber") || strings.Contains(view, "connected") {
		t.Fatal(view)
	}
}

func TestAtLeastOneAgentRowShowsWhenTheHeightAllowsItAndTheMoreCountIsExact(t *testing.T) {
	m, _ := send(newModel(), snapshotMsg{agents: agentsNamed(30), at: clock})
	for height := 3; height <= 8; height++ {
		next, _ := send(m, tea.WindowSizeMsg{Width: 100, Height: height})
		view := next.View()
		lines := strings.Split(view, "\n")
		if len(lines) > height {
			t.Fatalf("height %d: %d lines", height, len(lines))
		}
		rows := 0
		more := -1
		for _, l := range lines {
			if strings.HasPrefix(l, "agent") {
				rows++
			}
			if strings.HasPrefix(l, "+") {
				fmt.Sscanf(l, "+%d more", &more)
			}
		}
		if height >= 4 && rows < 1 {
			t.Fatalf("height %d: no agent row in\n%s", height, view)
		}
		if height >= 4 && more != 30-rows {
			t.Fatalf("height %d: shows %d rows and says %d more\n%s", height, rows, more, view)
		}
		if !strings.Contains(view, "hub http://hub:1") {
			t.Fatalf("height %d: no status line", height)
		}
	}
}
