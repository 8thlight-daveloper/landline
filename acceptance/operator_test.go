package acceptance

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const operatorStamp = "from: operator (operator on mac)"

var sentLine = regexp.MustCompile(`^sent m-\S+ \(seq \d+\) to (\S+)$`)

func (w *world) registerOperator(kind string) string {
	w.t.Helper()
	status, body := w.post("/v1/operators", secret, map[string]string{"kind": kind})
	if status != http.StatusOK {
		w.t.Fatalf("POST /v1/operators: %d %s", status, body)
	}
	var issued struct{ Token string }
	if err := json.Unmarshal([]byte(body), &issued); err != nil || issued.Token == "" {
		w.t.Fatalf("operator answer %q: %v", body, err)
	}
	return issued.Token
}

func (w *world) operatorSend(token string, body any) (int, string) {
	w.t.Helper()
	return w.post("/v1/operator/send", token, body)
}

func withoutEnv(env []string, names ...string) []string {
	var out []string
	for _, pair := range env {
		keep := true
		for _, name := range names {
			if strings.HasPrefix(pair, name+"=") {
				keep = false
			}
		}
		if keep {
			out = append(out, pair)
		}
	}
	return out
}

func (w *world) logFile() string {
	w.t.Helper()
	raw, err := os.ReadFile(filepath.Join(w.config, "log.jsonl"))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(raw)
}

func TestAnOperatorIsRegisteredWithTheSecretAndAKindThatFollowsTheFreeTextRules(t *testing.T) {
	w := start(t)
	w.register("alpha")
	status, body := w.post("/v1/operators", secret, map[string]string{"kind": "tui"})
	var issued struct{ Token string }
	if status != 200 || json.Unmarshal([]byte(body), &issued) != nil || issued.Token == "" {
		t.Fatalf("a valid registration: %d %s", status, body)
	}
	observer := w.registerObserver("tui")
	if issued.Token == secret || issued.Token == w.token("alpha") || issued.Token == observer {
		t.Fatal("the operator token repeats another credential")
	}
	if status, _ := w.post("/v1/operators", secret, map[string]string{"kind": "cli"}); status != 200 {
		t.Errorf("kind cli: %d", status)
	}
	if status, _ := w.post("/v1/operators", secret, map[string]string{"kind": strings.Repeat("k", 64)}); status != 200 {
		t.Errorf("a 64 byte kind: %d", status)
	}
	for name, token := range map[string]string{"no token": "", "a wrong secret": "wrong-secret-wrong-secret", "an agent token": w.token("alpha")} {
		if status, _ := w.post("/v1/operators", token, map[string]string{"kind": "tui"}); status != 401 {
			t.Errorf("POST /v1/operators with %s: %d, want 401", name, status)
		}
	}
	for name, token := range map[string]string{"an observer token": observer, "an operator token": issued.Token} {
		if status, _ := w.post("/v1/operators", token, map[string]string{"kind": "tui"}); status != 403 {
			t.Errorf("POST /v1/operators with %s: %d, want 403", name, status)
		}
	}
	for name, kind := range map[string]string{
		"blank":     "   ",
		"empty":     "",
		"65 bytes":  strings.Repeat("k", 65),
		"newline":   "tu\ni",
		"escape":    "tu\x1bi",
		"nul":       "tu\x00i",
		"separator": "tu\u2028i",
		"format":    "tu\u202ei",
	} {
		if status, body := w.post("/v1/operators", secret, map[string]string{"kind": kind}); status != 400 {
			t.Errorf("%s kind: %d %s", name, status, body)
		}
	}
	if status, _ := w.post("/v1/operators", secret, map[string]string{}); status != 400 {
		t.Errorf("a missing kind: %d", status)
	}
	if status, _ := w.post("/v1/operators", secret, map[string]string{"kind": "tui", "name": "operator"}); status != 400 {
		t.Errorf("an unknown field: %d", status)
	}
}

func TestAnOperatorIsNeverAnAgentAndSeesTheSameListAnAgentSees(t *testing.T) {
	w := start(t)
	w.watch()
	w.register("alpha")
	_, handle := w.selfRegistered("lead", "claude-code")
	before := strings.Count(w.must("", "agents"), "\n")
	operator := w.registerOperator("tui")
	w.registerOperator("cli")
	listed := w.agentsBody(operator)
	var entries []map[string]any
	if err := json.Unmarshal([]byte(listed), &entries); err != nil || len(entries) != before {
		t.Fatalf("the operator's list has %d entries (%v), the agents are %d", len(entries), err, before)
	}
	for _, entry := range entries {
		if entry["name"] == "operator" || entry["name"] == "tui" || entry["name"] == "cli" || entry["name"] == operator {
			t.Fatalf("an operator is listed: %v", entry)
		}
	}
	if text := w.must("", "agents"); strings.Contains(text, "operator") || strings.Contains(text, operator) {
		t.Fatalf("the CLI list shows the operator:\n%s", text)
	}
	byAgent, byOperator, byObserver := w.agentsBody(w.token(handle)), w.agentsBody(operator), w.agentsBody(w.registerObserver("tui"))
	if !bytes.Equal(withoutLastSeen(t, byOperator), withoutLastSeen(t, byAgent)) || !bytes.Equal(withoutLastSeen(t, byOperator), withoutLastSeen(t, byObserver)) {
		t.Fatalf("the lists differ:\n%s\n%s\n%s", byOperator, byAgent, byObserver)
	}
	if status, _ := w.sendHTTP("alpha", "tui", "to an operator kind"); status != 404 {
		t.Errorf("a send to the operator's kind: %d", status)
	}
}

func TestAnOperatorTokenReadsTheAgentListAndEveryOtherRouteAnswers403WithNothingSentOrLogged(t *testing.T) {
	w := start(t)
	w.watch()
	_, handle := w.selfRegistered("lead", "claude-code")
	w.register("alpha")
	stream := w.openStream("watcher", true)
	logBefore := w.logText()
	operator := w.registerOperator("tui")
	if status, _ := w.request("GET", "/v1/agents", operator, nil, nil); status != 200 {
		t.Fatalf("GET /v1/agents: %d", status)
	}
	requests := []struct {
		method, path string
		body         any
	}{
		{"POST", "/v1/send", map[string]string{"to": "watcher", "body": "from an operator by the agent route"}},
		{"GET", "/v1/stream", nil},
		{"POST", "/v1/attach", map[string]string{"label": "mine"}},
		{"POST", "/v1/deliveries/some-id/report", map[string]any{"ok": true}},
		{"POST", "/v1/register", map[string]string{"name": "mallory", "harness": "x", "host": "mac"}},
		{"POST", "/v1/observers", map[string]string{"kind": "tui"}},
		{"POST", "/v1/operators", map[string]string{"kind": "tui"}},
		{"GET", "/v1/log", nil},
	}
	for _, r := range requests {
		status, body := w.request(r.method, r.path, operator, r.body, nil)
		var refusal struct{ Error string }
		if status != 403 || json.Unmarshal([]byte(body), &refusal) != nil || refusal.Error == "" {
			t.Errorf("%s %s: %d %s", r.method, r.path, status, body)
		}
	}
	if status, _ := w.post("/v1/operator/send", "unknown-token-unknown-token", map[string]string{"to": "watcher", "body": "x"}); status != 401 {
		t.Errorf("an unknown token on the operator route: %d", status)
	}
	if status, _ := w.post("/v1/operator/send", "", map[string]string{"to": "watcher", "body": "x"}); status != 401 {
		t.Errorf("no token on the operator route: %d", status)
	}
	stream.expectNothing(t)
	if got := w.logText(); got != logBefore {
		t.Fatalf("the log changed:\n%s\n--\n%s", logBefore, got)
	}
	if w.state("mallory") != "" {
		t.Fatal("an operator registered an agent")
	}
	if got := w.state(handle); got != "connected" {
		t.Fatalf("the agent is %q", got)
	}
}

func TestOnlyAnOperatorTokenMaySendOnTheOperatorRouteAndNothingIsDeliveredOrLoggedForTheOthers(t *testing.T) {
	w := start(t)
	w.register("alpha", "bravo")
	stream := w.openStream("bravo", true)
	logBefore := w.logText()
	observer := w.registerObserver("tui")
	request := map[string]string{"to": "bravo", "body": "not from an operator"}
	status, body := w.operatorSend(observer, request)
	var refusal struct{ Error string }
	if status != 403 || json.Unmarshal([]byte(body), &refusal) != nil || refusal.Error == "" {
		t.Errorf("an observer token: %d %s", status, body)
	}
	for name, token := range map[string]string{"an agent token": w.token("alpha"), "the secret": secret} {
		if status, body := w.operatorSend(token, request); status != 401 && status != 403 {
			t.Errorf("%s: %d %s, want a refusal", name, status, body)
		}
	}
	stream.expectNothing(t)
	if got := w.logText(); got != logBefore {
		t.Fatalf("the log changed:\n%s\n--\n%s", logBefore, got)
	}
	operator := w.registerOperator("tui")
	if status, body := w.operatorSend(operator, request); status != 200 {
		t.Fatalf("an operator token: %d %s", status, body)
	}
	if got := stream.next(t); got.Message == nil || got.Message.Body != "not from an operator" {
		t.Fatalf("delivery %+v", got)
	}
}

func TestAMessageFromAnOperatorReachesAConnectedChannelAgentOnceWithTheMarkerFirstAndTheOperatorStamp(t *testing.T) {
	w := start(t)
	bravo := w.channelAgent("bravo")
	operator := w.registerOperator("tui")
	status, body := w.operatorSend(operator, map[string]string{"to": "bravo", "body": "please look at the failing build", "thread": "deploy", "kind": "task", "in_reply_to": "m-0"})
	var sent struct {
		ID         string
		Seq        int
		Recipients []string
	}
	if status != 200 || json.Unmarshal([]byte(body), &sent) != nil || !strings.HasPrefix(sent.ID, "m-") || sent.Seq != 1 || len(sent.Recipients) != 1 || sent.Recipients[0] != "bravo" {
		t.Fatalf("send: %d %s", status, body)
	}
	note := bravo.notification()
	content := note.Params.Content
	if !strings.HasPrefix(content, marker+"\n") {
		t.Fatalf("the marker is not first: %q", content)
	}
	for _, want := range []string{operatorStamp, " label: tui", "thread: deploy", "kind: task", "id: " + sent.ID, "in-reply-to: m-0", "\n\nplease look at the failing build"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in %q", want, content)
		}
	}
	if note.Params.Meta["from"] != "operator" || note.Params.Meta["thread"] != "deploy" || note.Params.Meta["kind"] != "task" {
		t.Fatalf("meta %+v", note.Params.Meta)
	}
	bravo.expectNoNotification()
	for _, entry := range w.agentsJSON() {
		if entry["name"] == "operator" {
			t.Fatalf("the operator is listed: %v", entry)
		}
	}
}

func TestTheStampOfAnOperatorMessageNamesTheSwitchboardsHostAndTheKindOfTheOperatorAndNeverAnEmptyHost(t *testing.T) {
	w := startHub(t, build(t), t.TempDir(), "", "--host", "brig")
	w.register("bravo")
	stream := w.openStream("bravo", true)
	for _, kind := range []string{"tui", "cli"} {
		operator := w.registerOperator(kind)
		if status, body := w.operatorSend(operator, map[string]string{"to": "bravo", "body": "from " + kind}); status != 200 {
			t.Fatalf("%s: %d %s", kind, status, body)
		}
		got := stream.next(t)
		if got.Message == nil || got.Message.From != "operator" {
			t.Fatalf("%s: delivery %+v", kind, got)
		}
		if !strings.HasPrefix(got.Text, marker+"\n") || !strings.Contains(got.Text, "from: operator (operator on brig)") || !strings.Contains(got.Text, " label: "+kind) {
			t.Errorf("%s: text %q", kind, got.Text)
		}
		if got.Message.Text != got.Text {
			t.Errorf("%s: the envelope text differs from the delivery text", kind)
		}
		if strings.Contains(got.Text, "(operator on )") || strings.Contains(got.Text, "(operator on  ") {
			t.Errorf("%s: an empty host in %q", kind, got.Text)
		}
	}
}

func TestAnOperatorMessageIsLoggedWithTheSenderOperatorAndItsDeliveryResult(t *testing.T) {
	w := start(t)
	w.register("bravo")
	w.openStream("bravo", true)
	operator := w.registerOperator("tui")
	if status, body := w.operatorSend(operator, map[string]string{"to": "bravo", "body": "log me", "thread": "deploy", "kind": "review", "in_reply_to": "m-9"}); status != 200 {
		t.Fatalf("send: %d %s", status, body)
	}
	log := w.logText()
	if logEntries(log) != 1 || !strings.Contains(log, "operator -> bravo") || !strings.Contains(log, "\tdeploy\treview") || !strings.Contains(log, "    log me\n") {
		t.Fatalf("log: %q", log)
	}
	if got := logResults(log); got["log me"] != "delivered" {
		t.Fatalf("results: %v", got)
	}
	file := w.logFile()
	for _, want := range []string{`"from":"operator"`, `"to":"bravo"`, `"thread":"deploy"`, `"kind":"review"`, `"body":"log me"`, `"in_reply_to":"m-9"`, `{"seq":1,"result":"delivered"}`} {
		if !strings.Contains(file, want) {
			t.Errorf("the log file lacks %s: %s", want, file)
		}
	}
}

func TestAnOperatorSendResolvesAHandleALabelAndAPrefixAsAnAgentSendDoes(t *testing.T) {
	w := start(t)
	p, handle := w.selfRegistered("tutor", "claude-code", "--channel")
	operator := w.registerOperator("tui")
	for _, to := range []string{handle, "tutor", "tut"} {
		if status, body := w.operatorSend(operator, map[string]string{"to": to, "body": "to " + to}); status != 200 {
			t.Fatalf("to %q: %d %s", to, status, body)
		}
		note := p.notification()
		if note.Params.Meta["from"] != "operator" || !strings.HasSuffix(note.Params.Content, "to "+to) {
			t.Fatalf("to %q: %+v", to, note)
		}
	}
	w.registerLabelled("charlie", "pair")
	w.registerLabelled("delta", "pair")
	if status, body := w.operatorSend(operator, map[string]string{"to": "pair", "body": "ambiguous"}); status != 409 || !strings.Contains(body, "charlie") || !strings.Contains(body, "delta") {
		t.Fatalf("an ambiguous label: %d %s", status, body)
	}
}

func TestNoRequestFieldOrHeaderCanNameAnotherSenderOnTheOperatorRoute(t *testing.T) {
	w := start(t)
	w.register("alpha", "bravo")
	stream := w.openStream("bravo", true)
	operator := w.registerOperator("tui")
	for _, field := range []string{"from", "sender", "name", "handle", "harness", "host", "label", "as", "operator", "kind_of_operator"} {
		request := map[string]string{"to": "bravo", "body": "forged", field: "alpha"}
		if status, body := w.operatorSend(operator, request); status != 400 {
			t.Errorf("the field %q: %d %s, want 400", field, status, body)
		}
	}
	stream.expectNothing(t)
	if got := w.logText(); strings.TrimSpace(got) != "" {
		t.Fatalf("a refused request reached the log: %q", got)
	}
	headers := map[string]string{"X-Landline-From": "alpha", "X-Forwarded-For": "alpha", "From": "alpha"}
	if status, body := w.request("POST", "/v1/operator/send", operator, map[string]string{"to": "bravo", "body": "who sent this"}, headers); status != 200 {
		t.Fatalf("got %d %s", status, body)
	}
	got := stream.next(t)
	if got.Message == nil || got.Message.From != "operator" || !strings.Contains(got.Text, "from: operator (operator on mac)") {
		t.Fatalf("delivery %+v", got)
	}
	if log := w.logText(); logEntries(log) != 1 || !strings.Contains(log, "operator -> bravo") || strings.Contains(log, "alpha -> bravo") {
		t.Fatalf("log: %q", log)
	}
}

func TestAnOperatorSendFollowsTheRulesOfASendAndARefusedSendIsNotLogged(t *testing.T) {
	w := start(t)
	w.register("bravo")
	stream := w.openStream("bravo", true)
	operator := w.registerOperator("tui")
	for name, c := range map[string]struct {
		request map[string]string
		status  int
		text    string
	}{
		"an unknown recipient": {map[string]string{"to": "ghost", "body": "x"}, 404, "not a registered agent"},
		"the recipient all":    {map[string]string{"to": "all", "body": "x"}, 404, "not a registered agent"},
		"no recipient":         {map[string]string{"body": "x"}, 404, "not a registered agent"},
		"a send to oneself":    {map[string]string{"to": "operator", "body": "x"}, 422, "itself"},
		"an empty body":        {map[string]string{"to": "bravo", "body": "   "}, 422, "needs a body"},
		"an oversized body":    {map[string]string{"to": "bravo", "body": strings.Repeat("x", 70*1024)}, 413, "larger than 64 KiB"},
		"a bad thread":         {map[string]string{"to": "bravo", "body": "x", "thread": "Bad Thread"}, 422, "thread"},
		"an unknown kind":      {map[string]string{"to": "bravo", "body": "x", "kind": "shout"}, 422, "kind"},
		"a bad in_reply_to":    {map[string]string{"to": "bravo", "body": "x", "in_reply_to": "a b"}, 422, "in_reply_to"},
	} {
		status, body := w.operatorSend(operator, c.request)
		if status != c.status || !strings.Contains(body, c.text) {
			t.Errorf("%s: %d %s, want %d with %q", name, status, body, c.status, c.text)
		}
	}
	stream.expectNothing(t)
	if got := w.logText(); strings.TrimSpace(got) != "" {
		t.Fatalf("a refused message reached the log: %q", got)
	}
	if status, body := w.post("/v1/operator/send", operator, "not an object"); status != 400 {
		t.Errorf("a body that is not an object: %d %s", status, body)
	}
}

func TestAnOperatorSendToAnAgentThatIsNotConnectedAnswers409NotConnectedAndTheMessageIsLoggedWithThatResult(t *testing.T) {
	w := start(t)
	w.register("bravo")
	operator := w.registerOperator("tui")
	status, body := w.operatorSend(operator, map[string]string{"to": "bravo", "body": "nobody is listening"})
	if status != 409 || !strings.Contains(body, notConnected) {
		t.Fatalf("got %d %s", status, body)
	}
	log := w.logText()
	if logEntries(log) != 1 || !strings.Contains(log, "operator -> bravo") || !strings.HasPrefix(logResults(log)["nobody is listening"], notConnected) {
		t.Fatalf("log: %q", log)
	}
}

func TestAnOperatorSendWhoseDeliveryFailsAnswersWhatAnAgentSendWouldAnswer(t *testing.T) {
	w := startHub(t, build(t), t.TempDir(), "", "--delivery-timeout", "500ms")
	w.register("bravo", "charlie")
	failing := w.openStream("bravo", false)
	operator := w.registerOperator("tui")
	if status, body := w.operatorSend(operator, map[string]string{"to": "bravo", "body": "never reported"}); status != 504 || !strings.Contains(body, "no delivery report") {
		t.Fatalf("no report: %d %s", status, body)
	}
	failing.next(t)
	reporting := w.openStream("charlie", false)
	done := make(chan [2]any, 1)
	go func() {
		status, body := w.operatorSend(operator, map[string]string{"to": "charlie", "body": "refused by the session"})
		done <- [2]any{status, body}
	}()
	delivery := reporting.next(t)
	w.postQuietly("/v1/deliveries/"+delivery.ID+"/report", w.token("charlie"), map[string]any{"ok": false, "error": "the session said no"})
	select {
	case got := <-done:
		if got[0] != 502 || !strings.Contains(got[1].(string), "the session said no") {
			t.Fatalf("a failed report: %v", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the send did not answer")
	}
	results := logResults(w.logText())
	if results["never reported"] != "no delivery report" || results["refused by the session"] != "the session said no" {
		t.Fatalf("results: %v", results)
	}
}

func TestAnAgentReplyToTheOperatorIsToldNotConnectedAndTheReplyIsInTheLog(t *testing.T) {
	w := start(t)
	w.register("alpha")
	bravo := w.channelAgent("bravo")
	operator := w.registerOperator("tui")
	_, body := w.operatorSend(operator, map[string]string{"to": "bravo", "body": "how is it going"})
	var sent struct{ ID string }
	json.Unmarshal([]byte(body), &sent)
	note := bravo.notification()
	if note.Params.Meta["from"] != "operator" {
		t.Fatalf("meta %+v", note.Params.Meta)
	}
	answer := bravo.tool("landline_send", `{"to":"operator","body":"going well","kind":"reply","in_reply_to":"`+sent.ID+`"}`)
	if !strings.Contains(answer, notConnected) {
		t.Fatalf("the tool answered %q", answer)
	}
	status, raw := w.sendHTTP("alpha", "operator", "a reply from alpha")
	if status != 409 || !strings.Contains(raw, notConnected) {
		t.Fatalf("HTTP: %d %s", status, raw)
	}
	out, err := w.run("a reply from the command line", "send", "--as", "alpha", "--to", "operator", "-")
	if err == nil || !strings.Contains(out, notConnected) {
		t.Fatalf("CLI: %q, %v", out, err)
	}
	bravo.expectNoNotification()
	log := w.logText()
	if logEntries(log) != 4 || !strings.Contains(log, "bravo -> operator") || !strings.Contains(log, "alpha -> operator") {
		t.Fatalf("log: %q", log)
	}
	results := logResults(log)
	for _, text := range []string{"going well", "a reply from alpha", "a reply from the command line"} {
		if !strings.HasPrefix(results[text], notConnected) {
			t.Errorf("the result of %q is %q", text, results[text])
		}
	}
	if !strings.Contains(w.logFile(), `"in_reply_to":"`+sent.ID+`"`) {
		t.Errorf("the reply does not carry in_reply_to: %s", w.logFile())
	}
	if status := w.get("/v1/stream", operator); status != 403 {
		t.Errorf("an operator opened a stream: %d", status)
	}
}

func TestTheNameOperatorGoesToTheOperatorBeforeAnyAgentWhoseHandleStartsWithIt(t *testing.T) {
	w := start(t)
	w.register("alpha")
	status, body := w.post("/v1/register", secret, map[string]string{"seed": "operator", "harness": "claude-code", "host": "mac"})
	var issued struct{ Name, Token string }
	if status != 200 || json.Unmarshal([]byte(body), &issued) != nil || issued.Name == "operator" || !strings.HasPrefix(issued.Name, "operator") {
		t.Fatalf("a seed of operator: %d %s", status, body)
	}
	stream := w.openStreamWith(issued.Token, true)
	w.waitState(issued.Name, "connected")
	if status, body := w.sendHTTP("alpha", "operator", "for the operator and not the agent"); status != 409 || !strings.Contains(body, notConnected) {
		t.Fatalf("got %d %s", status, body)
	}
	stream.expectNothing(t)
	if got := logResults(w.logText()); !strings.HasPrefix(got["for the operator and not the agent"], notConnected) {
		t.Fatalf("results: %v", got)
	}
	operator := w.registerOperator("tui")
	if status, body := w.operatorSend(operator, map[string]string{"to": issued.Name, "body": "to the agent by its handle"}); status != 200 {
		t.Fatalf("to the handle: %d %s", status, body)
	}
	if got := stream.next(t); got.Message == nil || got.Message.Body != "to the agent by its handle" {
		t.Fatalf("delivery %+v", got)
	}
}

func TestRegisteringAnAgentNamedOrLabelledOperatorOrAllIsRefusedAndNoSessionIsWritten(t *testing.T) {
	w := start(t)
	for _, name := range []string{"operator", "all"} {
		if status, body := w.post("/v1/register", secret, map[string]string{"name": name, "harness": "codex", "host": "mac"}); status != 400 {
			t.Errorf("the name %s: %d %s", name, status, body)
		}
		if status, body := w.post("/v1/register", secret, map[string]string{"name": "alpha", "harness": "codex", "host": "mac", "label": name}); status != 400 {
			t.Errorf("the label %s: %d %s", name, status, body)
		}
		if status, body := w.post("/v1/register", secret, map[string]string{"seed": "alpha", "harness": "codex", "host": "mac", "label": name}); status != 400 {
			t.Errorf("the label %s with a seed: %d %s", name, status, body)
		}
		if out, err := w.run("", "register", "--name", name, "--harness", "codex"); err == nil {
			t.Errorf("landline register --name %s: %q", name, out)
		}
	}
	if status, body := w.post("/v1/register", secret, map[string]string{"name": "alpha", "harness": "codex", "host": "mac", "label": "operators"}); status != 200 {
		t.Errorf("the label operators: %d %s", status, body)
	}
	if entries, _ := os.ReadDir(filepath.Join(w.config, ".config", "landline")); len(entries) != 0 {
		t.Fatalf("session files were written: %v", entries)
	}
	for _, entry := range w.agentsJSON() {
		if entry["name"] == "operator" || entry["name"] == "all" || entry["label"] == "operator" || entry["label"] == "all" {
			t.Fatalf("a reserved name is listed: %v", entry)
		}
	}
}

func TestTheLabelUpdateOfAnAttachedAgentCannotTakeOperatorOrAllAndKeepsTheOldLabel(t *testing.T) {
	w := start(t)
	token := w.registerAgentTokenWithLabel("alpha", "tutor")
	for _, label := range []string{"operator", "all"} {
		if status, body := w.post("/v1/attach", token, map[string]string{"label": label}); status != 400 {
			t.Errorf("the label %s: %d %s", label, status, body)
		}
	}
	if got := w.agentJSON("alpha")["label"]; got != "tutor" {
		t.Fatalf("the label became %v", got)
	}
	if status, body := w.post("/v1/attach", token, map[string]string{"label": "reviewer"}); status != 200 {
		t.Fatalf("a normal update: %d %s", status, body)
	}
	if got := w.agentJSON("alpha")["label"]; got != "reviewer" {
		t.Fatalf("the label is %v", got)
	}
}

func (w *world) registerAgentToken(name string) string {
	w.t.Helper()
	return w.registerAgentTokenWithLabel(name, "")
}

func (w *world) registerAgentTokenWithLabel(name, label string) string {
	w.t.Helper()
	fields := map[string]string{"name": name, "harness": "claude-code", "host": "mac"}
	if label != "" {
		fields["label"] = label
	}
	status, body := w.post("/v1/register", secret, fields)
	var issued struct{ Token string }
	if status != 200 || json.Unmarshal([]byte(body), &issued) != nil || issued.Token == "" {
		w.t.Fatalf("register %s: %d %s", name, status, body)
	}
	return issued.Token
}

func TestAnOperatorIsForgottenAfterTheGraceWithoutARequestAndRequestsInsideTheGraceKeepItAlive(t *testing.T) {
	w := startHub(t, build(t), t.TempDir(), "", "--grace", "3s")
	w.register("bravo")
	stream := w.openStream("bravo", true)
	idle := w.registerOperator("tui")
	busy := w.registerOperator("tui")
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second)
		if i%2 == 0 {
			if status := w.get("/v1/agents", busy); status != 200 {
				t.Fatalf("request %d inside the grace: %d", i, status)
			}
		} else if status, body := w.operatorSend(busy, map[string]string{"to": "bravo", "body": "keep alive"}); status != 200 {
			t.Fatalf("send %d inside the grace: %d %s", i, status, body)
		} else {
			stream.next(t)
		}
	}
	if status := w.get("/v1/agents", idle); status != 401 {
		t.Fatalf("an idle operator after the grace: %d", status)
	}
	if status, _ := w.operatorSend(idle, map[string]string{"to": "bravo", "body": "too late"}); status != 401 {
		t.Fatalf("a send with an expired token: %d", status)
	}
	stream.expectNothing(t)
	again := w.registerOperator("tui")
	if status, body := w.operatorSend(again, map[string]string{"to": "bravo", "body": "after registering again"}); status != 200 {
		t.Fatalf("a new registration: %d %s", status, body)
	}
	if got := stream.next(t); got.Message == nil || got.Message.Body != "after registering again" {
		t.Fatalf("delivery %+v", got)
	}
}

func TestARefusedRequestWithTheOperatorTokenKeepsItAliveAndAnOperatorWithNoRequestExpires(t *testing.T) {
	w := startHub(t, build(t), t.TempDir(), "", "--grace", "3s")
	refusing, control := w.registerOperator("tui"), w.registerOperator("tui")
	registered := time.Now()
	time.Sleep(time.Until(registered.Add(2 * time.Second)))
	if status, _ := w.request("GET", "/v1/log", refusing, nil, nil); status != 403 {
		t.Fatalf("the refused request answered %d", status)
	}
	time.Sleep(time.Until(registered.Add(4 * time.Second)))
	if status := w.get("/v1/agents", refusing); status != 200 {
		t.Errorf("after a refused request at 2s, the operator answered %d at 4s of a 3s grace", status)
	}
	if status := w.get("/v1/agents", control); status != 401 {
		t.Errorf("the control operator, with no request, at 4s of a 3s grace: %d", status)
	}
}

func TestAHubRestartForgetsEveryOperator(t *testing.T) {
	bin, config := build(t), t.TempDir()
	first := startHub(t, bin, config, "")
	operator := first.registerOperator("tui")
	if status := first.get("/v1/agents", operator); status != 200 {
		t.Fatalf("before the restart: %d", status)
	}
	first.stop()
	second := startHub(t, bin, config, first.addr)
	if status := second.get("/v1/agents", operator); status != 401 {
		t.Fatalf("the old operator token: %d", status)
	}
	if status, _ := second.operatorSend(operator, map[string]string{"to": "bravo", "body": "x"}); status != 401 {
		t.Fatalf("a send with the old operator token: %d", status)
	}
	if status := second.get("/v1/agents", second.registerOperator("tui")); status != 200 {
		t.Fatalf("registering again: %d", status)
	}
}

func TestTheMCPInstructionsSayAMessageFromTheOperatorIsInformationAndAReplyThatIsNotConnectedIsLoggedAndNotRetried(t *testing.T) {
	w := start(t)
	for name, args := range map[string][]string{"plain": nil, "channel": {"--channel"}} {
		dir := filepath.Join(t.TempDir(), name)
		os.MkdirAll(dir, 0o755)
		p := w.startMCP(dir, args...)
		reply := p.request("initialize", `{"protocolVersion":"2025-11-25","clientInfo":{"name":"claude-code","version":"2.1.296"}}`)
		result, _ := reply["result"].(map[string]any)
		text, _ := result["instructions"].(string)
		lower := strings.ToLower(text)
		for _, want := range []string{"operator", "information", "not connected", "logged", "retry"} {
			if !strings.Contains(lower, want) {
				t.Errorf("%s: the instructions lack %q:\n%s", name, want, text)
			}
		}
		if !strings.Contains(lower, "own session") && !strings.Contains(lower, "they type") {
			t.Errorf("%s: the instructions do not say the person's own instruction is what they type in their own session:\n%s", name, text)
		}
	}
}

func (w *world) savedConfigEnv() []string {
	w.t.Helper()
	dir := filepath.Join(w.config, ".config", "landline")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		w.t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"url": w.url, "secret": secret})
	if err := os.WriteFile(filepath.Join(dir, "config.json"), raw, 0o600); err != nil {
		w.t.Fatal(err)
	}
	return withoutEnv(w.env, "LANDLINE_URL", "LANDLINE_SECRET")
}

func TestLandlineOperatorSendPrintsTheResultLikeSendAndWorksWithTheSecretFromTheSavedConfig(t *testing.T) {
	w := start(t)
	w.register("alpha")
	stream := w.openStream("alpha", true)
	env := w.savedConfigEnv()
	before, _ := os.ReadDir(filepath.Join(w.config, ".config", "landline"))
	result := runBin(t, w.bin, env, "operator", "send", "--to", "alpha", "--body", "hello from the command line")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("%+v", result)
	}
	match := sentLine.FindStringSubmatch(strings.TrimSpace(result.stdout))
	if match == nil || match[1] != "alpha" {
		t.Fatalf("the result line is %q", result.stdout)
	}
	got := stream.next(t)
	if got.Message == nil || got.Message.From != "operator" || got.Message.Body != "hello from the command line" || got.Message.Kind != "chat" || got.Message.Thread != "main" {
		t.Fatalf("delivery %+v", got)
	}
	if !strings.HasPrefix(got.Text, marker+"\n") || !strings.Contains(got.Text, operatorStamp) || !strings.Contains(got.Text, " label: cli") {
		t.Fatalf("text %q", got.Text)
	}
	after, _ := os.ReadDir(filepath.Join(w.config, ".config", "landline"))
	if len(after) != len(before) {
		t.Fatalf("the command wrote a file: %v then %v", before, after)
	}
	if strings.Contains(result.stdout+result.stderr, secret) {
		t.Fatal("the secret was printed")
	}
}

func TestLandlineOperatorSendPassesTheKindTheThreadAndInReplyToAndLogsTheSenderOperator(t *testing.T) {
	w := start(t)
	bravo := w.channelAgent("bravo")
	result := runBin(t, w.bin, w.env, "operator", "send", "--to", "bravo", "--body", "review this", "--kind", "review", "--thread", "deploy", "--in-reply-to", "m-7")
	if result.code != 0 || sentLine.FindStringSubmatch(strings.TrimSpace(result.stdout)) == nil {
		t.Fatalf("%+v", result)
	}
	note := bravo.notification()
	if note.Params.Meta["from"] != "operator" || note.Params.Meta["kind"] != "review" || note.Params.Meta["thread"] != "deploy" ||
		!strings.Contains(note.Params.Content, "in-reply-to: m-7") || !strings.HasSuffix(note.Params.Content, "review this") {
		t.Fatalf("note %+v", note)
	}
	if log := w.logText(); !strings.Contains(log, "operator -> bravo") || !strings.Contains(log, "\tdeploy\treview") {
		t.Fatalf("log: %q", log)
	}
}

func TestLandlineOperatorSendExitsWithTheCodesOfSendAndSaysWhy(t *testing.T) {
	w := start(t)
	w.register("alpha", "bravo")
	for name, c := range map[string]struct {
		args []string
		want string
	}{
		"an unknown recipient":              {[]string{"--to", "ghost", "--body", "x"}, "not a registered agent"},
		"the recipient all":                 {[]string{"--to", "all", "--body", "x"}, "not a registered agent"},
		"a recipient that is not connected": {[]string{"--to", "bravo", "--body", "x"}, notConnected},
		"a send to oneself":                 {[]string{"--to", "operator", "--body", "x"}, "itself"},
		"an empty body":                     {[]string{"--to", "bravo", "--body", "  "}, "needs a body"},
		"no body":                           {[]string{"--to", "bravo"}, "needs a body"},
		"an unknown kind":                   {[]string{"--to", "bravo", "--body", "x", "--kind", "shout"}, "kind"},
		"a bad thread":                      {[]string{"--to", "bravo", "--body", "x", "--thread", "Bad Thread"}, "thread"},
	} {
		result := runBin(t, w.bin, w.env, append([]string{"operator", "send"}, c.args...)...)
		failed := runBin(t, w.bin, w.env, "send", "--as", "alpha", "--to", "ghost", "x")
		if result.code != failed.code || result.code == 0 {
			t.Errorf("%s: exit %d, and a failed landline send exits %d", name, result.code, failed.code)
		}
		if result.stdout != "" || !strings.Contains(result.stderr, c.want) {
			t.Errorf("%s: stdout %q stderr %q, want %q on stderr", name, result.stdout, result.stderr, c.want)
		}
	}
	if got := w.logText(); logEntries(got) != 1 || !strings.Contains(got, "operator -> bravo") {
		t.Fatalf("only the send to the agent that is not connected is logged: %q", got)
	}
	flagFailure := runBin(t, w.bin, w.env, "send", "--bogus")
	if result := runBin(t, w.bin, w.env, "operator", "send", "--bogus"); result.code != flagFailure.code || result.code == 0 {
		t.Errorf("an unknown flag: exit %d, and landline send exits %d: %+v", result.code, flagFailure.code, result)
	}
}

func TestLandlineOperatorSendNeedsTheSecretAndAWrongOneIsRefusedWithoutBeingPrinted(t *testing.T) {
	w := start(t)
	w.register("alpha")
	w.openStream("alpha", true)
	missing := runBin(t, w.bin, withoutEnv(w.env, "LANDLINE_URL", "LANDLINE_SECRET"), "operator", "send", "--to", "alpha", "--body", "x")
	if missing.code != 1 || missing.stdout != "" || !strings.Contains(missing.stderr, "LANDLINE_SECRET") {
		t.Errorf("no secret: %+v", missing)
	}
	wrong := runBin(t, w.bin, withEnv(w.env, "LANDLINE_SECRET=wrong-secret-wrong-secret"), "operator", "send", "--to", "alpha", "--body", "x")
	if wrong.code != 1 || wrong.stdout != "" || strings.TrimSpace(wrong.stderr) == "" || strings.Contains(wrong.stderr, "wrong-secret-wrong-secret") {
		t.Errorf("a wrong secret: %+v", wrong)
	}
	agentToken := runBin(t, w.bin, withEnv(w.env, "LANDLINE_SECRET="+w.token("alpha")), "operator", "send", "--to", "alpha", "--body", "x")
	if agentToken.code != 1 || agentToken.stdout != "" {
		t.Errorf("an agent token as the secret: %+v", agentToken)
	}
	if got := w.logText(); strings.TrimSpace(got) != "" {
		t.Fatalf("a refused command reached the log: %q", got)
	}
}

func TestLandlineOperatorWithNoSubcommandOrAnUnknownOneExitsNonZeroAndSendsNothing(t *testing.T) {
	w := start(t)
	w.register("alpha")
	w.openStream("alpha", true)
	for _, args := range [][]string{{"operator"}, {"operator", "frobnicate"}} {
		result := runBin(t, w.bin, w.env, args...)
		if result.code == 0 || result.stdout != "" || strings.TrimSpace(result.stderr) == "" {
			t.Errorf("%v: %+v", args, result)
		}
	}
	if got := w.logText(); strings.TrimSpace(got) != "" {
		t.Fatalf("the log: %q", got)
	}
}
