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
	return Input{Address: "http://127.0.0.1:8787", Reachable: reachable, App: app(), LandlineFound: true}
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
		{Handle: "alpha", Label: "amy-docs", Harness: "claude-code", Host: "mac", Marker: agentsview.Active, Folder: "/work/docs", Branch: "main"},
		{Handle: "delta", Harness: "claude-code", Host: "mac", Marker: agentsview.Disconnected},
	}
	want := `icon: jack 1
amy-docs [active]
  Handle: alpha (disabled)
  Harness: claude-code (disabled)
  Host: mac (disabled)
  Folder: /work/docs (disabled)
  Branch: main (disabled)
  ---
  Copy handle
delta [disconnected]
  Handle: delta (disabled)
  Harness: claude-code (disabled)
  Host: mac (disabled)
  ---
  Copy handle
---
Switchboard
  Running at http://127.0.0.1:8787 (disabled)
  ---
  Start (disabled)
  Stop
  Restart
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
  ---
  Switchboard (build unknown) (disabled)
` + tail
	if got := Build(local(false)).Text(); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAnUnreachableSwitchboardListsNoAgentsWhateverTheInputHolds(t *testing.T) {
	in := local(false)
	in.Agents = []agentsview.Row{{Handle: "alpha", Marker: agentsview.Active}}
	if got := Build(in).Text(); strings.Contains(got, "alpha") || strings.HasPrefix(got, "icon: jack 1") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestARemoteSwitchboardDisablesAllThreeServerItems(t *testing.T) {
	for _, reachable := range []bool{false, true} {
		in := local(reachable)
		in.Address = "http://tailnet-mac:8787"
		got := Build(in).Text()
		for _, want := range []string{"  Remote switchboard at http://tailnet-mac:8787 (disabled)\n", "  Start (disabled)\n", "  Stop (disabled)\n", "  Restart (disabled)\n"} {
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
	for _, want := range []string{"  landline not found. Run just install (disabled)\n", "  Start (disabled)\n", "  Stop (disabled)\n", "  Restart (disabled)\n", "  Set up Claude Code\n"} {
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
	for _, want := range []string{"  Failed: boom (disabled)\n", "  Stop (disabled)\n", "  Restart (disabled)\n", "  Set up Claude Code (disabled)\n", "  Remove from Codex (disabled)\n"} {
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
	in.Agents = []agentsview.Row{{Handle: "alpha", Label: long, Folder: "/" + long, Marker: agentsview.Active}}
	in.Notice = "Failed: a\nb\x1b[2J"
	got := Build(in).Text()
	for _, l := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		if len([]rune(l)) > maxTitleRunes+len(disabledText)+len(" [disconnected]")+len(indent)+len("Folder: ") {
			t.Fatalf("a line is %d runes", len([]rune(l)))
		}
	}
	if strings.ContainsAny(got, "\x1b\r\t") || !strings.Contains(got, "  Failed: a b [2J (disabled)\n") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestIconCountsConnectedAgentsOnly(t *testing.T) {
	in := local(true)
	in.Agents = []agentsview.Row{{Handle: "a", Marker: agentsview.Active}, {Handle: "b", Marker: agentsview.Disconnected}}
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

func TestLoopbackAcceptsLocalhostAndLoopbackIPsOnly(t *testing.T) {
	yes := []string{"http://localhost:8787", "http://LOCALHOST", "http://127.0.0.1:1", "http://127.9.9.9", "http://[::1]:8787"}
	no := []string{"", "http://tailnet-mac:8787", "http://10.0.0.5:8787", "http://localhost.evil.test:1", "http://0.0.0.0:1", "://bad"}
	for _, a := range yes {
		if !Loopback(a) {
			t.Errorf("%q should be loopback", a)
		}
	}
	for _, a := range no {
		if Loopback(a) {
			t.Errorf("%q should not be loopback", a)
		}
	}
}

func TestTheActionsAreTheFixedCommandsAndNothingFromTheSwitchboard(t *testing.T) {
	in := local(true)
	in.Agents = []agentsview.Row{{Handle: "evil; rm -rf /", Marker: agentsview.Active}}
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
		"server up", "server down", "server restart",
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

func TestCopyHandleCopiesTheHandleAndNothingElse(t *testing.T) {
	in := local(true)
	in.Agents = []agentsview.Row{{Handle: "alpha", Label: "amy", Folder: "/f", Marker: agentsview.Active}}
	row := Build(in).Items[0]
	copyItem := row.Children[len(row.Children)-1]
	if copyItem.Title != "Copy handle" || copyItem.Action.Kind != Copy || copyItem.Action.Text != "alpha" {
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
