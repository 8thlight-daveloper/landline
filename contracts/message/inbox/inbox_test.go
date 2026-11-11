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
