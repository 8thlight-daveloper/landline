package registry

import (
	"errors"
	"reflect"
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
