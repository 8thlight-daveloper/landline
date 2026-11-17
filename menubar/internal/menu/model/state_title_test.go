package model

import (
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/menubar/internal/agentsview"
	"github.com/8thlight-daveloper/landline/menubar/internal/buildinfo"
)

const shapes = "●○"

func stateInput(reachable bool) Input {
	return Input{Address: "http://127.0.0.1:8787", Reachable: reachable, App: buildinfo.Build{Version: "0.2.0"}, LandlineFound: true}
}

func namedRow(name, state string) agentsview.Row { return agentsview.Row{ID: name, State: state} }

func stateRows() []agentsview.Row {
	return []agentsview.Row{
		namedRow("alpha", agentsview.Connected),
		namedRow("bravo", agentsview.Disconnected),
	}
}

func TestAnAgentRowItemCarriesTheStateAndATitleWithoutTheShape(t *testing.T) {
	in := stateInput(true)
	in.Agents = stateRows()
	menu := Build(in)
	want := []struct{ title, state string }{{"alpha", "connected"}, {"bravo", "disconnected"}}
	for i, w := range want {
		item := menu.Items[i]
		if item.State != w.state {
			t.Errorf("row %d has State %q, want %q", i, item.State, w.state)
		}
		if item.Title != w.title {
			t.Errorf("row %d has Title %q, want the name %q with no shape, because the screen draws the shape as the item's image", i, item.Title, w.title)
		}
		if strings.ContainsAny(item.Title, shapes) {
			t.Errorf("row %d Title %q holds a text shape", i, item.Title)
		}
	}
}

func TestTheTextRendererPrependsTheShapeAndASpaceToAnItemThatHasAState(t *testing.T) {
	menu := Menu{Items: []Item{
		{Title: "alpha", State: "connected"},
		{Title: "bravo", State: "disconnected"},
		{Title: "plain"},
	}}
	if got, want := menu.Text(), "icon: jack\n● alpha\n○ bravo\nplain\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTheTextOfABuiltMenuShowsOneShapePerAgentRowAndNoShapeAnywhereElse(t *testing.T) {
	in := stateInput(true)
	in.Agents = stateRows()
	text := Build(in).Text()
	lines := strings.Split(text, "\n")
	if lines[1] != "● alpha" || lines[len(lines)-1] != "" {
		t.Errorf("the first row is %q", lines[1])
	}
	shapeLines := 0
	for _, line := range lines {
		if strings.ContainsAny(line, shapes) {
			shapeLines++
			if !strings.HasPrefix(line, "● alpha") && !strings.HasPrefix(line, "○ bravo") {
				t.Errorf("a line that is not an agent row has a shape: %q", line)
			}
		}
	}
	if shapeLines != 2 {
		t.Errorf("%d lines have a shape, want one per agent row:\n%s", shapeLines, text)
	}
}

func TestItemsThatAreNotAgentRowsHaveNoStateAndGetNoShape(t *testing.T) {
	for name, in := range map[string]Input{"reachable": stateInput(true), "unreachable": stateInput(false)} {
		in.Agents = nil
		menu := Build(in)
		var walk func(items []Item)
		walk = func(items []Item) {
			for _, item := range items {
				if item.State != "" {
					t.Errorf("%s: %q has State %q", name, item.Title, item.State)
				}
				walk(item.Children)
			}
		}
		walk(menu.Items)
		if strings.ContainsAny(menu.Text(), shapes) {
			t.Errorf("%s: the text holds a shape:\n%s", name, menu.Text())
		}
	}
	in := stateInput(true)
	in.Agents = stateRows()
	for _, row := range Build(in).Items[:2] {
		for _, child := range row.Children {
			if child.State != "" {
				t.Errorf("the submenu item %q of an agent row has State %q", child.Title, child.State)
			}
		}
	}
}
