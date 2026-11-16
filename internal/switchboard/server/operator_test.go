package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

func (h *harness) registerOperator(kind string) string {
	h.t.Helper()
	status, raw := h.call("POST", "/v1/operators", secret, map[string]string{"kind": kind})
	if status != 200 {
		h.t.Fatalf("register operator: %d %s", status, raw)
	}
	var out struct{ Token string }
	json.Unmarshal(raw, &out)
	if out.Token == "" {
		h.t.Fatal("empty operator token")
	}
	return out.Token
}

func (h *harness) operatorSendAsync(token string, body model.SendBody) <-chan sendResult {
	result := make(chan sendResult, 1)
	go func() {
		status, raw := h.call("POST", "/v1/operator/send", token, body)
		result <- sendResult{status: status, raw: raw}
	}()
	return result
}

func TestOperatorRegistrationNeedsTheSecretAndAValidKind(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	for _, token := range []string{"", "wrong", alpha} {
		if status, _ := h.call("POST", "/v1/operators", token, map[string]string{"kind": "tui"}); status != 401 {
			t.Fatalf("token %q answered %d", token, status)
		}
	}
	operator := h.registerOperator("tui")
	for name, token := range map[string]string{"an operator": operator, "an observer": h.registerObserver("tui")} {
		if status, _ := h.call("POST", "/v1/operators", token, map[string]string{"kind": "tui"}); status != 403 {
			t.Fatalf("%s registering an operator answered %d", name, status)
		}
	}
	for name, kind := range map[string]string{"empty": "", "blank": "  \t", "long": strings.Repeat("k", 65), "control": "t\nui", "line separator": "t ui", "format": "t​ui"} {
		if status, _ := h.call("POST", "/v1/operators", secret, map[string]string{"kind": kind}); status != 400 {
			t.Fatalf("%s kind answered %d", name, status)
		}
	}
	if status, _ := h.call("POST", "/v1/operators", secret, map[string]string{}); status != 400 {
		t.Fatalf("a missing kind answered %d", status)
	}
	if status, _ := h.call("POST", "/v1/operators", secret, map[string]string{"kind": strings.Repeat("k", 64)}); status != 200 {
		t.Fatalf("a 64 byte kind answered %d", status)
	}
}

func TestAnOperatorIsNeverListedAndReadsTheListAnAgentReads(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	h.register("bravo")
	operator := h.registerOperator("tui")
	for _, each := range h.agents(operator) {
		if each.Name == "operator" || each.Name == "tui" || each.Name == operator {
			t.Fatalf("an operator is listed: %+v", each)
		}
	}
	byOperator, byAgent := h.agents(operator), h.agents(alpha)
	if len(byOperator) != 2 || !reflect.DeepEqual(byOperator, byAgent) {
		t.Fatalf("%+v and %+v", byOperator, byAgent)
	}
}

func TestAnOperatorTokenIsRefusedOnEveryOtherRouteWithNothingSentOrLogged(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	stream := h.openStream(alpha)
	operator := h.registerOperator("tui")
	routes := []struct {
		method, path string
		body         any
	}{
		{"POST", "/v1/send", model.SendBody{To: "alpha", Body: "x"}},
		{"GET", "/v1/stream", nil},
		{"POST", "/v1/attach", model.AttachRequest{}},
		{"POST", "/v1/deliveries/d-1/report", model.DeliveryReport{OK: true}},
		{"GET", "/v1/log", nil},
		{"POST", "/v1/register", model.RegisterRequest{Name: "charlie", Harness: "codex"}},
		{"POST", "/v1/observers", ObserverRequest{Kind: "tui"}},
		{"POST", "/v1/operators", map[string]string{"kind": "tui"}},
	}
	for _, route := range routes {
		status, raw := h.call(route.method, route.path, operator, route.body)
		var problem model.ErrorResponse
		json.Unmarshal(raw, &problem)
		if status != 403 || problem.Error == "" {
			t.Fatalf("%s %s answered %d %s", route.method, route.path, status, raw)
		}
	}
	if len(h.logged()) != 0 {
		t.Fatal("a refused operator wrote to the log")
	}
	if len(h.agents(alpha)) != 1 {
		t.Fatal("a refused operator changed the agents")
	}
	select {
	case line := <-stream.lines:
		if strings.HasPrefix(line, "event:") {
			t.Fatalf("a delivery went down the stream: %q", line)
		}
	case <-time.After(100 * time.Millisecond):
	}
}

func TestOnlyAnOperatorTokenMaySendOnTheOperatorRoute(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	h.openStream(bravo)
	observer := h.registerObserver("tui")
	body := model.SendBody{To: "bravo", Body: "x"}
	if status, _ := h.call("POST", "/v1/operator/send", observer, body); status != 403 {
		t.Fatalf("an observer answered %d", status)
	}
	for name, token := range map[string]string{"an agent": alpha, "the secret": secret, "no token": "", "an unknown token": "nope"} {
		status, _ := h.call("POST", "/v1/operator/send", token, body)
		want := status == 401 || status == 403
		if name == "an unknown token" || name == "no token" {
			want = status == 401
		}
		if !want {
			t.Fatalf("%s answered %d", name, status)
		}
	}
	if len(h.logged()) != 0 {
		t.Fatal("a refused send wrote to the log")
	}
}

func TestAnOperatorMessageIsDeliveredWithTheMarkerFirstAndTheOperatorStampAndLoggedFromOperator(t *testing.T) {
	t.Parallel()
	h := start(t)
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	operator := h.registerOperator("tui")
	result := h.operatorSendAsync(operator, model.SendBody{To: "bravo", Body: "hello", Thread: "deploy", Kind: model.KindTask, InReplyTo: "m-0"})
	delivery := stream.nextDelivery()
	if status := h.report(bravo, delivery.ID, true, ""); status != 200 {
		t.Fatalf("report: %d", status)
	}
	answer := h.await(result)
	var response model.SendResponse
	json.Unmarshal(answer.raw, &response)
	if answer.status != 200 || response.ID != "m-01" || response.Seq != 1 || !reflect.DeepEqual(response.Recipients, []string{"bravo"}) {
		t.Fatalf("%d %s", answer.status, answer.raw)
	}
	first, _, _ := strings.Cut(delivery.Text, "\n")
	if first != "[landline] "+model.Marker {
		t.Fatalf("first line %q", first)
	}
	m := delivery.Message
	if m == nil || m.From != "operator" || m.FromInfo != "operator on mac" || m.To != "bravo" || m.Thread != "deploy" || m.Kind != model.KindTask || m.InReplyTo != "m-0" || m.Text != delivery.Text {
		t.Fatalf("message %+v", m)
	}
	for _, want := range []string{"from: operator (operator on mac)", "label: tui", "in-reply-to: m-0"} {
		if !strings.Contains(delivery.Text, want) {
			t.Errorf("missing %q in %q", want, delivery.Text)
		}
	}
	logged := h.logged()
	if len(logged) != 1 || logged[0].From != "operator" || logged[0].To != "bravo" || logged[0].Result != "delivered" {
		t.Fatalf("log %+v", logged)
	}
}

func TestTheStampNamesTheSwitchboardsHostAndNeverAnEmptyOne(t *testing.T) {
	t.Parallel()
	h := startWith(t, func(c *Config) { c.Host = "brig" })
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	operator := h.registerOperator("cli")
	result := h.operatorSendAsync(operator, model.SendBody{To: "bravo", Body: "hello"})
	delivery := stream.nextDelivery()
	h.report(bravo, delivery.ID, true, "")
	h.await(result)
	if !strings.Contains(delivery.Text, "from: operator (operator on brig)") || !strings.Contains(delivery.Text, "label: cli") || delivery.Message.FromInfo != "operator on brig" {
		t.Fatalf("text %q info %q", delivery.Text, delivery.Message.FromInfo)
	}
}

func TestAnOperatorSendTakesNoSenderFieldAndFollowsTheRulesOfASend(t *testing.T) {
	t.Parallel()
	h := start(t)
	h.register("alpha")
	operator := h.registerOperator("tui")
	for _, field := range []string{"from", "sender", "host", "harness", "name"} {
		status, _ := h.call("POST", "/v1/operator/send", operator, map[string]string{"to": "alpha", "body": "x", field: "alpha"})
		if status != 400 {
			t.Errorf("the field %s answered %d", field, status)
		}
	}
	for name, c := range map[string]struct {
		body   model.SendBody
		status int
	}{
		"an unknown recipient": {model.SendBody{To: "ghost", Body: "x"}, 404},
		"the recipient all":    {model.SendBody{To: "all", Body: "x"}, 404},
		"oneself":              {model.SendBody{To: "operator", Body: "x"}, 422},
		"an empty body":        {model.SendBody{To: "alpha", Body: " "}, 422},
		"a bad kind":           {model.SendBody{To: "alpha", Body: "x", Kind: "shout"}, 422},
		"a bad thread":         {model.SendBody{To: "alpha", Body: "x", Thread: "Bad"}, 422},
		"a bad in_reply_to":    {model.SendBody{To: "alpha", Body: "x", InReplyTo: "a b"}, 422},
		"a large body":         {model.SendBody{To: "alpha", Body: strings.Repeat("x", model.MaxBodyBytes+1)}, 413},
	} {
		if status, raw := h.call("POST", "/v1/operator/send", operator, c.body); status != c.status {
			t.Errorf("%s answered %d %s", name, status, raw)
		}
	}
	if len(h.logged()) != 0 {
		t.Fatal("a refused send wrote to the log")
	}
}

func TestAnOperatorSendToAnAgentThatIsNotConnectedIs409AndLoggedWithThatResult(t *testing.T) {
	t.Parallel()
	h := start(t)
	h.register("alpha")
	operator := h.registerOperator("tui")
	status, raw := h.call("POST", "/v1/operator/send", operator, model.SendBody{To: "alpha", Body: "x"})
	if status != 409 || !strings.Contains(string(raw), "not connected") {
		t.Fatalf("%d %s", status, raw)
	}
	if got := h.resultOf(1); !strings.HasPrefix(got, "not connected") {
		t.Fatalf("result %q", got)
	}
}

func TestAReplyToTheOperatorIs409NotConnectedAndLoggedAndNoAgentCanTakeTheName(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	handle, token := h.registerSeed("operator", nil)
	stream := h.openStream(token)
	if handle == "operator" || !strings.HasPrefix(handle, "operator") {
		t.Fatalf("handle %q", handle)
	}
	h.waitUntil("the agent to be connected", func() bool { a, ok := h.agent(alpha, handle); return ok && a.Connected })
	status, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: "operator", Body: "a reply", Kind: model.KindReply})
	var problem model.ErrorResponse
	json.Unmarshal(raw, &problem)
	if status != 409 || problem.Error != "not connected" {
		t.Fatalf("%d %s", status, raw)
	}
	logged := h.logged()
	if len(logged) != 1 || logged[0].To != "operator" || logged[0].From != "alpha" || logged[0].Result != "not connected" {
		t.Fatalf("log %+v", logged)
	}
	select {
	case line := <-stream.lines:
		if strings.HasPrefix(line, "event:") {
			t.Fatalf("the reply went to the agent %s: %q", handle, line)
		}
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAnOperatorIsKeptForExactlyTheGraceAndEveryRequestRefreshesIt(t *testing.T) {
	t.Parallel()
	h := start(t)
	operator := h.registerOperator("tui")
	h.advance(2 * time.Minute)
	if status, _ := h.call("GET", "/v1/agents", operator, nil); status != 200 {
		t.Fatalf("at the grace the token answered %d", status)
	}
	h.advance(2 * time.Minute)
	if status, _ := h.call("GET", "/v1/log", operator, nil); status != 403 {
		t.Fatalf("a refused request answered %d", status)
	}
	h.advance(2 * time.Minute)
	if status, _ := h.call("GET", "/v1/agents", operator, nil); status != 200 {
		t.Fatalf("a refused request did not refresh the operator: %d", status)
	}
	h.advance(2*time.Minute + time.Nanosecond)
	for _, path := range []string{"/v1/agents"} {
		if status, _ := h.call("GET", path, operator, nil); status != 401 {
			t.Fatalf("past the grace the token answered %d", status)
		}
	}
	if status, _ := h.call("POST", "/v1/operator/send", operator, model.SendBody{To: "alpha", Body: "x"}); status != 401 {
		t.Fatalf("a send past the grace answered %d", status)
	}
	if status, _ := h.call("GET", "/v1/agents", h.registerOperator("tui"), nil); status != 200 {
		t.Fatalf("a new operator answered %d", status)
	}
}

func TestTheNamesOperatorAndAllAreRefusedAsANameOrALabelAndAnAttachCannotTakeThem(t *testing.T) {
	t.Parallel()
	h := start(t)
	for _, name := range []string{"operator", "all"} {
		if status, _ := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: name, Harness: "codex"}); status != 400 {
			t.Errorf("the name %s answered %d", name, status)
		}
		if status, _ := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: "alpha", Harness: "codex", Label: name}); status != 400 {
			t.Errorf("the label %s answered %d", name, status)
		}
	}
	status, raw := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: "alpha", Harness: "codex", Label: "tutor"})
	var registered model.RegisterResponse
	json.Unmarshal(raw, &registered)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	for _, label := range []string{"operator", "all"} {
		if status, _ := h.call("POST", "/v1/attach", registered.Token, model.AttachRequest{Label: label}); status != 400 {
			t.Errorf("an attach with the label %s answered %d", label, status)
		}
	}
	if found, _ := h.agent(registered.Token, "alpha"); found.Label != "tutor" {
		t.Fatalf("the label became %q", found.Label)
	}
}

func TestTheTmuxAttributesAreRegisteredListedReplacedAndCheckedAndAnAttachCannotSetThem(t *testing.T) {
	t.Parallel()
	h := start(t)
	register := func(socket, pane string) (int, string) {
		status, raw := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: "alpha", Harness: "codex", TmuxSocket: socket, TmuxPane: pane})
		var out model.RegisterResponse
		json.Unmarshal(raw, &out)
		return status, out.Token
	}
	status, token := register("default", "%3")
	if status != 200 {
		t.Fatalf("register: %d", status)
	}
	if found, _ := h.agent(token, "alpha"); found.TmuxSocket != "default" || found.TmuxPane != "%3" {
		t.Fatalf("%+v", found)
	}
	if status, _ := register("a b", "%3"); status != 400 {
		t.Fatalf("a bad socket answered %d", status)
	}
	if status, _ := register("default", "%x"); status != 400 {
		t.Fatalf("a bad pane answered %d", status)
	}
	if found, _ := h.agent(token, "alpha"); found.TmuxSocket != "default" || found.TmuxPane != "%3" {
		t.Fatalf("a refused registration changed the entry: %+v", found)
	}
	if status, _ := h.call("POST", "/v1/attach", token, map[string]string{"tmux_socket": "x"}); status != 400 {
		t.Fatalf("an attach with a tmux field answered %d", status)
	}
	status, token = register("", "")
	if found, _ := h.agent(token, "alpha"); status != 200 || found.TmuxSocket != "" || found.TmuxPane != "" {
		t.Fatalf("%d %+v", status, found)
	}
}
