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
	return model.Agent{ID: name, Harness: "codex", Host: "mac", Session: "s-" + name}
}

func names(agents []model.Agent) []string {
	out := []string{}
	for _, a := range agents {
		out = append(out, a.ID)
	}
	return out
}

func TestRegisterAddsAnAgentAndStampsLastSeen(t *testing.T) {
	t.Parallel()
	got, err := newImpl().Register(nil, agent("alpha"), t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "alpha" || !got[0].LastSeen.Equal(t0) || got[0].Harness != "codex" {
		t.Fatalf("got %+v", got)
	}
}

func TestRegisterKeepsAgentsSortedByID(t *testing.T) {
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

func TestRegisterRefusesABadIDOrAMissingHarness(t *testing.T) {
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

func TestFindReturnsTheAgentWithThatID(t *testing.T) {
	t.Parallel()
	r := newImpl()
	agents, _ := r.Register(nil, agent("alpha"), t0)
	if got, ok := r.Find(agents, "alpha"); !ok || got.ID != "alpha" {
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

func TestAnIDThatSortsBeforeAllIsStillAValidID(t *testing.T) {
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
		if got, ok := r.Find(agents, name); !ok || got.ID != name {
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
	if got[0].Session != "s-new" || got[1].ID != "bravo" {
		t.Fatalf("got %+v", got)
	}
}

func TestAnIDOfExactlyThirtyTwoCharactersIsValidAndThirtyThreeIsNot(t *testing.T) {
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
		{ID: "charlie", Harness: "h", LastSeen: t0},
		{ID: "alpha", Harness: "h", LastSeen: t0},
		{ID: "bravo", Harness: "h", LastSeen: t0},
	}
	before := append([]model.Agent(nil), unsorted...)
	newImpl().Register(unsorted, agent("delta"), t0)
	if !reflect.DeepEqual(unsorted, before) {
		t.Fatalf("Register reordered the input: %+v", unsorted)
	}
}

func TestFindMatchesTheIDExactly(t *testing.T) {
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

func TestIDsAreLowercaseOnlyAndOnlyTheExactIDAllIsReserved(t *testing.T) {
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
	entry := model.Agent{ID: "alpha", Harness: "Codex CLI", Host: " mac book ", Session: " s 1 "}
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

func TestFaultsAreCheckedIDThenHarnessThenFields(t *testing.T) {
	t.Parallel()
	both := agent("BAD ID")
	both.Harness = ""
	both.Host = "bad\nhost"
	if _, err := newImpl().Register(nil, both, t0); !errors.Is(err, model.ErrBadName) {
		t.Fatalf("id and harness: %v", err)
	}
	noHarness := agent("alpha")
	noHarness.Harness = ""
	noHarness.Session = "bad\nsession"
	if _, err := newImpl().Register(nil, noHarness, t0); !errors.Is(err, model.ErrNoHarness) {
		t.Fatalf("harness and field: %v", err)
	}
}

func TestIDsMayStartWithADigitAndHoldHyphensAndUnderscores(t *testing.T) {
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
	case "LandlineVersion":
		entry.LandlineVersion = value
	case "LandlineCommit":
		entry.LandlineCommit = value
	case "LandlineDate":
		entry.LandlineDate = value
	case "Model":
		entry.Model = value
	case "Effort":
		entry.Effort = value
	case "Project":
		entry.Project = value
	case "Task":
		entry.Task = value
	}
}

func TestEveryFieldAcceptsExactlySixtyFourBytesAndRefusesTheUnicodeLineBreaks(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"Harness", "Host", "Session", "Folder", "Branch", "Worktree", "HarnessVersion", "LandlineVersion", "LandlineCommit", "LandlineDate", "Model", "Effort", "Project", "Task"} {
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
	entry.LandlineVersion = "0.2.0"
	entry.LandlineCommit, entry.LandlineDate = fullCommit, buildDate
	entry.Model, entry.Effort, entry.Project = "claude-opus-5-5", "medium", "  main reviewer  "
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
	if err != nil || bare[0].Project != "" || bare[0].Model != "" {
		t.Fatalf("empty attributes: %+v, %v", bare, err)
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
	for _, field := range []string{"Harness", "Host", "Session", "Folder", "Branch", "Worktree", "HarnessVersion", "LandlineVersion", "LandlineCommit", "LandlineDate", "Model", "Effort", "Project", "Task"} {
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

func TestALandlineVersionIsStoredAndFoundUnchanged(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"0.2.0", "dev", " 0.2.0-rc 1 ", strings.Repeat("v", 64)} {
		entry := agent("alpha")
		entry.LandlineVersion = version
		agents, err := newImpl().Register(nil, entry, t0)
		if err != nil {
			t.Fatalf("%q: %v", version, err)
		}
		found, ok := newImpl().Find(agents, "alpha")
		if !ok || found.LandlineVersion != version {
			t.Fatalf("%q: found %+v, %v", version, found, ok)
		}
		bare, _ := newImpl().Register(agents, agent("alpha"), t0)
		if bare[0].LandlineVersion != "" {
			t.Fatalf("re-registering without a version kept %q", bare[0].LandlineVersion)
		}
	}
}

func TestAnOverLongOrInvisibleLandlineVersionIsRefused(t *testing.T) {
	t.Parallel()
	for _, version := range []string{strings.Repeat("v", 65), "0.2.0\u202e", "\u200b0.2.0", "0.\u20602.0", "0.2\n.0", "0.2.0\u2028"} {
		entry := agent("alpha")
		entry.LandlineVersion = version
		if _, err := newImpl().Register(nil, entry, t0); !errors.Is(err, model.ErrBadField) {
			t.Errorf("%q: %v", version, err)
		}
	}
}

const (
	fullCommit = "0123456789abcdef0123456789abcdef01234567"
	buildDate  = "2026-11-12T16:34:00Z"
)

func TestALandlineCommitAndDateAreStoredAndFoundUnchanged(t *testing.T) {
	t.Parallel()
	for _, pair := range [][2]string{
		{fullCommit, buildDate},
		{fullCommit + "-dirty", ""},
		{"", ""},
		{"", buildDate},
		{"  not a hash  ", " 2026-11-12 "},
		{strings.Repeat("c", 64), strings.Repeat("d", 64)},
	} {
		entry := agent("alpha")
		entry.LandlineCommit, entry.LandlineDate = pair[0], pair[1]
		agents, err := newImpl().Register(nil, entry, t0)
		if err != nil {
			t.Fatalf("%q: %v", pair, err)
		}
		found, ok := newImpl().Find(agents, "alpha")
		if !ok || found.LandlineCommit != pair[0] || found.LandlineDate != pair[1] {
			t.Fatalf("%q: found %+v, %v", pair, found, ok)
		}
	}
}

func TestACommitAndADateAreSeparateFieldsThatDoNotLeakIntoEachOtherOrIntoTheVersion(t *testing.T) {
	t.Parallel()
	entry := agent("alpha")
	entry.LandlineVersion, entry.LandlineCommit, entry.LandlineDate = "0.2.0", fullCommit, buildDate
	agents, err := newImpl().Register(nil, entry, t0)
	if err != nil {
		t.Fatal(err)
	}
	if got := agents[0]; got.LandlineVersion != "0.2.0" || got.LandlineCommit != fullCommit || got.LandlineDate != buildDate {
		t.Fatalf("got %+v", got)
	}
}

func TestRegisteringAgainReplacesTheCommitAndTheDateAndAnEmptyValueClearsThem(t *testing.T) {
	t.Parallel()
	r := newImpl()
	first := agent("alpha")
	first.LandlineCommit, first.LandlineDate = fullCommit, buildDate
	agents, err := r.Register(nil, first, t0)
	if err != nil {
		t.Fatal(err)
	}
	second := agent("alpha")
	second.LandlineCommit, second.LandlineDate = strings.Repeat("a", 40)+"-dirty", ""
	agents, err = r.Register(agents, second, t0.Add(time.Second))
	if err != nil || len(agents) != 1 || agents[0].LandlineCommit != second.LandlineCommit || agents[0].LandlineDate != "" {
		t.Fatalf("replace: %+v, %v", agents, err)
	}
	agents, err = r.Register(agents, agent("alpha"), t0.Add(2*time.Second))
	if err != nil || len(agents) != 1 || agents[0].LandlineCommit != "" || agents[0].LandlineDate != "" {
		t.Fatalf("clear: %+v, %v", agents, err)
	}
}

func TestAnOverLongOrInvisibleCommitOrDateIsRefusedAndTheListIsUnchanged(t *testing.T) {
	t.Parallel()
	bad := []string{strings.Repeat("c", 65), fullCommit + "\u202e", "\u200b" + fullCommit, "01\u206023", "0123\n4567", "0123\u20284567", "01\x0023", "01\t23", "\U000e0041" + fullCommit}
	for _, field := range []string{"LandlineCommit", "LandlineDate"} {
		for _, value := range bad {
			existing, _ := newImpl().Register(nil, agent("bravo"), t0)
			before := append([]model.Agent(nil), existing...)
			entry := agent("alpha")
			setField(&entry, field, value)
			got, err := newImpl().Register(existing, entry, t0)
			if !errors.Is(err, model.ErrBadField) {
				t.Errorf("%s %q: %v", field, value, err)
			}
			if got != nil || !reflect.DeepEqual(existing, before) {
				t.Errorf("%s %q: returned %+v, input %+v", field, value, got, existing)
			}
		}
	}
}

func TestACommitOfFortySixBytesWithTheDirtyMarkIsAccepted(t *testing.T) {
	t.Parallel()
	entry := agent("alpha")
	entry.LandlineCommit = fullCommit + "-dirty"
	if len(entry.LandlineCommit) != 46 {
		t.Fatal("fixture is not 46 bytes")
	}
	if _, err := newImpl().Register(nil, entry, t0); err != nil {
		t.Fatal(err)
	}
}

func TestABadCommitDoesNotHideABadIDOrAMissingHarness(t *testing.T) {
	t.Parallel()
	badID := agent("Alpha")
	badID.LandlineCommit = strings.Repeat("c", 65)
	if _, err := newImpl().Register(nil, badID, t0); !errors.Is(err, model.ErrBadName) {
		t.Errorf("got %v, want ErrBadName", err)
	}
	noHarness := agent("alpha")
	noHarness.Harness, noHarness.LandlineDate = "", "bad\ndate"
	if _, err := newImpl().Register(nil, noHarness, t0); !errors.Is(err, model.ErrNoHarness) {
		t.Errorf("got %v, want ErrNoHarness", err)
	}
}

var taskTime = time.Date(2026, 11, 16, 9, 30, 15, 0, time.UTC)

func TestATaskAndItsTimeAreStoredAndFoundUnchanged(t *testing.T) {
	t.Parallel()
	entry := agent("alpha")
	entry.Project, entry.Task, entry.TaskAt = "landline", "  menu bar app for ADR 0140  ", taskTime
	agents, err := newImpl().Register(nil, entry, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	found, ok := newImpl().Find(agents, "alpha")
	if !ok || found.Task != entry.Task || !found.TaskAt.Equal(taskTime) || found.Project != "landline" {
		t.Fatalf("found %+v, %v", found, ok)
	}
	if !found.LastSeen.Equal(t0.Add(time.Hour)) {
		t.Fatalf("the task time changed the last seen time: %+v", found)
	}
}

func TestRegisteringAnIDAgainReplacesTheWholeEntryAndSoClearsTheTaskAndItsTime(t *testing.T) {
	t.Parallel()
	r := newImpl()
	first := agent("alpha")
	first.Project, first.Task, first.TaskAt = "landline", "ship ADR 0160", taskTime
	agents, err := r.Register(nil, first, t0)
	if err != nil {
		t.Fatal(err)
	}
	agents, err = r.Register(agents, agent("alpha"), t0.Add(time.Second))
	if err != nil || len(agents) != 1 {
		t.Fatalf("%+v, %v", agents, err)
	}
	if got := agents[0]; got.Task != "" || !got.TaskAt.IsZero() || got.Project != "" {
		t.Fatalf("the old entry leaked into the new one: %+v", got)
	}
	other := agent("bravo")
	other.Task, other.TaskAt = "keep me", taskTime
	agents, _ = r.Register(agents, other, t0)
	agents, _ = r.Register(agents, agent("alpha"), t0)
	if got, _ := r.Find(agents, "bravo"); got.Task != "keep me" || !got.TaskAt.Equal(taskTime) {
		t.Fatalf("registering another id changed bravo: %+v", got)
	}
}

func TestATaskOfExactlySixtyFourBytesIsAcceptedAndASixtyFifthByteIsRefusedWhateverTheCharacters(t *testing.T) {
	t.Parallel()
	for _, task := range []string{strings.Repeat("t", 64), strings.Repeat("\u00e9", 32), "", " "} {
		entry := agent("alpha")
		entry.Task = task
		if _, err := newImpl().Register(nil, entry, t0); err != nil {
			t.Errorf("%q: %v", task, err)
		}
	}
	for _, task := range []string{strings.Repeat("t", 65), strings.Repeat("\u00e9", 33), strings.Repeat("\u4e2d", 22)} {
		entry := agent("alpha")
		entry.Task = task
		if _, err := newImpl().Register(nil, entry, t0); !errors.Is(err, model.ErrBadField) {
			t.Errorf("%d bytes: %v", len(task), err)
		}
	}
}

func TestATaskRefusesControlLineSeparatorAndInvisibleFormattingCharactersAndTheListIsUnchanged(t *testing.T) {
	t.Parallel()
	existing, _ := newImpl().Register(nil, agent("bravo"), t0)
	before := append([]model.Agent(nil), existing...)
	for _, task := range []string{"a\nb", "a\rb", "a\tb", "a\x00b", "a\x7fb", "a\u0085b", "a\u2028b", "a\u2029b", "a\u202eb", "a\u200bb", "a\u2066b", "\ufeffa", "a\U000e0041"} {
		entry := agent("alpha")
		entry.Task = task
		got, err := newImpl().Register(existing, entry, t0)
		if !errors.Is(err, model.ErrBadField) || got != nil {
			t.Errorf("%q: %+v, %v", task, got, err)
		}
		if !reflect.DeepEqual(existing, before) {
			t.Fatalf("the list changed: %+v", existing)
		}
	}
}

func TestABadTaskDoesNotHideABadIDOrAMissingHarness(t *testing.T) {
	t.Parallel()
	badID := agent("Alpha")
	badID.Task = "bad\ntask"
	if _, err := newImpl().Register(nil, badID, t0); !errors.Is(err, model.ErrBadName) {
		t.Errorf("got %v, want ErrBadName", err)
	}
	noHarness := agent("alpha")
	noHarness.Harness, noHarness.Task = "", strings.Repeat("t", 65)
	if _, err := newImpl().Register(nil, noHarness, t0); !errors.Is(err, model.ErrNoHarness) {
		t.Errorf("got %v, want ErrNoHarness", err)
	}
}

func TestAnIDOfFourLowercaseHexCharactersIsValid(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"a3f9", "0000", "ffff", "09af", "b7c2"} {
		if _, err := newImpl().Register(nil, agent(id), t0); err != nil {
			t.Errorf("%q: %v", id, err)
		}
	}
	for _, id := range []string{"A3F9", "a3 f9", "a3f9\n", "a3f9\x00", "\u00e0f9"} {
		if _, err := newImpl().Register(nil, agent(id), t0); !errors.Is(err, model.ErrBadName) {
			t.Errorf("%q: %v", id, err)
		}
	}
}

func TestARegistrationWhoseIDIsAlreadyListedReplacesItAndDoesNotAddASecondEntry(t *testing.T) {
	t.Parallel()
	r := newImpl()
	agents, _ := r.Register(nil, agent("a3f9"), t0)
	agents, _ = r.Register(agents, agent("b7c2"), t0)
	agents, err := r.Register(agents, agent("a3f9"), t0.Add(time.Second))
	if err != nil || !reflect.DeepEqual(names(agents), []string{"a3f9", "b7c2"}) {
		t.Fatalf("%v, %v", names(agents), err)
	}
}

func TestFindMatchesOnlyAnExactIDNotAProjectAPrefixAnEmptyIDOrAnyOtherField(t *testing.T) {
	t.Parallel()
	entry := agent("a3f9")
	entry.Project, entry.Task, entry.Folder = "landline", "ship ADR 0160", "landline-checkout"
	entry.LandlineCommit, entry.LandlineDate = fullCommit, buildDate
	agents, _ := newImpl().Register(nil, entry, t0)
	other := agent("a3f9x")
	agents, _ = newImpl().Register(agents, other, t0)
	for _, query := range []string{"landline", "land", "a3", "a3f", "f9", "A3F9", " a3f9", "a3f9 ", "", "ship ADR 0160", "landline-checkout", fullCommit, buildDate, "codex", "mac", "s-a3f9"} {
		if got, ok := newImpl().Find(agents, query); ok {
			t.Errorf("%q matched %+v", query, got)
		}
	}
	if got, ok := newImpl().Find(agents, "a3f9"); !ok || got.ID != "a3f9" || got.Project != "landline" {
		t.Fatalf("exact id: %+v, %v", got, ok)
	}
	if got, ok := newImpl().Find(agents, "a3f9x"); !ok || got.ID != "a3f9x" {
		t.Fatalf("the longer id: %+v, %v", got, ok)
	}
}

func TestFindLooksAmongEveryAgentItIsGivenAndChangesNothing(t *testing.T) {
	t.Parallel()
	agents := []model.Agent{{ID: "charlie", Harness: "h"}, {ID: "alpha", Harness: "h"}, {ID: "bravo", Harness: "h"}}
	before := append([]model.Agent(nil), agents...)
	for _, id := range []string{"alpha", "bravo", "charlie"} {
		if got, ok := newImpl().Find(agents, id); !ok || got.ID != id {
			t.Errorf("%s: %+v %v", id, got, ok)
		}
	}
	if !reflect.DeepEqual(agents, before) {
		t.Fatalf("Find changed its input: %+v", agents)
	}
}

func TestTheErrorForABadIDSaysAgentIDAndNotAgentName(t *testing.T) {
	t.Parallel()
	_, err := newImpl().Register(nil, agent("Bad ID"), t0)
	if !errors.Is(err, model.ErrBadName) {
		t.Fatalf("got %v", err)
	}
	text := err.Error()
	if !strings.HasPrefix(text, "agent id ") || strings.Contains(text, "name") {
		t.Fatalf("the error text is %q", text)
	}
}
