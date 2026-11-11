package inbox

import (
	"reflect"
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

func env(seq int64, from, to, thread string) model.Envelope {
	return model.Envelope{Seq: seq, From: from, To: to, Thread: thread, Body: "b"}
}

func seqs(page model.Page) []int64 {
	out := []int64{}
	for _, m := range page.Messages {
		out = append(out, m.Seq)
	}
	return out
}

var log = []model.Envelope{
	env(1, "bravo", "alpha", "main"),
	env(2, "bravo", "charlie", "main"),
	env(3, "bravo", model.Broadcast, "deploy"),
	env(4, "alpha", model.Broadcast, "main"),
	env(5, "charlie", "alpha", "deploy"),
}

func TestAnAgentSeesDirectMessagesAndBroadcastsFromOthers(t *testing.T) {
	t.Parallel()
	got := newImpl().Select(log, "alpha", 0, nil, 0)
	if want := []int64{1, 3, 5}; !reflect.DeepEqual(seqs(got), want) {
		t.Fatalf("got %v, want %v", seqs(got), want)
	}
}

func TestOnlyMessagesAfterTheCursorAreReturnedAndNextAdvances(t *testing.T) {
	t.Parallel()
	got := newImpl().Select(log, "alpha", 1, nil, 0)
	if want := []int64{3, 5}; !reflect.DeepEqual(seqs(got), want) || got.Next != 5 {
		t.Fatalf("got %v next %d", seqs(got), got.Next)
	}
}

func TestNextStaysAtTheCursorWhenNothingIsNew(t *testing.T) {
	t.Parallel()
	got := newImpl().Select(log, "alpha", 5, nil, 0)
	if len(got.Messages) != 0 || got.Next != 5 || got.Remaining != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestAThreadFilterKeepsOnlyThoseThreads(t *testing.T) {
	t.Parallel()
	got := newImpl().Select(log, "alpha", 0, []string{"deploy"}, 0)
	if want := []int64{3, 5}; !reflect.DeepEqual(seqs(got), want) {
		t.Fatalf("got %v, want %v", seqs(got), want)
	}
}

func TestALimitReturnsTheOldestFirstAndCountsTheRest(t *testing.T) {
	t.Parallel()
	got := newImpl().Select(log, "alpha", 0, nil, 2)
	if want := []int64{1, 3}; !reflect.DeepEqual(seqs(got), want) || got.Next != 3 || got.Remaining != 1 {
		t.Fatalf("got %v next %d remaining %d", seqs(got), got.Next, got.Remaining)
	}
}

func TestAMessageAnAgentSentItselfIsNotInItsInbox(t *testing.T) {
	t.Parallel()
	for _, m := range newImpl().Select(log, "alpha", 0, nil, 0).Messages {
		if m.From == "alpha" {
			t.Fatalf("own message %+v", m)
		}
	}
}

func TestSelectLeavesTheLogUnchangedAndIsDeterministic(t *testing.T) {
	t.Parallel()
	before := append([]model.Envelope(nil), log...)
	i := newImpl()
	a := i.Select(log, "alpha", 0, nil, 0)
	b := i.Select(log, "alpha", 0, nil, 0)
	if !reflect.DeepEqual(log, before) || !reflect.DeepEqual(a, b) {
		t.Fatal("changed input or gave different results")
	}
}

func TestTheResultIsOldestFirstWhateverTheOrderOfTheLog(t *testing.T) {
	t.Parallel()
	shuffled := []model.Envelope{env(3, "bravo", "alpha", "main"), env(1, "bravo", "alpha", "main"), env(2, "bravo", "alpha", "main")}
	got := newImpl().Select(shuffled, "alpha", 0, nil, 0)
	if want := []int64{1, 2, 3}; !reflect.DeepEqual(seqs(got), want) || got.Next != 3 {
		t.Fatalf("got %v next %d", seqs(got), got.Next)
	}
}

func TestALimitEqualToTheMatchesLeavesNothingRemaining(t *testing.T) {
	t.Parallel()
	got := newImpl().Select(log, "alpha", 0, nil, 3)
	if len(got.Messages) != 3 || got.Remaining != 0 {
		t.Fatalf("got %v remaining %d", seqs(got), got.Remaining)
	}
	one := newImpl().Select(log, "alpha", 0, nil, 1)
	if want := []int64{1}; !reflect.DeepEqual(seqs(one), want) || one.Remaining != 2 || one.Next != 1 {
		t.Fatalf("got %v remaining %d next %d", seqs(one), one.Remaining, one.Next)
	}
}

func TestANegativeOrZeroLimitMeansNoLimit(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{0, -1} {
		if got := newImpl().Select(log, "alpha", 0, nil, limit); len(got.Messages) != 3 || got.Remaining != 0 {
			t.Errorf("limit %d: got %v remaining %d", limit, seqs(got), got.Remaining)
		}
	}
}

func TestBroadcastsFromAnyOtherAgentAreSeenWhateverTheNameOrder(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{
		env(1, "alpha", model.Broadcast, "main"),
		env(2, "zulu", model.Broadcast, "main"),
		env(3, "bravo", model.Broadcast, "main"),
	}
	got := newImpl().Select(l, "bravo", 0, nil, 0)
	if want := []int64{1, 2}; !reflect.DeepEqual(seqs(got), want) {
		t.Fatalf("got %v, want %v", seqs(got), want)
	}
}

func TestADirectMessageToANameBeforeAllIsNotABroadcast(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{env(1, "bravo", "aaa", "main"), env(2, "bravo", "all2", "main")}
	if got := newImpl().Select(l, "alpha", 0, nil, 0); len(got.Messages) != 0 {
		t.Fatalf("got %v", seqs(got))
	}
}

func TestAnEmptyThreadListMeansAllThreadsAndANilListToo(t *testing.T) {
	t.Parallel()
	a := newImpl().Select(log, "alpha", 0, []string{}, 0)
	b := newImpl().Select(log, "alpha", 0, nil, 0)
	if !reflect.DeepEqual(seqs(a), seqs(b)) || len(a.Messages) != 3 {
		t.Fatalf("got %v and %v", seqs(a), seqs(b))
	}
}

func TestACursorBeyondTheLogReturnsNothingAndStaysPut(t *testing.T) {
	t.Parallel()
	got := newImpl().Select(log, "alpha", 99, nil, 0)
	if len(got.Messages) != 0 || got.Next != 99 {
		t.Fatalf("got %+v", got)
	}
}

func TestALimitLargerThanTheMatchesReturnsAllOfThem(t *testing.T) {
	t.Parallel()
	got := newImpl().Select(log, "alpha", 0, nil, 10)
	if want := []int64{1, 3, 5}; !reflect.DeepEqual(seqs(got), want) || got.Remaining != 0 || got.Next != 5 {
		t.Fatalf("got %v remaining %d next %d", seqs(got), got.Remaining, got.Next)
	}
}
