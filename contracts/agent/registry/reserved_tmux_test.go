package registry

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

func withTmux(t *testing.T, entry model.Agent, socket, pane string) model.Agent {
	t.Helper()
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if socket != "" {
		fields["tmux_socket"] = socket
	}
	if pane != "" {
		fields["tmux_pane"] = pane
	}
	raw, _ = json.Marshal(fields)
	var out model.Agent
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func tmuxOf(t *testing.T, entry model.Agent) (string, string) {
	t.Helper()
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{}
	json.Unmarshal(raw, &fields)
	socket, _ := fields["tmux_socket"].(string)
	pane, _ := fields["tmux_pane"].(string)
	return socket, pane
}

func TestTheNamesOperatorAndAllAreRefusedAsAName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"operator", "all"} {
		got, err := newImpl().Register(nil, agent(name), t0)
		if !errors.Is(err, model.ErrBadName) || got != nil {
			t.Errorf("name %q: got %+v, %v, want ErrBadName", name, got, err)
		}
	}
}

func TestNamesThatOnlyContainOrResembleOperatorAreAccepted(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"operator-1", "operator_2", "my-operator", "operators", "operato", "all-1"} {
		if _, err := newImpl().Register(nil, agent(name), t0); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	if _, err := newImpl().Register(nil, agent("Operator"), t0); !errors.Is(err, model.ErrBadName) {
		t.Errorf("Operator: %v", err)
	}
}

func TestTheNamesOperatorAndAllAreRefusedAsALabelAndTheListIsUnchanged(t *testing.T) {
	t.Parallel()
	for _, label := range []string{"operator", "all"} {
		existing, _ := newImpl().Register(nil, agent("bravo"), t0)
		before := append([]model.Agent(nil), existing...)
		entry := agent("alpha")
		entry.Label = label
		got, err := newImpl().Register(existing, entry, t0)
		if !errors.Is(err, model.ErrBadField) || got != nil {
			t.Errorf("label %q: got %+v, %v, want ErrBadField", label, got, err)
		}
		if !reflect.DeepEqual(existing, before) {
			t.Errorf("label %q: the caller's list changed", label)
		}
	}
}

func TestReRegisteringAnAttachedAgentWithTheLabelOperatorIsRefusedAndKeepsItsOldLabel(t *testing.T) {
	t.Parallel()
	first := agent("alpha")
	first.Label = "tutor"
	agents, err := newImpl().Register(nil, first, t0)
	if err != nil {
		t.Fatal(err)
	}
	updated := agents[0]
	updated.Label = "operator"
	got, err := newImpl().Register(agents, updated, t0.Add(time.Second))
	if !errors.Is(err, model.ErrBadField) || got != nil {
		t.Fatalf("got %+v, %v", got, err)
	}
	if found, _ := newImpl().Find(agents, "alpha"); found.Label != "tutor" {
		t.Fatalf("the label became %q", found.Label)
	}
}

func TestLabelsThatOnlyContainOrDifferInCaseFromTheReservedNamesAreAccepted(t *testing.T) {
	t.Parallel()
	for _, label := range []string{"operators", "my operator", "operator-2", "Operator", "ALL", "all-in", "alle"} {
		entry := agent("alpha")
		entry.Label = label
		got, err := newImpl().Register(nil, entry, t0)
		if err != nil || got[0].Label != label {
			t.Errorf("%q: %+v, %v", label, got, err)
		}
	}
}

func TestAReservedLabelDoesNotHideABadNameOrAMissingHarness(t *testing.T) {
	t.Parallel()
	badName := agent("operator")
	badName.Label = "operator"
	if _, err := newImpl().Register(nil, badName, t0); !errors.Is(err, model.ErrBadName) {
		t.Errorf("got %v, want ErrBadName", err)
	}
	noHarness := agent("alpha")
	noHarness.Harness, noHarness.Label = "", "all"
	if _, err := newImpl().Register(nil, noHarness, t0); !errors.Is(err, model.ErrNoHarness) {
		t.Errorf("got %v, want ErrNoHarness", err)
	}
}

func TestAValidTmuxSocketAndPaneAreStoredAndFoundUnchanged(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ socket, pane string }{
		{"default", "%3"},
		{"a", "%0"},
		{"work-2.sock_1", "%123456789"},
		{".hidden", "%7"},
		{"_x", "%7"},
		{"9lives", "%10"},
		{strings.Repeat("s", 64), "%1"},
	} {
		entry := withTmux(t, agent("alpha"), c.socket, c.pane)
		got, err := newImpl().Register(nil, entry, t0)
		if err != nil {
			t.Errorf("%q %q: %v", c.socket, c.pane, err)
			continue
		}
		if socket, pane := tmuxOf(t, got[0]); socket != c.socket || pane != c.pane {
			t.Errorf("stored %q %q, want %q %q", socket, pane, c.socket, c.pane)
		}
		found, ok := newImpl().Find(got, "alpha")
		if socket, pane := tmuxOf(t, found); !ok || socket != c.socket || pane != c.pane {
			t.Errorf("found %q %q, want %q %q", socket, pane, c.socket, c.pane)
		}
	}
}

func TestAnAgentWithNoTmuxAttributesHasNoneAfterRegistering(t *testing.T) {
	t.Parallel()
	got, err := newImpl().Register(nil, agent("alpha"), t0)
	if err != nil {
		t.Fatal(err)
	}
	if socket, pane := tmuxOf(t, got[0]); socket != "" || pane != "" {
		t.Fatalf("got %q %q", socket, pane)
	}
}

func TestAHostileTmuxSocketRefusesTheWholeRegistrationAndLeavesTheListUnchanged(t *testing.T) {
	t.Parallel()
	for _, socket := range []string{
		"has space", "a b", " default", "default ", "a;b", "a;rm -rf /", "$(id)", "a$(id)", "`id`", "a`id`", "${HOME}",
		"-x", "-L", "--help", "a\nb", "default\n", "\ndefault", "a\rb", "a\tb", "a\x00b",
		strings.Repeat("s", 65), strings.Repeat("s", 200),
		"a/b", "../x", "a:b", "a|b", "a&b", "a'b", "a\"b", "a\\b", "a*b", "é", "a‮b", "a b", "a​b",
	} {
		existing, _ := newImpl().Register(nil, agent("bravo"), t0)
		before := append([]model.Agent(nil), existing...)
		entry := withTmux(t, agent("alpha"), socket, "%3")
		got, err := newImpl().Register(existing, entry, t0)
		if !errors.Is(err, model.ErrBadField) || got != nil {
			t.Errorf("socket %q: got %+v, %v, want ErrBadField", socket, got, err)
		}
		if !reflect.DeepEqual(existing, before) {
			t.Errorf("socket %q: the caller's list changed", socket)
		}
	}
}

func TestAHostileTmuxPaneRefusesTheWholeRegistrationAndLeavesTheListUnchanged(t *testing.T) {
	t.Parallel()
	for _, pane := range []string{
		"%", "%abc", "%a1", "%1a", "3", "%%3", "% 3", "%3 ", " %3", "%3;x", "%3;id", "$(id)", "%$(id)", "`id`", "%`id`",
		"-%3", "-3", "%-1", "%+1", "%1\n", "\n%1", "%1\r", "%1\t2", "%1\x00",
		"%1234567890", "%0000000000", "%１", "%١", "%1​", "%1‮", "%1.5", "%0x1",
	} {
		existing, _ := newImpl().Register(nil, agent("bravo"), t0)
		before := append([]model.Agent(nil), existing...)
		entry := withTmux(t, agent("alpha"), "default", pane)
		got, err := newImpl().Register(existing, entry, t0)
		if !errors.Is(err, model.ErrBadField) || got != nil {
			t.Errorf("pane %q: got %+v, %v, want ErrBadField", pane, got, err)
		}
		if !reflect.DeepEqual(existing, before) {
			t.Errorf("pane %q: the caller's list changed", pane)
		}
	}
}

func TestATmuxSocketOfExactlySixtyFourBytesIsAcceptedAndSixtyFiveIsNot(t *testing.T) {
	t.Parallel()
	if _, err := newImpl().Register(nil, withTmux(t, agent("alpha"), strings.Repeat("s", 64), "%1"), t0); err != nil {
		t.Errorf("64 bytes: %v", err)
	}
	if _, err := newImpl().Register(nil, withTmux(t, agent("alpha"), strings.Repeat("s", 65), "%1"), t0); !errors.Is(err, model.ErrBadField) {
		t.Errorf("65 bytes: %v", err)
	}
}

func TestATmuxPaneOfNineDigitsIsAcceptedAndTenIsNot(t *testing.T) {
	t.Parallel()
	if _, err := newImpl().Register(nil, withTmux(t, agent("alpha"), "default", "%999999999"), t0); err != nil {
		t.Errorf("9 digits: %v", err)
	}
	if _, err := newImpl().Register(nil, withTmux(t, agent("alpha"), "default", "%1000000000"), t0); !errors.Is(err, model.ErrBadField) {
		t.Errorf("10 digits: %v", err)
	}
}

func TestARegistrationWithOnlyOneTmuxValueKeepsItAndRefusesItWhenItIsBad(t *testing.T) {
	t.Parallel()
	onlySocket, err := newImpl().Register(nil, withTmux(t, agent("alpha"), "default", ""), t0)
	if err != nil {
		t.Fatalf("a socket alone: %v", err)
	}
	if socket, pane := tmuxOf(t, onlySocket[0]); socket != "default" || pane != "" {
		t.Errorf("got %q %q", socket, pane)
	}
	onlyPane, err := newImpl().Register(nil, withTmux(t, agent("alpha"), "", "%3"), t0)
	if err != nil {
		t.Fatalf("a pane alone: %v", err)
	}
	if socket, pane := tmuxOf(t, onlyPane[0]); socket != "" || pane != "%3" {
		t.Errorf("got %q %q", socket, pane)
	}
	if _, err := newImpl().Register(nil, withTmux(t, agent("alpha"), "", "%abc"), t0); !errors.Is(err, model.ErrBadField) {
		t.Errorf("a bad pane alone: %v", err)
	}
	if _, err := newImpl().Register(nil, withTmux(t, agent("alpha"), "a b", ""), t0); !errors.Is(err, model.ErrBadField) {
		t.Errorf("a bad socket alone: %v", err)
	}
}

func TestRegisteringANameAgainReplacesTheTmuxValuesAndNoValuesClearThem(t *testing.T) {
	t.Parallel()
	r := newImpl()
	first, err := r.Register(nil, withTmux(t, agent("alpha"), "default", "%3"), t0)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := r.Register(first, withTmux(t, agent("alpha"), "work", "%9"), t0.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if socket, pane := tmuxOf(t, moved[0]); len(moved) != 1 || socket != "work" || pane != "%9" {
		t.Fatalf("replaced: %+v", moved)
	}
	cleared, err := r.Register(moved, agent("alpha"), t0.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if socket, pane := tmuxOf(t, cleared[0]); len(cleared) != 1 || socket != "" || pane != "" {
		t.Fatalf("cleared: %+v", cleared)
	}
}

func TestARefusedTmuxReRegistrationKeepsTheOldValuesInTheCallersList(t *testing.T) {
	t.Parallel()
	r := newImpl()
	first, _ := r.Register(nil, withTmux(t, agent("alpha"), "default", "%3"), t0)
	before := append([]model.Agent(nil), first...)
	if _, err := r.Register(first, withTmux(t, agent("alpha"), "default", "%x"), t0.Add(time.Second)); !errors.Is(err, model.ErrBadField) {
		t.Fatalf("got %v", err)
	}
	if !reflect.DeepEqual(first, before) {
		t.Fatal("the caller's list changed")
	}
	if socket, pane := tmuxOf(t, first[0]); socket != "default" || pane != "%3" {
		t.Fatalf("got %q %q", socket, pane)
	}
}

func TestABadTmuxValueDoesNotHideABadNameOrAMissingHarness(t *testing.T) {
	t.Parallel()
	badName := withTmux(t, agent("Alpha"), "a b", "%x")
	if _, err := newImpl().Register(nil, badName, t0); !errors.Is(err, model.ErrBadName) {
		t.Errorf("got %v, want ErrBadName", err)
	}
	noHarness := withTmux(t, agent("alpha"), "a b", "%x")
	noHarness.Harness = ""
	if _, err := newImpl().Register(nil, noHarness, t0); !errors.Is(err, model.ErrNoHarness) {
		t.Errorf("got %v, want ErrNoHarness", err)
	}
}

func TestResolveAndFindIgnoreTheTmuxValues(t *testing.T) {
	t.Parallel()
	agents, err := newImpl().Register(nil, withTmux(t, agent("alpha"), "default", "%3"), t0)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"default", "%3", "def", "%"} {
		if got, rest := newImpl().Resolve(agents, agents, query); got.Name != "" || rest != nil {
			t.Errorf("query %q matched: %+v %+v", query, got, rest)
		}
		if got, ok := newImpl().Find(agents, query); ok {
			t.Errorf("name %q matched: %+v", query, got)
		}
	}
}

func TestTmuxValuesDoNotLeakIntoOtherFields(t *testing.T) {
	t.Parallel()
	entry := withTmux(t, agent("alpha"), "default", "%3")
	got, _ := newImpl().Register(nil, entry, t0)
	want := agent("alpha")
	want.LastSeen = t0
	stripped := got[0]
	raw, _ := json.Marshal(stripped)
	fields := map[string]any{}
	json.Unmarshal(raw, &fields)
	delete(fields, "tmux_socket")
	delete(fields, "tmux_pane")
	raw, _ = json.Marshal(fields)
	var plain model.Agent
	json.Unmarshal(raw, &plain)
	plain.LastSeen = want.LastSeen
	if plain != want {
		t.Fatalf("got %+v, want %+v", plain, want)
	}
}
