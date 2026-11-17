package screen

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

const (
	connectedShape    = "●"
	disconnectedShape = "○"
	legendText        = "● connected  ○ disconnected"
)

func viewAt(agents []hubclient.Agent, width, height int) []string {
	m, _ := send(newModel(), snapshotMsg{agents: agents, at: clock})
	m, _ = send(m, tea.WindowSizeMsg{Width: width, Height: height})
	return strings.Split(m.View(), "\n")
}

func rowLines(lines []string) []string {
	var rows []string
	for _, l := range lines[1:] {
		if l == "" || strings.HasPrefix(l, "+") {
			break
		}
		rows = append(rows, l)
	}
	return rows
}

func TestEachShapeIsCountedAsOneCell(t *testing.T) {
	for _, shape := range []string{connectedShape, disconnectedShape} {
		if got := ansi.StringWidth(shape); got != 1 {
			t.Errorf("%q is counted as %d cells", shape, got)
		}
	}
}

func TestTheFirstColumnIsABlankHeaderCellThenTheGapThenTheHandleAndTheRowsStartWithTheirShape(t *testing.T) {
	lines := viewAt(sample, 200, 20)
	if !strings.HasPrefix(lines[0], "   HANDLE") {
		t.Errorf("the header is %q, want a blank cell, the gap of 2, then HANDLE", lines[0])
	}
	if strings.Contains(lines[0], "STATE") {
		t.Errorf("the header still has STATE: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], connectedShape+"  alpha") || !strings.HasPrefix(lines[2], disconnectedShape+"  bravo") {
		t.Errorf("the rows are %q and %q", lines[1], lines[2])
	}
}

func TestRowsNeverSayConnectedOrDisconnectedAndOnlyTheLegendDoes(t *testing.T) {
	lines := viewAt(sample, 200, 20)
	for _, l := range rowLines(lines) {
		if strings.Contains(l, "connected") {
			t.Errorf("a row says a state word: %q", l)
		}
	}
	legends := 0
	for _, l := range lines {
		if strings.Contains(l, "connected") {
			legends++
			if l != legendText {
				t.Errorf("the legend is %q, want %q", l, legendText)
			}
		}
	}
	if legends != 1 {
		t.Errorf("%d lines say a state word, want the legend alone", legends)
	}
}

func TestTheColumnsDropInTheOldOrderWithNoStateColumnAtTheNewBreakpoints(t *testing.T) {
	want := map[int]string{
		28:  "HANDLE LABEL",
		42:  "HANDLE LABEL HARNESS",
		53:  "HANDLE LABEL HARNESS LANDLINE",
		83:  "HANDLE LABEL HARNESS LANDLINE FOLDER",
		107: "HANDLE LABEL HARNESS LANDLINE FOLDER BRANCH",
		127: "HANDLE LABEL HARNESS LANDLINE FOLDER BRANCH MODEL",
		136: "HANDLE LABEL HARNESS LANDLINE FOLDER BRANCH MODEL EFFORT",
		150: "HANDLE LABEL HARNESS LANDLINE FOLDER BRANCH MODEL EFFORT",
		151: "HANDLE LABEL HARNESS LANDLINE FOLDER BRANCH MODEL EFFORT COMMIT",
		240: "HANDLE LABEL HARNESS LANDLINE FOLDER BRANCH MODEL EFFORT COMMIT",
	}
	for width, titles := range want {
		got := strings.Join(headerTitles(rendered(shortAgents(3), width)), " ")
		if got != titles {
			t.Errorf("width %d: got %q want %q", width, got, titles)
		}
	}
}

func TestTheShapeStaysAndTheHandleIsCutAtEveryNarrowWidth(t *testing.T) {
	agents := []hubclient.Agent{{Name: "amber-falcon-5fe1", Harness: "claude", Host: "mac", Connected: true}}
	for width := 1; width <= 4; width++ {
		lines := viewAt(agents, width, 20)
		rows := rowLines(lines)
		if len(rows) != 1 {
			t.Fatalf("width %d: the shape is gone:\n%s", width, strings.Join(lines, "\n"))
		}
		for _, r := range rows[0] {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				t.Errorf("width %d: part of the handle shows in %q", width, rows[0])
			}
		}
		for _, l := range lines {
			if ansi.StringWidth(l) > width {
				t.Errorf("width %d: %q is wider", width, l)
			}
		}
	}
	for width := 5; width <= 12; width++ {
		rows := rowLines(viewAt(agents, width, 20))
		tailMissing := width >= 8 && !strings.HasSuffix(strings.TrimRight(rows[0], " "), "5fe1")
		if len(rows) != 1 || !strings.HasPrefix(rows[0], connectedShape+"  ") || tailMissing {
			t.Errorf("width %d: the row is %v", width, rows)
		}
		if len(rows) == 1 && ansi.StringWidth(rows[0]) > width {
			t.Errorf("width %d: %q is wider", width, rows[0])
		}
	}
}

func TestTheLegendIsTheLastBodyLineAboveTheStatusAndBelowTheSpacer(t *testing.T) {
	lines := viewAt(sample, 100, 100)
	n := len(lines)
	if lines[n-4] != "" || lines[n-3] != legendText || !strings.Contains(lines[n-2], "switchboard http://hub:1") || lines[n-1] != "q quit  r refresh" {
		t.Errorf("the footer is %q", lines[n-4:])
	}
}

func TestTheLegendNamesBothStatesWhicheverAreInTheRows(t *testing.T) {
	for name, agents := range map[string][]hubclient.Agent{
		"only connected":    {{Name: "alpha", Harness: "claude", Host: "mac", Connected: true}},
		"only disconnected": {{Name: "bravo", Harness: "claude", Host: "mac"}},
	} {
		lines := viewAt(agents, 100, 100)
		if lines[len(lines)-3] != legendText {
			t.Errorf("%s: %q", name, lines[len(lines)-3])
		}
	}
}

func TestTheLegendShowsOnlyWhenTheTableShowsRows(t *testing.T) {
	empty, _ := send(newModel(), snapshotMsg{agents: nil, at: clock})
	failed, _ := send(newModel(), snapshotMsg{err: fmt.Errorf("boom")})
	for name, view := range map[string]string{"loading": newModel().View(), "empty": empty.View(), "failed first poll": failed.View()} {
		if strings.Contains(view, connectedShape) || strings.Contains(view, disconnectedShape) || strings.Contains(view, "connected") {
			t.Errorf("%s shows the legend or a shape:\n%s", name, view)
		}
	}
	stale, _ := send(newModel(), snapshotMsg{agents: sample, at: clock})
	stale, _ = send(stale, snapshotMsg{err: fmt.Errorf("boom"), at: clock})
	if !strings.Contains(stale.View(), legendText) {
		t.Errorf("a stale list shows rows, so it shows the legend:\n%s", stale.View())
	}
}

func TestAShortWindowDropsTheLegendThenTheSpacerThenTheKeyLineAndKeepsTheStatusLongest(t *testing.T) {
	one := []hubclient.Agent{{Name: "alpha", Harness: "claude", Host: "mac", Connected: true}}
	status := "switchboard http://hub:1"
	cases := map[int]struct {
		lines  int
		legend bool
		spacer bool
		keys   bool
	}{
		6: {6, true, true, true},
		5: {5, false, true, true},
		4: {4, false, false, true},
		3: {3, false, false, false},
	}
	for height, c := range cases {
		lines := viewAt(one, 100, height)
		view := strings.Join(lines, "\n")
		if len(lines) != c.lines || strings.Contains(view, legendText) != c.legend || (lines[len(lines)-1] == "q quit  r refresh") != c.keys {
			t.Errorf("height %d:\n%s", height, view)
		}
		hasSpacer := false
		for _, l := range lines[1:] {
			if l == "" {
				hasSpacer = true
			}
		}
		if hasSpacer != c.spacer || !strings.Contains(view, status) {
			t.Errorf("height %d: spacer %v, status %v:\n%s", height, hasSpacer, strings.Contains(view, status), view)
		}
	}
	for height := 1; height <= 8; height++ {
		view := strings.Join(viewAt(agentsNamed(30), 100, height), "\n")
		if !strings.Contains(view, status) {
			t.Errorf("height %d lost the status line:\n%s", height, view)
		}
	}
}

func TestTheLegendIsNotARowAndTheMoreCountDoesNotCountIt(t *testing.T) {
	lines := viewAt(agentsNamed(30), 100, 8)
	if len(lines) != 8 {
		t.Fatalf("%d lines:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if !strings.HasPrefix(lines[0], "   HANDLE") || !strings.HasPrefix(lines[1], connectedShape+"  agent00") || !strings.HasPrefix(lines[2], connectedShape+"  agent01") {
		t.Errorf("the table is %q", lines[:3])
	}
	if lines[3] != "+28 more" {
		t.Errorf("line 3 is %q, want +28 more, which counts the 30 agents less the 2 shown and not the legend", lines[3])
	}
	if lines[4] != "" || lines[5] != legendText || !strings.Contains(lines[6], "switchboard http://hub:1") || lines[7] != "q quit  r refresh" {
		t.Errorf("the footer is %q, want the blank spacer, the legend, the status, and the keys", lines[4:])
	}
}

func TestTheLegendIsCutWithAnEllipsisWhenTheTerminalIsNarrower(t *testing.T) {
	for _, width := range []int{26, 20, 12, 5} {
		lines := viewAt(sample, width, 100)
		got := lines[len(lines)-3]
		if want := ansi.Truncate(legendText, width, "…"); got != want {
			t.Errorf("width %d: the legend is %q, want %q", width, got, want)
		}
		for _, l := range lines {
			if ansi.StringWidth(l) > width {
				t.Errorf("width %d: %q is wider", width, l)
			}
		}
	}
}

func TestUnderTheAsciiProfileRowsCarryNoEscapesAndTheShapesStillDifferInTheSameWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	lines := viewAt(sample, 100, 20)
	rows := rowLines(lines)
	if len(rows) != 2 || !strings.HasPrefix(rows[0], connectedShape) || !strings.HasPrefix(rows[1], disconnectedShape) {
		t.Fatalf("%v", rows)
	}
	if strings.Contains(strings.Join(lines, "\n"), "\x1b") {
		t.Errorf("an escape sequence under the Ascii profile:\n%q", lines)
	}
	if connectedShape == disconnectedShape || ansi.StringWidth(connectedShape) != ansi.StringWidth(disconnectedShape) {
		t.Errorf("the shapes must differ and have the same width")
	}
}

func TestUnderTheAnsiProfileTheConnectedShapeIsGreenAndTheDisconnectedShapeIsFaint(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	lines := viewAt(sample, 100, 20)
	if !strings.Contains(lines[1], "\x1b[32m"+connectedShape) {
		t.Errorf("the connected row is %q", lines[1])
	}
	if !strings.Contains(lines[2], "\x1b[2m"+disconnectedShape) {
		t.Errorf("the disconnected row is %q", lines[2])
	}
}

func capFixture() []hubclient.Agent {
	return []hubclient.Agent{{
		Name: "agent00", Harness: strings.Repeat("h", 12), Host: "mac", Connected: true, Label: strings.Repeat("l", 16),
		LandlineVersion: strings.Repeat("v", 9), Folder: strings.Repeat("f", 28), Branch: strings.Repeat("b", 22),
		Model: strings.Repeat("m", 18), Effort: strings.Repeat("e", 7), LandlineCommit: "0123456789abcdef0123456789abcdef01234567",
	}}
}

func TestEachColumnShowsItsFullValueAtItsBreakpointAndIsCutOneCellBelow(t *testing.T) {
	cases := []struct {
		width int
		value string
	}{
		{28, strings.Repeat("l", 16)}, {42, strings.Repeat("h", 12)}, {53, strings.Repeat("v", 9)}, {83, strings.Repeat("f", 28)},
		{107, strings.Repeat("b", 22)}, {127, strings.Repeat("m", 18)}, {136, strings.Repeat("e", 7)},
	}
	for _, c := range cases {
		at := strings.Join(rowLines(viewAt(capFixture(), c.width, 20)), "\n")
		below := strings.Join(rowLines(viewAt(capFixture(), c.width-1, 20)), "\n")
		if !strings.Contains(at, c.value) {
			t.Errorf("width %d does not show %q in full:\n%s", c.width, c.value, at)
		}
		if strings.Contains(below, c.value) {
			t.Errorf("width %d already shows %q in full:\n%s", c.width-1, c.value, below)
		}
	}
}

func TestTheCommitColumnIsAbsentAtOneHundredFiftyAndPresentAtOneHundredFiftyOne(t *testing.T) {
	if titles := headerTitles(viewAt(capFixture(), 150, 20)[0] + "\n"); strings.Contains(strings.Join(titles, " "), "COMMIT") {
		t.Errorf("COMMIT shows at 150: %v", titles)
	}
	if titles := headerTitles(viewAt(capFixture(), 151, 20)[0] + "\n"); !strings.Contains(strings.Join(titles, " "), "COMMIT") {
		t.Errorf("COMMIT is missing at 151: %v", titles)
	}
}

func TestTheLabelColumnAppearsCutFromWidthEighteenAndNotBefore(t *testing.T) {
	label := "a-label-longer-than-sixteen"
	agents := []hubclient.Agent{{Name: "agent00", Harness: "claude", Host: "mac", Connected: true, Label: label}}
	for width := 10; width <= 17; width++ {
		if titles := headerTitles(strings.Join(viewAt(agents, width, 20), "\n")); strings.Join(titles, " ") != "HANDLE" {
			t.Errorf("width %d shows %v, want the handle alone", width, titles)
		}
	}
	for width := 18; width <= 27; width++ {
		lines := viewAt(agents, width, 20)
		if titles := strings.Join(headerTitles(strings.Join(lines, "\n")), " "); titles != "HANDLE LABEL" {
			t.Errorf("width %d shows %q", width, titles)
		}
		want := ansi.Truncate(label, width-12, "…")
		if !strings.Contains(rowLines(lines)[0], want) {
			t.Errorf("width %d: the row %q lacks the label cut to %q", width, rowLines(lines)[0], want)
		}
	}
}
