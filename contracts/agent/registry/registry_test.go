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

func TestReplacingAnEntryLeavesTheCallersListUnchanged(t *testing.T) {
	t.Parallel()
	r := newImpl()
	agents, _ := r.Register(nil, agent("alpha"), t0)
	agents, _ = r.Register(agents, agent("bravo"), t0)
	before := append([]model.Agent(nil), agents...)
	moved := agent("alpha")
	moved.Session = "s-new"
	got, err := r.Register(agents, moved, t0.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(agents, before) {
		t.Fatalf("the input changed: %+v then %+v", before, agents)
	}
	if got[0].Session != "s-new" || got[1].Name != "bravo" {
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
	for _, field := range []string{"Harness", "Host", "Session"} {
		for _, bad := range []string{"two\nlines", "tab\there", "bell\a", strings.Repeat("x", 65)} {
			entry := agent("alpha")
			switch field {
			case "Harness":
				entry.Harness = bad
			case "Host":
				entry.Host = bad
			case "Session":
				entry.Session = bad
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

func TestRegisterDoesNotReorderTheCallersList(t *testing.T) {
	t.Parallel()
	unsorted := []model.Agent{
		{Name: "charlie", Harness: "h", LastSeen: t0},
		{Name: "alpha", Harness: "h", LastSeen: t0},
		{Name: "bravo", Harness: "h", LastSeen: t0},
	}
	before := append([]model.Agent(nil), unsorted...)
	newImpl().Register(unsorted, agent("delta"), t0)
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
	entry := model.Agent{Name: "alpha", Harness: "Codex CLI", Host: " mac book ", Session: " s 1 "}
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
	case "Folder":
		entry.Folder = value
	case "Branch":
		entry.Branch = value
	case "Worktree":
		entry.Worktree = value
	case "HarnessVersion":
		entry.HarnessVersion = value
	case "Model":
		entry.Model = value
	case "Effort":
		entry.Effort = value
	case "Label":
		entry.Label = value
	}
}

func TestEveryFieldAcceptsExactlySixtyFourBytesAndRefusesTheUnicodeLineBreaks(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"Harness", "Host", "Session", "Folder", "Branch", "Worktree", "HarnessVersion", "Model", "Effort", "Label"} {
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

func TestReRegisteringWithEmptyOptionalFieldsReplacesTheOldValues(t *testing.T) {
	t.Parallel()
	r := newImpl()
	first := agent("alpha")
	agents, _ := r.Register(nil, first, t0)
	second := agent("alpha")
	second.Host, second.Session = "", ""
	got, err := r.Register(agents, second, t0.Add(time.Second))
	if err != nil || len(got) != 1 || got[0].Host != "" || got[0].Session != "" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestEveryAttributeIsStoredAsGivenAndMayBeEmpty(t *testing.T) {
	t.Parallel()
	entry := agent("alpha")
	entry.Folder, entry.Branch, entry.Worktree, entry.HarnessVersion = "threnos", "fix/login", "wt-2", "2.1.296"
	entry.Model, entry.Effort, entry.Label = "claude-opus-5-5", "medium", "  main reviewer  "
	got, err := newImpl().Register(nil, entry, t0)
	if err != nil {
		t.Fatal(err)
	}
	want := entry
	want.LastSeen = t0
	if got[0] != want {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
	bare, err := newImpl().Register(nil, agent("bravo"), t0)
	if err != nil || bare[0].Label != "" || bare[0].Model != "" {
		t.Fatalf("empty attributes: %+v, %v", bare, err)
	}
}

func resolveList() []model.Agent {
	mk := func(name, label string) model.Agent { return model.Agent{Name: name, Harness: "h", Label: label} }
	return []model.Agent{mk("alpha-1k2", "tutor"), mk("alpha-9z9", "reviewer"), mk("bravo-3x3", "tutor"), mk("charlie-7q7", "")}
}

func TestResolveFindsAnExactHandleWhetherOrNotTheAgentIsLive(t *testing.T) {
	t.Parallel()
	all := resolveList()
	got, candidates := newImpl().Resolve(all, nil, "charlie-7q7")
	if got.Name != "charlie-7q7" || candidates != nil {
		t.Fatalf("got %+v %v", got, candidates)
	}
}

func TestResolveByLabelNeedsExactlyOneLiveMatchAndOtherwiseReturnsTheCandidatesSorted(t *testing.T) {
	t.Parallel()
	all := resolveList()
	got, candidates := newImpl().Resolve(all, all, "reviewer")
	if got.Name != "alpha-9z9" || candidates != nil {
		t.Fatalf("one match: %+v %v", got, candidates)
	}
	got, candidates = newImpl().Resolve(all, []model.Agent{all[2], all[0]}, "tutor")
	if got.Name != "" || !reflect.DeepEqual(names(candidates), []string{"alpha-1k2", "bravo-3x3"}) {
		t.Fatalf("two matches: %+v %v", got, names(candidates))
	}
	got, candidates = newImpl().Resolve(all, []model.Agent{all[0]}, "tutor")
	if got.Name != "alpha-1k2" || candidates != nil {
		t.Fatalf("one live match among two: %+v %v", got, names(candidates))
	}
}

func TestResolveByPrefixOfAHandleOrALabelNeedsExactlyOneLiveMatch(t *testing.T) {
	t.Parallel()
	all := resolveList()
	if got, _ := newImpl().Resolve(all, all, "bra"); got.Name != "bravo-3x3" {
		t.Fatalf("handle prefix: %+v", got)
	}
	if got, _ := newImpl().Resolve(all, all, "rev"); got.Name != "alpha-9z9" {
		t.Fatalf("label prefix: %+v", got)
	}
	got, candidates := newImpl().Resolve(all, all, "alpha")
	if got.Name != "" || !reflect.DeepEqual(names(candidates), []string{"alpha-1k2", "alpha-9z9"}) {
		t.Fatalf("ambiguous prefix: %+v %v", got, names(candidates))
	}
}

func TestResolveFindsNothingForAnEmptyUnknownOrCaseDifferentQuery(t *testing.T) {
	t.Parallel()
	all := resolveList()
	for _, query := range []string{"", "zulu", "TUTOR", "Bravo", "tutors", "alpha-1k2x"} {
		if got, candidates := newImpl().Resolve(all, all, query); got.Name != "" || len(candidates) != 0 {
			t.Errorf("%q: %+v %v", query, got, names(candidates))
		}
	}
}

func TestResolvePrefersAnExactHandleOverALabelAndALabelOverAPrefix(t *testing.T) {
	t.Parallel()
	all := []model.Agent{
		{Name: "tutor", Harness: "h"},
		{Name: "bravo-1", Harness: "h", Label: "tutor"},
		{Name: "tutorial-2", Harness: "h"},
	}
	if got, _ := newImpl().Resolve(all, all, "tutor"); got.Name != "tutor" {
		t.Fatalf("handle first: %+v", got)
	}
	all = all[1:]
	if got, _ := newImpl().Resolve(all, all, "tutor"); got.Name != "bravo-1" {
		t.Fatalf("label before prefix: %+v", got)
	}
}

func TestResolveNeverChangesItsInputs(t *testing.T) {
	t.Parallel()
	all := resolveList()
	live := []model.Agent{all[3], all[1], all[0]}
	beforeAll, beforeLive := append([]model.Agent(nil), all...), append([]model.Agent(nil), live...)
	newImpl().Resolve(all, live, "alpha")
	newImpl().Resolve(all, live, "tutor")
	if !reflect.DeepEqual(all, beforeAll) || !reflect.DeepEqual(live, beforeLive) {
		t.Fatalf("an input changed: %v %v", names(all), names(live))
	}
}

func TestResolveDoesNotMatchTheMiddleOrEndOfAHandleOrALabel(t *testing.T) {
	t.Parallel()
	all := resolveList()
	if got, _ := newImpl().Resolve(all, all, "alpha-1k2"); got.Name != "alpha-1k2" {
		t.Fatalf("the whole handle: %+v", got)
	}
	for _, query := range []string{"lpha", "ravo", "1k2", "k2", "utor", "tor", "viewer", "-"} {
		if got, candidates := newImpl().Resolve(all, all, query); got.Name != "" || len(candidates) != 0 {
			t.Errorf("%q: %+v %v", query, got, names(candidates))
		}
	}
}

func TestResolveDoesNotTrimTheQuery(t *testing.T) {
	t.Parallel()
	all := resolveList()
	if got, _ := newImpl().Resolve(all, all, "alpha-1k2"); got.Name != "alpha-1k2" {
		t.Fatalf("the untrimmed handle: %+v", got)
	}
	for _, query := range []string{" alpha-1k2", "alpha-1k2 ", "\talpha-1k2", " reviewer", "reviewer\n", " bra", "bra ", " ", "\u00a0reviewer"} {
		if got, candidates := newImpl().Resolve(all, all, query); got.Name != "" || len(candidates) != 0 {
			t.Errorf("%q: %+v %v", query, got, names(candidates))
		}
	}
}

func TestResolveMatchesHandlesAndLabelsOnlyInTheCaseTheyWereGiven(t *testing.T) {
	t.Parallel()
	all := []model.Agent{
		{Name: "alpha", Harness: "h", Label: "Tutor"},
		{Name: "bravo", Harness: "h", Label: "Reviewer"},
	}
	for _, query := range []string{"ALPHA", "Alpha", "aLPHA", "tutor", "TUTOR", "reviewer", "REV", "rev", "BRA", "tu"} {
		if got, candidates := newImpl().Resolve(all, all, query); got.Name != "" || len(candidates) != 0 {
			t.Errorf("%q: %+v %v", query, got, names(candidates))
		}
	}
	for query, want := range map[string]string{"alpha": "alpha", "Tutor": "alpha", "Rev": "bravo", "Tu": "alpha"} {
		if got, _ := newImpl().Resolve(all, all, query); got.Name != want {
			t.Errorf("%q: %+v, want %s", query, got, want)
		}
	}
}

func TestResolveALabelTieIsNotBrokenByAPrefixMatch(t *testing.T) {
	t.Parallel()
	all := []model.Agent{
		{Name: "alpha", Harness: "h", Label: "tutor"},
		{Name: "bravo", Harness: "h", Label: "tutor"},
		{Name: "tutor-x", Harness: "h"},
		{Name: "tutor-y", Harness: "h", Label: "tutor"},
	}
	got, candidates := newImpl().Resolve(all, all, "tutor")
	if got.Name != "" || !reflect.DeepEqual(names(candidates), []string{"alpha", "bravo", "tutor-y"}) {
		t.Fatalf("got %+v %v", got, names(candidates))
	}
}

func TestResolveListsAnAgentOnceWhenItsHandleAndItsLabelBothStartWithTheQuery(t *testing.T) {
	t.Parallel()
	all := []model.Agent{
		{Name: "tutor-1", Harness: "h", Label: "tutor"},
		{Name: "tutor-2", Harness: "h", Label: "tutor-two"},
		{Name: "zulu", Harness: "h"},
	}
	got, candidates := newImpl().Resolve(all, all, "tut")
	if got.Name != "" || !reflect.DeepEqual(names(candidates), []string{"tutor-1", "tutor-2"}) {
		t.Fatalf("got %+v %v", got, names(candidates))
	}
	single := all[:1]
	if got, candidates := newImpl().Resolve(single, single, "tut"); got.Name != "tutor-1" || candidates != nil {
		t.Fatalf("one agent matching twice is one match: %+v %v", got, names(candidates))
	}
}

func TestAHarnessOfOnlyAnInvisibleCharacterThatIsNotWhiteSpaceIsABadFieldNotNoHarness(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"\u200b", "\u200b\u200b", " \u200b ", "\x00", "\x1f", "\ufeff"} {
		entry := agent("alpha")
		entry.Harness = harness
		if _, err := newImpl().Register(nil, entry, t0); !errors.Is(err, model.ErrBadField) {
			t.Errorf("%q: %v", harness, err)
		}
	}
}

func TestEveryFieldRefusesInvisibleFormattingCharacters(t *testing.T) {
	t.Parallel()
	formatting := []string{"\u00ad", "\u0600", "\u061c", "\u180e", "\u200b", "\u200c", "\u200d", "\u200e", "\u200f", "\u202a", "\u202d", "\u202e", "\u2060", "\u2066", "\u2069", "\ufeff", "\U000e0001", "\U000e0041"}
	for _, field := range []string{"Harness", "Host", "Session", "Folder", "Branch", "Worktree", "HarnessVersion", "Model", "Effort", "Label"} {
		for _, char := range formatting {
			for _, text := range []string{"a" + char + "b", char + "codex", "codex" + char} {
				entry := agent("alpha")
				setField(&entry, field, text)
				if _, err := newImpl().Register(nil, entry, t0); !errors.Is(err, model.ErrBadField) {
					t.Errorf("%s %q: %v", field, text, err)
				}
			}
		}
	}
}
