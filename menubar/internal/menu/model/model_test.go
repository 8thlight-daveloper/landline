package model

import (
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/menubar/internal/agentsview"
	"github.com/8thlight-daveloper/landline/menubar/internal/buildinfo"
)

const commit = "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"

func app() buildinfo.Build { return buildinfo.Build{Version: "0.2.0", Commit: commit} }

func local(reachable bool) Input {
	return Input{Address: "http://127.0.0.1:8787", Local: true, Reachable: reachable, App: app(), LandlineFound: true}
}

const tail = `Harnesses
  Set up Claude Code
  Remove from Claude Code
  Set up Codex
  Remove from Codex
---
Menubar 0.2.0 (a1b2c3d) (disabled)
About Landline
Refresh
---
Quit
`

func TestTextOfAReachableSwitchboardWithAgents(t *testing.T) {
	in := local(true)
	in.Switchboard = Switchboard{Known: true, Build: app()}
	in.Agents = []agentsview.Row{
		{ID: "alpha", Project: "amy-docs", Task: "menu bar app for ADR 0140", Harness: "claude-code", Host: "mac", State: agentsview.Connected, Folder: "/work/docs", Branch: "main"},
		{ID: "delta", Harness: "claude-code", Host: "mac", State: agentsview.Disconnected},
	}
	want := `icon: jack 1
● amy-docs alpha
  State: connected (disabled)
  ID: alpha (disabled)
  Project: amy-docs (disabled)
  Task: menu bar app for ADR 0140 (disabled)
  Harness: claude-code (disabled)
  Host: mac (disabled)
  Folder: /work/docs (disabled)
  Branch: main (disabled)
  ---
  Copy id
○ delta
  State: disconnected (disabled)
  ID: delta (disabled)
  Harness: claude-code (disabled)
  Host: mac (disabled)
  ---
  Copy id
---
Switchboard
  Running at http://127.0.0.1:8787 (disabled)
  ---
  Start (disabled)
  Stop
  Restart
  Rotate secret…
  ---
  Switchboard 0.2.0 (a1b2c3d) (disabled)
` + tail
	if got := Build(in).Text(); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTextOfAReachableSwitchboardWithNoAgentsHasNoSeparatorAfterTheIcon(t *testing.T) {
	in := local(true)
	in.App = buildinfo.Build{Version: "dev"}
	in.Switchboard = Switchboard{Known: true, Build: buildinfo.Build{Version: "dev"}}
	got := Build(in).Text()
	if !strings.HasPrefix(got, "icon: jack\nSwitchboard\n  Running at http://127.0.0.1:8787 (disabled)\n") || !strings.Contains(got, "  Switchboard dev (disabled)\n") || !strings.Contains(got, "Menubar dev (disabled)\n") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestTextOfAnUnreachableSwitchboard(t *testing.T) {
	want := `icon: jack dimmed
Switchboard unreachable (disabled)
---
Switchboard
  Not running (disabled)
  ---
  Start
  Stop (disabled)
  Restart (disabled)
  Rotate secret… (disabled)
  ---
  Switchboard (build unknown) (disabled)
` + tail
	if got := Build(local(false)).Text(); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAnUnreachableSwitchboardListsNoAgentsWhateverTheInputHolds(t *testing.T) {
	in := local(false)
	in.Agents = []agentsview.Row{{ID: "alpha", State: agentsview.Connected}}
	if got := Build(in).Text(); strings.Contains(got, "alpha") || strings.HasPrefix(got, "icon: jack 1") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestARemoteSwitchboardDisablesAllFourServerItems(t *testing.T) {
	for _, reachable := range []bool{false, true} {
		in := local(reachable)
		in.Address, in.Local = "http://tailnet-mac:8787", false
		got := Build(in).Text()
		for _, want := range []string{"  Remote switchboard at http://tailnet-mac:8787 (disabled)\n", "  Start (disabled)\n", "  Stop (disabled)\n", "  Restart (disabled)\n", "  Rotate secret… (disabled)\n"} {
			if !strings.Contains(got, want) {
				t.Errorf("reachable %v lacks %q:\n%s", reachable, want, got)
			}
		}
	}
}

func TestWhenLandlineIsMissingTheServerItemsAreDisabledAndTheHarnessItemsStayEnabled(t *testing.T) {
	in := local(true)
	in.LandlineFound = false
	got := Build(in).Text()
	for _, want := range []string{"  landline not found. Run just install (disabled)\n", "  Start (disabled)\n", "  Stop (disabled)\n", "  Restart (disabled)\n", "  Rotate secret… (disabled)\n", "  Set up Claude Code\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
}

func TestANoticeReplacesTheFirstLineAndBusyDisablesEveryRunItem(t *testing.T) {
	in := local(true)
	in.Notice = "Failed: boom"
	in.Busy = true
	got := Build(in).Text()
	for _, want := range []string{"  Failed: boom (disabled)\n", "  Stop (disabled)\n", "  Restart (disabled)\n", "  Rotate secret… (disabled)\n", "  Set up Claude Code (disabled)\n", "  Remove from Codex (disabled)\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Running at") {
		t.Errorf("the notice did not replace the line:\n%s", got)
	}
}

func TestTheBuildLineSaysWhenTheCommitDiffersOnlyThen(t *testing.T) {
	cases := map[string]struct {
		known     bool
		build     buildinfo.Build
		appCommit string
		want      string
		reachable bool
	}{
		"same":              {true, buildinfo.Build{Version: "0.2.0", Commit: commit}, commit, "Switchboard 0.2.0 (a1b2c3d)", true},
		"other":             {true, buildinfo.Build{Version: "0.2.0", Commit: commit}, "f" + commit[1:], "Switchboard 0.2.0 (a1b2c3d) (differs from this app)", true},
		"dirty":             {true, buildinfo.Build{Version: "0.2.0", Commit: commit + "-dirty"}, commit, "Switchboard 0.2.0 (a1b2c3d-dirty) (differs from this app)", true},
		"app has no commit": {true, buildinfo.Build{Version: "0.2.0", Commit: commit}, "", "Switchboard 0.2.0 (a1b2c3d)", true},
		"both no commit":    {true, buildinfo.Build{Version: "0.2.0"}, "", "Switchboard 0.2.0", true},
		"unknown":           {false, buildinfo.Build{}, commit, "Switchboard (build unknown)", true},
		"no version":        {true, buildinfo.Build{Commit: commit}, commit, "Switchboard (build unknown)", true},
		"unreachable":       {true, buildinfo.Build{Version: "0.2.0", Commit: commit}, commit, "Switchboard (build unknown)", false},
	}
	for name, c := range cases {
		in := local(c.reachable)
		in.App.Commit = c.appCommit
		in.Switchboard = Switchboard{Known: c.known, Build: c.build}
		if got := Build(in).Text(); !strings.Contains(got, "  "+c.want+" (disabled)\n") {
			t.Errorf("%s: lacks %q:\n%s", name, c.want, got)
		}
	}
}

func TestEveryLineIsOneLineAndLongValuesAreCut(t *testing.T) {
	long := strings.Repeat("x", 5000)
	in := local(true)
	in.Agents = []agentsview.Row{{ID: "alpha", Project: long, Folder: "/" + long, State: agentsview.Connected}}
	in.Notice = "Failed: a\nb\x1b[2J"
	got := Build(in).Text()
	for _, l := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		if len([]rune(l)) > maxTitleRunes+len(disabledText)+len("○ ")+len("State: disconnected")+len(indent)+len("Folder: ") {
			t.Fatalf("a line is %d runes", len([]rune(l)))
		}
	}
	if strings.ContainsAny(got, "\x1b\r\t") || !strings.Contains(got, "  Failed: a b [2J (disabled)\n") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestIconCountsConnectedAgentsOnly(t *testing.T) {
	in := local(true)
	in.Agents = []agentsview.Row{{ID: "a", State: agentsview.Connected}, {ID: "b", State: agentsview.Disconnected}}
	if got := Build(in).Icon; got != (Icon{Count: 1}) || got.CountText() != "1" {
		t.Fatalf("%+v", got)
	}
	if got := Build(local(true)).Icon; got.CountText() != "" {
		t.Fatalf("%+v", got)
	}
	if got := Build(local(false)).Icon; !got.Dimmed || got.CountText() != "" {
		t.Fatalf("%+v", got)
	}
}

func TestRotateSecretAsksFirstAndRunsTheCommandWithNoFlags(t *testing.T) {
	var rotate Item
	for _, item := range Build(local(true)).Items[0].Children {
		if item.Title == "Rotate secret…" {
			rotate = item
		}
	}
	if rotate.Disabled || strings.Join(rotate.Action.Args, " ") != "server rotate-secret" || rotate.Action.Kind != Run {
		t.Fatalf("%+v", rotate)
	}
	if rotate.Action.Confirm != "Rotate the switchboard secret? Every agent must register again." || rotate.Action.ConfirmAccept != "Rotate" {
		t.Fatalf("%+v", rotate.Action)
	}
}

func TestOnlyRotateSecretAsksBeforeItRuns(t *testing.T) {
	var walk func(items []Item)
	walk = func(items []Item) {
		for _, item := range items {
			if (item.Action.Confirm != "") != (item.Title == "Rotate secret…") {
				t.Errorf("%q has the confirmation %q", item.Title, item.Action.Confirm)
			}
			walk(item.Children)
		}
	}
	walk(Build(local(true)).Items)
}

func TestTheActionsAreTheFixedCommandsAndNothingFromTheSwitchboard(t *testing.T) {
	in := local(true)
	in.Agents = []agentsview.Row{{ID: "evil; rm -rf /", State: agentsview.Connected}}
	var runs [][]string
	var walk func(items []Item)
	walk = func(items []Item) {
		for _, item := range items {
			if item.Action.Kind == Run {
				runs = append(runs, item.Action.Args)
			}
			walk(item.Children)
		}
	}
	walk(Build(in).Items)
	want := []string{
		"server up", "server down", "server restart", "server rotate-secret",
		"setup claude", "setup claude --remove", "setup codex", "setup codex --remove",
	}
	if len(runs) != len(want) {
		t.Fatalf("%v", runs)
	}
	for i := range want {
		if strings.Join(runs[i], " ") != want[i] {
			t.Errorf("run %d is %q, want %q", i, runs[i], want[i])
		}
	}
}

func TestCopyIDCopiesTheIDAndNothingElse(t *testing.T) {
	in := local(true)
	in.Agents = []agentsview.Row{{ID: "alpha", Project: "amy", Folder: "/f", State: agentsview.Connected}}
	row := Build(in).Items[0]
	copyItem := row.Children[len(row.Children)-1]
	if copyItem.Title != "Copy id" || copyItem.Action.Kind != Copy || copyItem.Action.Text != "alpha" {
		t.Fatalf("%+v", copyItem)
	}
}

func TestSuccessLinesNameTheHarness(t *testing.T) {
	var dones []string
	for _, item := range harnessItems(false) {
		dones = append(dones, item.Action.Done)
	}
	want := "Claude Code set up. Start a new session.|Claude Code removed.|Codex set up. Start a new session.|Codex removed."
	if strings.Join(dones, "|") != want {
		t.Fatal(dones)
	}
}

func TestARowIsTitledByProjectThenIDAndAnAgentWithoutAProjectShowsItsIDAlone(t *testing.T) {
	in := local(true)
	in.Agents = []agentsview.Row{
		{ID: "a3f9", Project: "landline", State: agentsview.Connected},
		{ID: "b7c2", State: agentsview.Connected},
		{ID: "c0de", Project: "landline", State: agentsview.Disconnected},
	}
	menu := Build(in)
	for i, want := range []string{"landline a3f9", "b7c2", "landline c0de"} {
		if got := strings.TrimLeft(menu.Items[i].Title, "\u25cf\u25cb "); got != want {
			t.Errorf("row %d is titled %q, want the name %q after any text shape", i, menu.Items[i].Title, want)
		}
	}
	lines := strings.Split(menu.Text(), "\n")
	for _, want := range []string{"\u25cf landline a3f9", "\u25cf b7c2", "\u25cb landline c0de"} {
		found := false
		for _, line := range lines {
			found = found || line == want
		}
		if !found {
			t.Errorf("the menu text has no row line %q:\n%s", want, menu.Text())
		}
	}
}

func TestTheSubmenuHasStateThenIDThenProjectThenTaskAndLeavesOutAProjectOrTaskThatIsNotSet(t *testing.T) {
	titles := func(row agentsview.Row) []string {
		in := local(true)
		in.Agents = []agentsview.Row{row}
		var out []string
		for _, child := range Build(in).Items[0].Children {
			out = append(out, child.Title)
		}
		return out
	}
	full := titles(agentsview.Row{ID: "a3f9", Project: "landline", Task: "write tests", Harness: "claude-code", Host: "mac", State: agentsview.Connected})
	want := []string{"State: connected", "ID: a3f9", "Project: landline", "Task: write tests", "Harness: claude-code", "Host: mac", "", "Copy id"}
	if strings.Join(full, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", full, want)
	}
	noProject := titles(agentsview.Row{ID: "a3f9", Task: "docs only", State: agentsview.Connected})
	if want := "State: connected|ID: a3f9|Task: docs only||Copy id"; strings.Join(noProject, "|") != want {
		t.Fatalf("a task and no project: got %q, want %q", noProject, want)
	}
	bare := titles(agentsview.Row{ID: "a3f9", Harness: "claude-code", Host: "mac", State: agentsview.Connected})
	for _, title := range bare {
		if strings.HasPrefix(title, "Project") || strings.HasPrefix(title, "Task") || strings.Contains(title, "Handle") || strings.Contains(title, "Label") {
			t.Errorf("an unset or old item shows: %q", title)
		}
	}
}

func TestATaskAndAProjectAreOneLineEachAndCutWhenLong(t *testing.T) {
	long := strings.Repeat("t", 5000)
	in := local(true)
	in.Agents = []agentsview.Row{{ID: "alpha", Project: "p", Task: long, State: agentsview.Connected}}
	for _, l := range strings.Split(strings.TrimSuffix(Build(in).Text(), "\n"), "\n") {
		if len([]rune(l)) > maxTitleRunes+len(disabledText)+len("\u25cb ")+len("State: disconnected")+len(indent)+len("Folder: ") {
			t.Fatalf("a line is %d runes", len([]rune(l)))
		}
	}
}

func TestAnAgentWithATaskAndNoProjectIsTitledByItsIDAlone(t *testing.T) {
	in := local(true)
	in.Agents = []agentsview.Row{{ID: "a3f9", Task: "docs only", State: agentsview.Connected}}
	if got := strings.TrimLeft(Build(in).Items[0].Title, "\u25cf\u25cb "); got != "a3f9" {
		t.Fatalf("the title is %q", got)
	}
}
