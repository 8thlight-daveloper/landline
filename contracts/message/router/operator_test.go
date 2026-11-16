package router

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

func TestTheOperatorIsAKnownSenderThatIsNotInTheAgentList(t *testing.T) {
	t.Parallel()
	got, err := send(t, "operator", model.SendRequest{To: "bravo", Body: "hello"}, "alpha", "bravo")
	if err != nil {
		t.Fatal(err)
	}
	want := model.Envelope{Seq: 7, ID: "m-1", Time: t0, Thread: model.DefaultThread, From: "operator", To: "bravo", Kind: model.KindChat, Body: "hello"}
	if got.Envelope != want {
		t.Fatalf("got %+v, want %+v", got.Envelope, want)
	}
	if !reflect.DeepEqual(got.Recipients, []string{"bravo"}) {
		t.Fatalf("recipients %v", got.Recipients)
	}
}

func TestTheOperatorIsAKnownSenderWhenTheAgentListIsEmpty(t *testing.T) {
	t.Parallel()
	if _, err := send(t, "operator", model.SendRequest{To: "bravo", Body: "x"}); !errors.Is(err, model.ErrUnknownRecipient) {
		t.Fatalf("got %v, want ErrUnknownRecipient and not ErrUnknownSender", err)
	}
}

func TestAnOperatorMessageKeepsItsThreadKindAndInReplyTo(t *testing.T) {
	t.Parallel()
	got, err := send(t, "operator", model.SendRequest{To: "bravo", Thread: "deploy", Kind: model.KindTask, Body: "ship it", InReplyTo: "m-0"}, "bravo")
	if err != nil {
		t.Fatal(err)
	}
	e := got.Envelope
	if e.From != "operator" || e.Thread != "deploy" || e.Kind != model.KindTask || e.InReplyTo != "m-0" || e.Body != "ship it" {
		t.Fatalf("envelope %+v", e)
	}
}

func TestTheOperatorIsAKnownRecipientThatIsNotInTheAgentList(t *testing.T) {
	t.Parallel()
	got, err := send(t, "alpha", model.SendRequest{To: "operator", Kind: model.KindReply, Body: "done", InReplyTo: "m-0"}, "alpha", "bravo")
	if err != nil {
		t.Fatal(err)
	}
	want := model.Envelope{Seq: 7, ID: "m-1", Time: t0, Thread: model.DefaultThread, From: "alpha", To: "operator", Kind: model.KindReply, Body: "done", InReplyTo: "m-0"}
	if got.Envelope != want {
		t.Fatalf("got %+v, want %+v", got.Envelope, want)
	}
	if !reflect.DeepEqual(got.Recipients, []string{"operator"}) {
		t.Fatalf("recipients %v", got.Recipients)
	}
}

func TestAMessageFromTheOperatorToTheOperatorIsASelfSend(t *testing.T) {
	t.Parallel()
	if got, err := send(t, "operator", model.SendRequest{To: "operator", Body: "x"}, "alpha"); !errors.Is(err, model.ErrSelfSend) {
		t.Fatalf("got %+v, %v, want ErrSelfSend", got, err)
	}
}

func TestAnOperatorSelfSendFollowsTheFixedOrderOfRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  model.SendRequest
		want error
	}{
		{"a bad thread", model.SendRequest{To: "operator", Thread: "Bad Thread", Body: "x"}, model.ErrBadThread},
		{"a bad kind", model.SendRequest{To: "operator", Kind: "shout", Body: "x"}, model.ErrBadKind},
		{"a bad in_reply_to", model.SendRequest{To: "operator", Body: "x", InReplyTo: "a b"}, model.ErrBadReplyTo},
		{"an empty body", model.SendRequest{To: "operator", Body: "  "}, model.ErrEmptyBody},
		{"a large body", model.SendRequest{To: "operator", Body: strings.Repeat("x", model.MaxBodyBytes+1)}, model.ErrBodyTooLarge},
	}
	for _, c := range cases {
		if _, err := send(t, "operator", c.req, "alpha"); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestTheOperatorFollowsEveryRuleOfASendToAnAgent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  model.SendRequest
		want error
	}{
		{"an unregistered recipient", model.SendRequest{To: "ghost", Body: "x"}, model.ErrUnknownRecipient},
		{"an empty recipient", model.SendRequest{To: "", Body: "x"}, model.ErrUnknownRecipient},
		{"the recipient all", model.SendRequest{To: "all", Body: "x"}, model.ErrUnknownRecipient},
		{"an empty body", model.SendRequest{To: "bravo", Body: ""}, model.ErrEmptyBody},
		{"a body of spaces", model.SendRequest{To: "bravo", Body: "   \n"}, model.ErrEmptyBody},
		{"a body over 64 KiB", model.SendRequest{To: "bravo", Body: strings.Repeat("x", model.MaxBodyBytes+1)}, model.ErrBodyTooLarge},
		{"a bad thread", model.SendRequest{To: "bravo", Thread: "Bad Thread", Body: "x"}, model.ErrBadThread},
		{"an unknown kind", model.SendRequest{To: "bravo", Kind: "shout", Body: "x"}, model.ErrBadKind},
		{"a bad in_reply_to", model.SendRequest{To: "bravo", Body: "x", InReplyTo: "m\n1"}, model.ErrBadReplyTo},
	}
	for _, c := range cases {
		if _, err := send(t, "operator", c.req, "alpha", "bravo"); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestTheNamesOperatorAndAllAreMatchedExactly(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Operator", "OPERATOR", "operator ", " operator", "operator\n", "operators", "operato", "operator-1"} {
		if _, err := send(t, name, model.SendRequest{To: "bravo", Body: "x"}, "bravo"); !errors.Is(err, model.ErrUnknownSender) {
			t.Errorf("sender %q: got %v, want ErrUnknownSender", name, err)
		}
		if _, err := send(t, "bravo", model.SendRequest{To: name, Body: "x"}, "bravo"); !errors.Is(err, model.ErrUnknownRecipient) {
			t.Errorf("recipient %q: got %v, want ErrUnknownRecipient", name, err)
		}
	}
}

func TestAnUnregisteredSenderStaysUnknownWhenTheRecipientIsTheOperator(t *testing.T) {
	t.Parallel()
	if _, err := send(t, "ghost", model.SendRequest{To: "operator", Body: "x"}, "alpha"); !errors.Is(err, model.ErrUnknownSender) {
		t.Fatalf("got %v, want ErrUnknownSender", err)
	}
}

func TestRoutingToOrFromTheOperatorNeverChangesTheCallersAgentList(t *testing.T) {
	t.Parallel()
	agents := known("bravo", "alpha")
	before := append([]model.Agent(nil), agents...)
	if _, err := newImpl().Route(agents, "operator", model.SendRequest{To: "alpha", Body: "x"}, "m-1", 1, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := newImpl().Route(agents, "alpha", model.SendRequest{To: "operator", Body: "x"}, "m-2", 2, t0); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(agents, before) {
		t.Fatalf("the list changed: %+v", agents)
	}
}

func TestADecisionToTheOperatorNamesExactlyOneRecipient(t *testing.T) {
	t.Parallel()
	got, err := send(t, "alpha", model.SendRequest{To: "operator", Body: "x"}, "alpha")
	if err != nil || got.Recipients == nil || len(got.Recipients) != 1 || got.Recipients[0] != "operator" {
		t.Fatalf("got %+v, %v", got, err)
	}
}
