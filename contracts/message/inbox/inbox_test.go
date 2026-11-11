package inbox

import (
	"reflect"
	"testing"
	"time"

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

func TestAnAgentDoesNotSeeItsOwnBroadcast(t *testing.T) {
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

func TestEveryNamedThreadIsKept(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{
		env(1, "bravo", "alpha", "main"),
		env(2, "bravo", "alpha", "deploy"),
		env(3, "bravo", "alpha", "review"),
		env(4, "bravo", "alpha", "other"),
	}
	got := newImpl().Select(l, "alpha", 0, []string{"review", "main", "deploy"}, 0)
	if want := []int64{1, 2, 3}; !reflect.DeepEqual(seqs(got), want) {
		t.Fatalf("got %v, want %v", seqs(got), want)
	}
}

func TestSelectNeverReordersTheCallersLog(t *testing.T) {
	t.Parallel()
	shuffled := []model.Envelope{env(3, "bravo", "alpha", "main"), env(1, "bravo", "alpha", "main"), env(2, "bravo", "alpha", "main")}
	before := append([]model.Envelope(nil), shuffled...)
	newImpl().Select(shuffled, "alpha", 0, nil, 0)
	if !reflect.DeepEqual(shuffled, before) {
		t.Fatalf("the log was reordered: %+v", shuffled)
	}
}

func TestABroadcastInAThreadNobodyAskedForIsLeftOut(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{
		env(1, "bravo", model.Broadcast, "deploy"),
		env(2, "bravo", model.Broadcast, "main"),
	}
	got := newImpl().Select(l, "alpha", 0, []string{"deploy"}, 0)
	if want := []int64{1}; !reflect.DeepEqual(seqs(got), want) {
		t.Fatalf("got %v, want %v", seqs(got), want)
	}
}

func TestAThreadIsMatchedWholeAndExactly(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{
		env(1, "bravo", "alpha", "deploy"),
		env(2, "bravo", "alpha", "deploy-2"),
		env(3, "bravo", "alpha", "dep"),
		env(4, "bravo", "alpha", "Deploy"),
	}
	got := newImpl().Select(l, "alpha", 0, []string{"deploy"}, 0)
	if want := []int64{1}; !reflect.DeepEqual(seqs(got), want) {
		t.Fatalf("got %v, want %v", seqs(got), want)
	}
}

func TestAMessageAddressedToTheAgentByNameIsItsEvenIfItSentIt(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{env(1, "alpha", "alpha", "main")}
	if got := newImpl().Select(l, "alpha", 0, nil, 0); len(got.Messages) != 1 {
		t.Fatalf("got %v", seqs(got))
	}
}

func TestAnAgentNamedAsAPrefixOfAnotherSeesOnlyItsOwnMail(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{
		env(1, "bravo", "alpha", "main"),
		env(2, "bravo", "alphabet", "main"),
		env(3, "bravo", "alph", "main"),
		env(4, "alpha2", model.Broadcast, "main"),
		env(5, "alph", model.Broadcast, "main"),
		env(6, "alpha", model.Broadcast, "main"),
	}
	got := newImpl().Select(l, "alpha", 0, nil, 0)
	if want := []int64{1, 4, 5}; !reflect.DeepEqual(seqs(got), want) {
		t.Fatalf("got %v, want %v", seqs(got), want)
	}
	if got := newImpl().Select(l, "alphabet", 0, nil, 0); !reflect.DeepEqual(seqs(got), []int64{2, 4, 5, 6}) {
		t.Fatalf("alphabet got %v", seqs(got))
	}
}

func TestNamesAndThreadsMatchWholeNotBySuffixOrCase(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{
		env(1, "bravo", "xalpha", "main"),
		env(2, "bravo", "Alpha", "main"),
		env(3, "xalpha", model.Broadcast, "main"),
		env(4, "ALPHA", model.Broadcast, "main"),
		env(5, "bravo", "alpha", "xdeploy"),
		env(6, "bravo", "alpha", "Deploy"),
		env(7, "bravo", "alpha", "deploy"),
	}
	if got := newImpl().Select(l, "alpha", 0, nil, 0); !reflect.DeepEqual(seqs(got), []int64{3, 4, 5, 6, 7}) {
		t.Fatalf("no filter: %v", seqs(got))
	}
	if got := newImpl().Select(l, "alpha", 0, []string{"deploy"}, 0); !reflect.DeepEqual(seqs(got), []int64{7}) {
		t.Fatalf("filter: %v", seqs(got))
	}
}

func TestTheLimitIsHonouredWithAThreadFilterToo(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{
		env(1, "bravo", "alpha", "deploy"),
		env(2, "bravo", "alpha", "deploy"),
		env(3, "bravo", "alpha", "deploy"),
		env(4, "bravo", "alpha", "main"),
	}
	got := newImpl().Select(l, "alpha", 0, []string{"deploy"}, 2)
	if !reflect.DeepEqual(seqs(got), []int64{1, 2}) || got.Remaining != 1 || got.Next != 2 {
		t.Fatalf("got %v remaining %d next %d", seqs(got), got.Remaining, got.Next)
	}
}

func TestAnEmptyThreadNameInTheFilterMatchesNothingRatherThanEverything(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{env(1, "bravo", "alpha", "main")}
	if got := newImpl().Select(l, "alpha", 0, []string{""}, 0); len(got.Messages) != 0 {
		t.Fatalf("got %v", seqs(got))
	}
}

func manyMessages(count int) []model.Envelope {
	out := make([]model.Envelope, count)
	for i := range out {
		out[i] = env(int64(i+1), "bravo", "alpha", "main")
	}
	return out
}

func TestRemainingCountsOnlyMessagesAfterTheCursor(t *testing.T) {
	t.Parallel()
	l := manyMessages(10)
	got := newImpl().Select(l, "alpha", 4, nil, 2)
	if !reflect.DeepEqual(seqs(got), []int64{5, 6}) || got.Remaining != 4 || got.Next != 6 {
		t.Fatalf("got %v remaining %d next %d", seqs(got), got.Remaining, got.Next)
	}
}

func TestADuplicateThreadNameInTheFilterDoesNotDuplicateMessages(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{env(1, "bravo", "alpha", "deploy")}
	if got := newImpl().Select(l, "alpha", 0, []string{"deploy", "deploy", "deploy"}, 0); !reflect.DeepEqual(seqs(got), []int64{1}) {
		t.Fatalf("got %v", seqs(got))
	}
}

func TestReturnedMessagesAreTheOriginalsUnchanged(t *testing.T) {
	t.Parallel()
	original := model.Envelope{Seq: 3, ID: "m-3", Time: time.Date(2031, 1, 2, 3, 4, 5, 6, time.UTC), Thread: "", From: "bravo", To: "alpha", Kind: model.KindReview, Body: "  padded body \n", InReplyTo: "m-2"}
	got := newImpl().Select([]model.Envelope{original}, "alpha", 0, nil, 0)
	if len(got.Messages) != 1 || got.Messages[0] != original {
		t.Fatalf("got %+v", got.Messages)
	}
}

func TestThereIsNoCapOnTheLimitOrOnTheLengthOfTheLog(t *testing.T) {
	t.Parallel()
	l := manyMessages(1500)
	if got := newImpl().Select(l, "alpha", 0, nil, 1200); len(got.Messages) != 1200 || got.Remaining != 300 {
		t.Fatalf("limit 1200: %d messages remaining %d", len(got.Messages), got.Remaining)
	}
	if got := newImpl().Select(l, "alpha", 0, nil, 0); len(got.Messages) != 1500 || got.Next != 1500 {
		t.Fatalf("no limit: %d messages next %d", len(got.Messages), got.Next)
	}
}

func TestANegativeCursorIsReturnedAsItIsWhenNothingMatches(t *testing.T) {
	t.Parallel()
	got := newImpl().Select(nil, "alpha", -5, nil, 0)
	if got.Next != -5 || len(got.Messages) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestThreadNamesInTheFilterAreNotTrimmed(t *testing.T) {
	t.Parallel()
	l := []model.Envelope{env(1, "bravo", "alpha", "deploy")}
	if got := newImpl().Select(l, "alpha", 0, []string{" deploy", "deploy "}, 0); len(got.Messages) != 0 {
		t.Fatalf("got %v", seqs(got))
	}
}
