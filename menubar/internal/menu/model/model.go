package model

import (
	"strconv"

	"github.com/8thlight-daveloper/landline/menubar/internal/agentsview"
	"github.com/8thlight-daveloper/landline/menubar/internal/buildinfo"
	"github.com/8thlight-daveloper/landline/menubar/internal/clean"
)

const (
	maxTitleRunes   = 120
	ellipsis        = "…"
	notFoundTitle   = "landline not found. Run just install"
	unreachableLine = "Switchboard unreachable"
	unknownBuild    = "Switchboard (build unknown)"
	differsSuffix   = " (differs from this app)"
)

type ActionKind int

const (
	NoAction ActionKind = iota
	Copy
	Run
	About
	Refresh
	Quit
)

type Action struct {
	Kind ActionKind
	Text string
	Args []string
	Done string

	Confirm       string
	ConfirmAccept string
}

type Item struct {
	Title     string
	Disabled  bool
	Separator bool
	Action    Action
	Children  []Item
	State     string
}

type Icon struct {
	Dimmed bool
	Count  int
}

type Menu struct {
	Icon  Icon
	Items []Item
}

type Switchboard struct {
	Known bool
	Build buildinfo.Build
}

type Input struct {
	Address       string
	Local         bool
	Reachable     bool
	Agents        []agentsview.Row
	Switchboard   Switchboard
	App           buildinfo.Build
	LandlineFound bool
	Busy          bool
	Notice        string
}

func Build(in Input) Menu {
	menu := Menu{Icon: iconFor(in)}
	if in.Reachable {
		for _, row := range in.Agents {
			menu.Items = append(menu.Items, agentItem(row))
		}
	} else {
		menu.Items = append(menu.Items, disabled(unreachableLine))
	}
	if len(menu.Items) > 0 {
		menu.Items = append(menu.Items, separator())
	}
	menu.Items = append(menu.Items,
		parent("Switchboard", switchboardItems(in)),
		parent("Harnesses", harnessItems(in.Busy)),
		separator(),
		disabled(in.App.Label("Menubar")),
		action("About Landline", Action{Kind: About}),
		action("Refresh", Action{Kind: Refresh}),
		separator(),
		action("Quit", Action{Kind: Quit}),
	)
	return menu
}

func iconFor(in Input) Icon {
	if !in.Reachable {
		return Icon{Dimmed: true}
	}
	return Icon{Count: agentsview.Count(in.Agents)}
}

func agentItem(row agentsview.Row) Item {
	fields := []struct{ name, value string }{
		{"ID", row.ID}, {"Project", row.Project}, {"Task", row.Task}, {"Harness", row.Harness}, {"Host", row.Host}, {"Model", row.Model},
		{"Effort", row.Effort}, {"Folder", row.Folder}, {"Branch", row.Branch}, {"Landline version", row.LandlineVersion},
	}
	children := []Item{disabled("State: " + row.State)}
	for _, field := range fields {
		if field.value != "" {
			children = append(children, disabled(field.name+": "+field.value))
		}
	}
	children = append(children, separator(), action("Copy id", Action{Kind: Copy, Text: row.ID}))
	item := parent(row.Title(), children)
	item.State = row.State
	return item
}

func switchboardItems(in Input) []Item {
	local := in.Local
	start, stop, restart, rotate := serverActions()
	canRun := local && in.LandlineFound && !in.Busy
	return []Item{
		disabled(firstLine(in, local)),
		separator(),
		runItem("Start", start, canRun && !in.Reachable),
		runItem("Stop", stop, canRun && in.Reachable),
		runItem("Restart", restart, canRun && in.Reachable),
		runItem("Rotate secret…", rotate, canRun && in.Reachable),
		separator(),
		disabled(buildLine(in)),
	}
}

const rotateQuestion = "Rotate the switchboard secret? The switchboard restarts and every agent must register again."

func serverActions() (start, stop, restart, rotate Action) {
	return Action{Kind: Run, Args: []string{"server", "up"}},
		Action{Kind: Run, Args: []string{"server", "down"}},
		Action{Kind: Run, Args: []string{"server", "restart"}},
		Action{Kind: Run, Args: []string{"server", "rotate-secret"}, Confirm: rotateQuestion, ConfirmAccept: "Rotate"}
}

func firstLine(in Input, local bool) string {
	switch {
	case in.Notice != "":
		return in.Notice
	case !local:
		return "Remote switchboard at " + in.Address
	case !in.LandlineFound:
		return notFoundTitle
	case in.Reachable:
		return "Running at " + in.Address
	}
	return "Not running"
}

func buildLine(in Input) string {
	if !in.Reachable || !in.Switchboard.Known || in.Switchboard.Build.Version == "" {
		return unknownBuild
	}
	built := buildinfo.Clean(in.Switchboard.Build)
	line := built.Label("Switchboard")
	if built.Commit != "" && in.App.Commit != "" && built.Commit != in.App.Commit {
		line += differsSuffix
	}
	return line
}

func harnessItems(busy bool) []Item {
	var items []Item
	for _, h := range []struct{ name, id string }{{"Claude Code", "claude"}, {"Codex", "codex"}} {
		items = append(items,
			runItem("Set up "+h.name, Action{Kind: Run, Args: []string{"setup", h.id}, Done: h.name + " set up. Start a new session."}, !busy),
			runItem("Remove from "+h.name, Action{Kind: Run, Args: []string{"setup", h.id, "--remove"}, Done: h.name + " removed."}, !busy),
		)
	}
	return items
}

func line(text string) string {
	text = clean.Text(text)
	runes := []rune(text)
	if len(runes) <= maxTitleRunes {
		return text
	}
	return string(runes[:maxTitleRunes-1]) + ellipsis
}

func disabled(title string) Item { return Item{Title: line(title), Disabled: true} }

func separator() Item { return Item{Separator: true} }

func action(title string, a Action) Item { return Item{Title: line(title), Action: a} }

func runItem(title string, a Action, enabled bool) Item {
	return Item{Title: title, Disabled: !enabled, Action: a}
}

func parent(title string, children []Item) Item {
	return Item{Title: line(title), Children: children}
}

func (i Icon) CountText() string {
	if i.Count == 0 {
		return ""
	}
	return strconv.Itoa(i.Count)
}
