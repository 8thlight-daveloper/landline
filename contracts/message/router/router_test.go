package router

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

var t0 = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func known(names ...string) []model.Agent {
	out := []model.Agent{}
	for _, n := range names {
		out = append(out, model.Agent{Name: n, Harness: "codex"})
	}
	return out
}

func send(t *testing.T, from string, request model.SendRequest, agents ...string) (model.Decision, error) {
	t.Helper()
	return newImpl().Route(known(agents...), from, request, "m-1", 7, t0)
}

func TestADirectMessageIsStampedFromTheAuthenticatedSender(t *testing.T) {
	t.Parallel()
	got, err := send(t, "alpha", model.SendRequest{To: "bravo", Body: "hello"}, "alpha", "bravo")
	if err != nil {
		t.Fatal(err)
	}
	e := got.Envelope
	want := model.Envelope{Seq: 7, ID: "m-1", Time: t0, Thread: model.DefaultThread, From: "alpha", To: "bravo", Kind: model.KindChat, Body: "hello"}
	if e != want {
		t.Fatalf("got %+v, want %+v", e, want)
	}
	if !reflect.DeepEqual(got.Recipients, []string{"bravo"}) {
		t.Fatalf("recipients %v", got.Recipients)
	}
}

func TestABroadcastGoesToEveryoneButTheSender(t *testing.T) {
	t.Parallel()
	got, err := send(t, "bravo", model.SendRequest{To: model.Broadcast, Thread: "deploy", Body: "ready"}, "alpha", "bravo", "charlie")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Recipients, []string{"alpha", "charlie"}) {
		t.Fatalf("recipients %v", got.Recipients)
	}
	if got.Envelope.To != model.Broadcast || got.Envelope.Thread != "deploy" {
		t.Fatalf("envelope %+v", got.Envelope)
	}
}

func TestABroadcastWithNobodyElseHasNoRecipientsAndIsNotAnError(t *testing.T) {
	t.Parallel()
	got, err := send(t, "alpha", model.SendRequest{To: model.Broadcast, Body: "anyone?"}, "alpha")
	if err != nil || len(got.Recipients) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestAReplyKeepsItsKindAndInReplyTo(t *testing.T) {
	t.Parallel()
	got, err := send(t, "alpha", model.SendRequest{To: "bravo", Kind: model.KindReply, Body: "done", InReplyTo: "m-0"}, "alpha", "bravo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Envelope.Kind != model.KindReply || got.Envelope.InReplyTo != "m-0" {
		t.Fatalf("envelope %+v", got.Envelope)
	}
}

func TestRefusals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from string
		req  model.SendRequest
		want error
	}{
		{"an unregistered sender", "ghost", model.SendRequest{To: "bravo", Body: "x"}, model.ErrUnknownSender},
		{"an unregistered recipient", "alpha", model.SendRequest{To: "ghost", Body: "x"}, model.ErrUnknownRecipient},
		{"a message to oneself", "alpha", model.SendRequest{To: "alpha", Body: "x"}, model.ErrSelfSend},
		{"an empty body", "alpha", model.SendRequest{To: "bravo", Body: ""}, model.ErrEmptyBody},
		{"a body of spaces", "alpha", model.SendRequest{To: "bravo", Body: "   \n"}, model.ErrEmptyBody},
		{"a body over 64 KiB", "alpha", model.SendRequest{To: "bravo", Body: strings.Repeat("x", model.MaxBodyBytes+1)}, model.ErrBodyTooLarge},
		{"a bad thread", "alpha", model.SendRequest{To: "bravo", Thread: "Bad Thread", Body: "x"}, model.ErrBadThread},
		{"an unknown kind", "alpha", model.SendRequest{To: "bravo", Kind: "shout", Body: "x"}, model.ErrBadKind},
	}
	for _, c := range cases {
		_, err := send(t, c.from, c.req, "alpha", "bravo")
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestABodyOfExactlyTheLimitIsAccepted(t *testing.T) {
	t.Parallel()
	if _, err := send(t, "alpha", model.SendRequest{To: "bravo", Body: strings.Repeat("x", model.MaxBodyBytes)}, "alpha", "bravo"); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingLeavesItsInputsUnchangedAndIsDeterministic(t *testing.T) {
	t.Parallel()
	agents := known("alpha", "bravo", "charlie")
	before := append([]model.Agent(nil), agents...)
	r := newImpl()
	a, _ := r.Route(agents, "alpha", model.SendRequest{To: model.Broadcast, Body: "x"}, "m-1", 1, t0)
	b, _ := r.Route(agents, "alpha", model.SendRequest{To: model.Broadcast, Body: "x"}, "m-1", 1, t0)
	if !reflect.DeepEqual(agents, before) || !reflect.DeepEqual(a, b) {
		t.Fatalf("inputs %+v, results %+v and %+v", agents, a, b)
	}
}
