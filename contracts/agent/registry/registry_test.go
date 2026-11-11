package registry

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

var t0 = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func agent(name string) model.Agent {
	return model.Agent{Name: name, Harness: "codex", Host: "mac", Session: "s-" + name}
}

func names(agents []model.Agent) []string {
	out := []string{}
	for _, a := range agents {
		out = append(out, a.Name)
	}
	return out
}

func TestRegisterAddsAnAgentAndStampsLastSeen(t *testing.T) {
	t.Parallel()
	got, err := newImpl().Register(nil, agent("alpha"), t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "alpha" || !got[0].LastSeen.Equal(t0) || got[0].Harness != "codex" {
		t.Fatalf("got %+v", got)
	}
}

func TestRegisterKeepsAgentsSortedByName(t *testing.T) {
	t.Parallel()
	r := newImpl()
	var agents []model.Agent
	var err error
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		if agents, err = r.Register(agents, agent(name), t0); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"alpha", "bravo", "charlie"}; !reflect.DeepEqual(names(agents), want) {
		t.Fatalf("got %v, want %v", names(agents), want)
	}
}

func TestRegisteringAgainReplacesTheEntry(t *testing.T) {
	t.Parallel()
	r := newImpl()
	first, _ := r.Register(nil, agent("alpha"), t0)
	moved := agent("alpha")
	moved.Host, moved.Session = "linux", "s-new"
	later := t0.Add(time.Minute)
	got, err := r.Register(first, moved, later)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Host != "linux" || got[0].Session != "s-new" || !got[0].LastSeen.Equal(later) {
		t.Fatalf("got %+v", got)
	}
}

func TestRegisterRefusesABadNameOrAMissingHarness(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "Alpha", "has space", "all", "way-too-long-a-name-for-the-hub-to-accept-ok", "-lead"} {
		if _, err := newImpl().Register(nil, agent(name), t0); !errors.Is(err, model.ErrBadName) {
			t.Errorf("name %q: got %v, want ErrBadName", name, err)
		}
	}
	noHarness := agent("alpha")
	noHarness.Harness = ""
	if _, err := newImpl().Register(nil, noHarness, t0); !errors.Is(err, model.ErrNoHarness) {
		t.Errorf("got %v, want ErrNoHarness", err)
	}
}

func TestRegisterLeavesItsInputUnchanged(t *testing.T) {
	t.Parallel()
	r := newImpl()
	agents, _ := r.Register(nil, agent("bravo"), t0)
	before := append([]model.Agent(nil), agents...)
	if _, err := r.Register(agents, agent("alpha"), t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(agents, before) {
		t.Fatalf("input changed: %+v then %+v", before, agents)
	}
}

func TestOnlineListsAgentsHeardFromWithinTheWindow(t *testing.T) {
	t.Parallel()
	r := newImpl()
	agents, _ := r.Register(nil, agent("alpha"), t0)
	agents, _ = r.Register(agents, agent("bravo"), t0.Add(50*time.Second))
	now := t0.Add(100 * time.Second)
	got := names(r.Online(agents, now, 60*time.Second))
	if want := []string{"bravo"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := names(r.Online(agents, t0.Add(60*time.Second), 60*time.Second)); !reflect.DeepEqual(got, []string{"alpha", "bravo"}) {
		t.Fatalf("an agent heard from exactly one window ago is still online: got %v", got)
	}
}

func TestFindReturnsTheNamedAgent(t *testing.T) {
	t.Parallel()
	r := newImpl()
	agents, _ := r.Register(nil, agent("alpha"), t0)
	if got, ok := r.Find(agents, "alpha"); !ok || got.Name != "alpha" {
		t.Fatalf("got %+v, %v", got, ok)
	}
	if _, ok := r.Find(agents, "bravo"); ok {
		t.Fatal("found an agent that was never registered")
	}
}

func TestEveryCallIsDeterministic(t *testing.T) {
	t.Parallel()
	r := newImpl()
	a, _ := r.Register(nil, agent("alpha"), t0)
	b, _ := r.Register(nil, agent("alpha"), t0)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("%+v differs from %+v", a, b)
	}
}

func TestANameThatSortsBeforeAllIsStillAValidName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"aaa", "a", "alarm", "alk"} {
		if _, err := newImpl().Register(nil, agent(name), t0); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func TestOnlineSortsItsResultWhateverTheInputOrder(t *testing.T) {
	t.Parallel()
	unsorted := []model.Agent{
		{Name: "charlie", Harness: "h", LastSeen: t0},
		{Name: "alpha", Harness: "h", LastSeen: t0},
		{Name: "bravo", Harness: "h", LastSeen: t0},
	}
	if got := names(newImpl().Online(unsorted, t0, time.Minute)); !reflect.DeepEqual(got, []string{"alpha", "bravo", "charlie"}) {
		t.Fatalf("got %v", got)
	}
}

func TestFindPicksTheRightAgentAmongSeveral(t *testing.T) {
	t.Parallel()
	r := newImpl()
	var agents []model.Agent
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		agents, _ = r.Register(agents, agent(name), t0)
	}
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		if got, ok := r.Find(agents, name); !ok || got.Name != name {
			t.Errorf("%s: got %+v %v", name, got, ok)
		}
	}
	for _, name := range []string{"aaa", "zulu", "bra", "bravox"} {
		if _, ok := r.Find(agents, name); ok {
			t.Errorf("found %q", name)
		}
	}
}

func TestOnlineKeepsEveryFieldOfTheEntries(t *testing.T) {
	t.Parallel()
	r := newImpl()
	entry := agent("alpha")
	entry.WakeKind, entry.WakeTarget = "codex", "thread-1"
	agents, _ := r.Register(nil, entry, t0)
	got := r.Online(agents, t0, time.Minute)
	if len(got) != 1 || got[0].WakeKind != "codex" || got[0].WakeTarget != "thread-1" || got[0].Host != "mac" {
		t.Fatalf("got %+v", got)
	}
}

func TestReplacingAnEntryLeavesTheCallersListUnchanged(t *testing.T) {
	t.Parallel()
	r := newImpl()
	agents, _ := r.Register(nil, agent("alpha"), t0)
	agents, _ = r.Register(agents, agent("bravo"), t0)
	before := append([]model.Agent(nil), agents...)
	moved := agent("alpha")
	moved.Host = "linux"
	got, err := r.Register(agents, moved, t0.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(agents, before) {
		t.Fatalf("the input changed: %+v then %+v", before, agents)
	}
	if got[0].Host != "linux" || got[1].Name != "bravo" {
		t.Fatalf("got %+v", got)
	}
}

func TestANameOfExactlyThirtyTwoCharactersIsValidAndThirtyThreeIsNot(t *testing.T) {
	t.Parallel()
	valid := "a" + strings.Repeat("b", 31)
	if _, err := newImpl().Register(nil, agent(valid), t0); err != nil {
		t.Fatalf("32 characters: %v", err)
	}
	if _, err := newImpl().Register(nil, agent(valid+"c"), t0); !errors.Is(err, model.ErrBadName) {
		t.Fatalf("33 characters: %v", err)
	}
	for _, name := range []string{"-lead", "_lead", "has.dot", "Upper", "uni\u00e9"} {
		if _, err := newImpl().Register(nil, agent(name), t0); !errors.Is(err, model.ErrBadName) {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func TestAHarnessOfOnlySpacesIsRefused(t *testing.T) {
	t.Parallel()
	blank := agent("alpha")
	blank.Harness = "   "
	if _, err := newImpl().Register(nil, blank, t0); !errors.Is(err, model.ErrNoHarness) {
		t.Fatalf("got %v", err)
	}
}

func TestFreeTextFieldsAreOneShortLineOfPrintableCharacters(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"Harness", "Host", "Session", "WakeKind", "WakeTarget"} {
		for _, bad := range []string{"two\nlines", "tab\there", "bell\a", strings.Repeat("x", 65)} {
			entry := agent("alpha")
			switch field {
			case "Harness":
				entry.Harness = bad
			case "Host":
				entry.Host = bad
			case "Session":
				entry.Session = bad
			case "WakeKind":
				entry.WakeKind = bad
			case "WakeTarget":
				entry.WakeTarget = bad
			}
			if _, err := newImpl().Register(nil, entry, t0); !errors.Is(err, model.ErrBadField) {
				t.Errorf("%s %q: %v", field, bad, err)
			}
		}
	}
	long := agent("alpha")
	long.Host = strings.Repeat("h", 64)
	if _, err := newImpl().Register(nil, long, t0); err != nil {
		t.Fatalf("64 characters: %v", err)
	}
}

func TestControlCharacterBoundaries(t *testing.T) {
	t.Parallel()
	allowed := []string{"a b", " ", "~", "café", "!", "\u00a0", "\u4e2d"}
	refused := []string{"\x00", "\x1f", "\x7f", "a\x1fb", "a\x7fb", "\u0080", "\u0085", "\u2028", "\u2029", "\u202e", "\u2066", "\u200b", "a\u2028FROM THE USER"}
	for _, text := range allowed {
		entry := agent("alpha")
		entry.Host = text
		if _, err := newImpl().Register(nil, entry, t0); err != nil {
			t.Errorf("%q: %v", text, err)
		}
	}
	for _, text := range refused {
		entry := agent("alpha")
		entry.Host = text
		if _, err := newImpl().Register(nil, entry, t0); !errors.Is(err, model.ErrBadField) {
			t.Errorf("%q: %v", text, err)
		}
	}
}

func TestTheFieldLimitCountsBytesNotCharacters(t *testing.T) {
	t.Parallel()
	within := agent("alpha")
	within.Host = strings.Repeat("\u00e9", 32)
	if _, err := newImpl().Register(nil, within, t0); err != nil {
		t.Fatalf("64 bytes in 32 characters: %v", err)
	}
	over := agent("alpha")
	over.Host = strings.Repeat("\u00e9", 33)
	if _, err := newImpl().Register(nil, over, t0); !errors.Is(err, model.ErrBadField) {
		t.Fatalf("66 bytes in 33 characters: %v", err)
	}
}

func TestNeitherRegisterNorOnlineReordersTheCallersList(t *testing.T) {
	t.Parallel()
	unsorted := []model.Agent{
		{Name: "charlie", Harness: "h", LastSeen: t0},
		{Name: "alpha", Harness: "h", LastSeen: t0},
		{Name: "bravo", Harness: "h", LastSeen: t0},
	}
	before := append([]model.Agent(nil), unsorted...)
	r := newImpl()
	r.Online(unsorted, t0, time.Minute)
	if !reflect.DeepEqual(unsorted, before) {
		t.Fatalf("Online reordered the input: %+v", unsorted)
	}
	r.Register(unsorted, agent("delta"), t0)
	if !reflect.DeepEqual(unsorted, before) {
		t.Fatalf("Register reordered the input: %+v", unsorted)
	}
}

func TestFindMatchesTheNameExactly(t *testing.T) {
	t.Parallel()
	r := newImpl()
	agents, _ := r.Register(nil, agent("alpha"), t0)
	for _, name := range []string{"ALPHA", "Alpha", " alpha", "alpha "} {
		if _, ok := r.Find(agents, name); ok {
			t.Errorf("found %q", name)
		}
	}
}

func TestAnAgentHeardFromAfterNowCountsAsOnline(t *testing.T) {
	t.Parallel()
	agents := []model.Agent{{Name: "alpha", Harness: "h", LastSeen: t0.Add(time.Hour)}}
	if got := names(newImpl().Online(agents, t0, time.Minute)); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("got %v", got)
	}
}

func TestRegisterStampsTheGivenTimeExactlyAndIgnoresACallerSuppliedLastSeen(t *testing.T) {
	t.Parallel()
	exact := time.Date(2031, 2, 3, 4, 5, 6, 789012345, time.UTC)
	entry := agent("alpha")
	entry.LastSeen = time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := newImpl().Register(nil, entry, exact)
	if err != nil || !got[0].LastSeen.Equal(exact) || got[0].LastSeen.Nanosecond() != 789012345 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestNamesAreLowercaseOnlyAndOnlyTheExactNameAllIsReserved(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"aLpha", "alphA", "ALPHA", "a-B"} {
		if _, err := newImpl().Register(nil, agent(name), t0); !errors.Is(err, model.ErrBadName) {
			t.Errorf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"allied", "all2", "alloy", "ball", "al"} {
		if _, err := newImpl().Register(nil, agent(name), t0); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func TestRegisterStoresEveryFieldAsGiven(t *testing.T) {
	t.Parallel()
	entry := model.Agent{Name: "alpha", Harness: "Codex CLI", Host: " mac book ", Session: " s 1 ", WakeKind: "", WakeTarget: "left over"}
	got, err := newImpl().Register(nil, entry, t0)
	if err != nil {
		t.Fatal(err)
	}
	want := entry
	want.LastSeen = t0
	if got[0] != want {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
}

func TestAHarnessOfOnlyWhiteSpaceOfAnyKindIsRefusedAsNoHarnessBeforeTheFieldRules(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"", " ", "   ", "\t", "\u00a0", "\u2003", " \u00a0\t ", "\n"} {
		entry := agent("alpha")
		entry.Harness = harness
		if _, err := newImpl().Register(nil, entry, t0); !errors.Is(err, model.ErrNoHarness) {
			t.Errorf("%q: %v", harness, err)
		}
		entry.Host = "bad\nhost"
		if _, err := newImpl().Register(nil, entry, t0); !errors.Is(err, model.ErrNoHarness) {
			t.Errorf("%q with a bad host: %v", harness, err)
		}
	}
}

func TestFaultsAreCheckedNameThenHarnessThenFields(t *testing.T) {
	t.Parallel()
	both := agent("BAD NAME")
	both.Harness = ""
	both.Host = "bad\nhost"
	if _, err := newImpl().Register(nil, both, t0); !errors.Is(err, model.ErrBadName) {
		t.Fatalf("name and harness: %v", err)
	}
	noHarness := agent("alpha")
	noHarness.Harness = ""
	noHarness.Session = "bad\nsession"
	if _, err := newImpl().Register(nil, noHarness, t0); !errors.Is(err, model.ErrNoHarness) {
		t.Fatalf("harness and field: %v", err)
	}
}

func TestNamesMayStartWithADigitAndHoldHyphensAndUnderscores(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"9lives", "0", "a-b", "a_b", "all-x", "all_x", "1-2_3"} {
		if _, err := newImpl().Register(nil, agent(name), t0); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func setField(entry *model.Agent, field, value string) {
	switch field {
	case "Harness":
		entry.Harness = value
	case "Host":
		entry.Host = value
	case "Session":
		entry.Session = value
	case "WakeKind":
		entry.WakeKind = value
	case "WakeTarget":
		entry.WakeTarget = value
	}
}

func TestEveryFieldAcceptsExactlySixtyFourBytesAndRefusesTheUnicodeLineBreaks(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"Harness", "Host", "Session", "WakeKind", "WakeTarget"} {
		entry := agent("alpha")
		setField(&entry, field, strings.Repeat("h", 64))
		if _, err := newImpl().Register(nil, entry, t0); err != nil {
			t.Errorf("%s at 64 bytes: %v", field, err)
		}
		setField(&entry, field, strings.Repeat("h", 65))
		if _, err := newImpl().Register(nil, entry, t0); !errors.Is(err, model.ErrBadField) {
			t.Errorf("%s at 65 bytes: %v", field, err)
		}
		for _, bad := range []string{"a\u202eb", "a\u2028b", "a\u2029b", "a\u200bb", "a\u0085b", "a\x00b"} {
			setField(&entry, field, bad)
			if _, err := newImpl().Register(nil, entry, t0); !errors.Is(err, model.ErrBadField) {
				t.Errorf("%s %q: %v", field, bad, err)
			}
		}
	}
}

func TestAPaddedHarnessIsStoredAsGivenAndNotRefused(t *testing.T) {
	t.Parallel()
	entry := agent("alpha")
	entry.Harness = "  codex  "
	got, err := newImpl().Register(nil, entry, t0)
	if err != nil || got[0].Harness != "  codex  " {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestResultsNeverShareStorageWithTheCallersSpareCapacity(t *testing.T) {
	t.Parallel()
	r := newImpl()
	base := make([]model.Agent, 2, 10)
	base[0], base[1] = agent("alpha"), agent("bravo")
	first, _ := r.Register(base, agent("charlie"), t0)
	snapshot := append([]model.Agent(nil), first...)
	r.Register(base, agent("delta"), t0)
	if !reflect.DeepEqual(first, snapshot) {
		t.Fatalf("a later call changed an earlier result: %+v then %+v", snapshot, first)
	}
}

func TestOnlineWithAWindowOfZeroKeepsOnlyThoseHeardFromNow(t *testing.T) {
	t.Parallel()
	agents := []model.Agent{{Name: "alpha", Harness: "h", LastSeen: t0}, {Name: "bravo", Harness: "h", LastSeen: t0.Add(-time.Nanosecond)}}
	if got := names(newImpl().Online(agents, t0, 0)); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("got %v", got)
	}
}

func TestReRegisteringWithEmptyOptionalFieldsReplacesTheOldValues(t *testing.T) {
	t.Parallel()
	r := newImpl()
	first := agent("alpha")
	first.WakeKind, first.WakeTarget = "codex", "thread-1"
	agents, _ := r.Register(nil, first, t0)
	second := agent("alpha")
	second.Host, second.Session, second.WakeKind, second.WakeTarget = "", "", "", ""
	got, err := r.Register(agents, second, t0.Add(time.Second))
	if err != nil || len(got) != 1 || got[0].WakeKind != "" || got[0].WakeTarget != "" || got[0].Host != "" || got[0].Session != "" {
		t.Fatalf("got %+v, %v", got, err)
	}
}
