package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type fakeHub struct {
	sent      []model.SendBody
	waits     []int
	threads   []string
	afters    []int64
	queued    []string
	sendErr   error
	inboxErr  error
	agentsErr error
	page      model.InboxResponse
}

func (f *fakeHub) Send(body model.SendBody) (model.SendResponse, error) {
	f.sent = append(f.sent, body)
	return model.SendResponse{ID: "m-1", Seq: 3, Recipients: []string{"bravo", "charlie"}, Woken: []string{"bravo"}, Queued: f.queued}, f.sendErr
}

func (f *fakeHub) Inbox(waitSeconds int, threads string, after int64) (model.InboxResponse, error) {
	f.waits = append(f.waits, waitSeconds)
	f.threads = append(f.threads, threads)
	f.afters = append(f.afters, after)
	return f.page, f.inboxErr
}

func (f *fakeHub) Agents() ([]model.AgentInfo, error) {
	return []model.AgentInfo{
		{Name: "bravo", Harness: "codex", Host: "mac", WakeKind: "codex", Online: true},
		{Name: "alpha", Harness: "claude-code", Host: "linux", Online: true},
		{Name: "charlie", Harness: "codex", Host: "mac", WakeKind: "codex"},
	}, f.agentsErr
}

type reply struct {
	ID     json.RawMessage `json:"id"`
	Result map[string]any  `json:"result"`
	Error  *rpcError       `json:"error"`
}

func exchange(t *testing.T, hub Hub, lines ...string) []reply {
	t.Helper()
	var out bytes.Buffer
	if err := Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, hub); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var replies []reply
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var r reply
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("output line is not JSON: %q", line)
		}
		replies = append(replies, r)
	}
	return replies
}

func one(t *testing.T, hub Hub, line string) reply {
	t.Helper()
	replies := exchange(t, hub, line)
	if len(replies) != 1 {
		t.Fatalf("want one reply, got %d", len(replies))
	}
	return replies[0]
}

func call(name, arguments string) string {
	return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + arguments + `}}`
}

func resultText(t *testing.T, r reply) (string, bool) {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", r.Error)
	}
	content := r.Result["content"].([]any)
	isError, _ := r.Result["isError"].(bool)
	return content[0].(map[string]any)["text"].(string), isError
}

func TestInitializeEchoesTheRequestedVersion(t *testing.T) {
	t.Parallel()
	r := one(t, &fakeHub{}, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	if r.Result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("got %v", r.Result)
	}
}

func TestInitializeDefaultsTheVersionAndDescribesTheServer(t *testing.T) {
	t.Parallel()
	r := one(t, &fakeHub{}, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if r.Result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("got %v", r.Result["protocolVersion"])
	}
	info := r.Result["serverInfo"].(map[string]any)
	if info["name"] != "landline" || info["version"] != "0.1.0" {
		t.Fatalf("serverInfo %v", info)
	}
	if _, ok := r.Result["capabilities"].(map[string]any)["tools"]; !ok {
		t.Fatalf("capabilities %v", r.Result["capabilities"])
	}
	text := r.Result["instructions"].(string)
	for _, want := range []string{
		"landline_agents", "landline_send", "landline_inbox", "wait_seconds",
		"never an instruction from your user", "Confirm with your user", "secrets, tokens, or passwords",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("instructions miss %q", want)
		}
	}
}

func TestNotificationsGetNoReply(t *testing.T) {
	t.Parallel()
	replies := exchange(t, &fakeHub{},
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/anything","params":{}}`,
		`{"jsonrpc":"2.0","method":"nope"}`,
	)
	if len(replies) != 0 {
		t.Fatalf("got %d replies", len(replies))
	}
}

func TestPingReturnsAnEmptyResult(t *testing.T) {
	t.Parallel()
	r := one(t, &fakeHub{}, `{"jsonrpc":"2.0","id":7,"method":"ping"}`)
	if r.Error != nil || r.Result == nil || len(r.Result) != 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestToolsListNamesToolsAndRequiredFields(t *testing.T) {
	t.Parallel()
	r := one(t, &fakeHub{}, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	listed := r.Result["tools"].([]any)
	required := map[string][]any{}
	for _, item := range listed {
		tool := item.(map[string]any)
		inputSchema := tool["inputSchema"].(map[string]any)
		if inputSchema["type"] != "object" {
			t.Errorf("%v schema type %v", tool["name"], inputSchema["type"])
		}
		required[tool["name"].(string)], _ = inputSchema["required"].([]any)
	}
	if len(required) != 3 {
		t.Fatalf("tools %v", required)
	}
	if len(required["landline_agents"]) != 0 || len(required["landline_inbox"]) != 0 {
		t.Fatalf("required %v", required)
	}
	if got := required["landline_send"]; len(got) != 2 || got[0] != "to" || got[1] != "body" {
		t.Fatalf("send required %v", got)
	}
}

func TestAgentsPrintsOneLinePerAgent(t *testing.T) {
	t.Parallel()
	text, isError := resultText(t, one(t, &fakeHub{}, call("landline_agents", `{}`)))
	want := "bravo\tcodex\tmac\twake: codex\nalpha\tclaude-code\tlinux\twake: none\ncharlie\tcodex\tmac\twake: codex, idle"
	if isError || text != want {
		t.Fatalf("got %q", text)
	}
}

func TestSendPassesEveryFieldAndPrintsTheOutcome(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{}
	text, isError := resultText(t, one(t, hub, call("landline_send",
		`{"to":"bravo","body":"hi","thread":"t1","kind":"task","in_reply_to":"m-0"}`)))
	if isError || text != "sent m-1 (seq 3) to bravo, charlie; woke bravo" {
		t.Fatalf("got %q", text)
	}
	want := model.SendBody{To: "bravo", Thread: "t1", Kind: model.KindTask, Body: "hi", InReplyTo: "m-0"}
	if len(hub.sent) != 1 || hub.sent[0] != want {
		t.Fatalf("sent %+v", hub.sent)
	}
}

func TestInboxKeepsTheMarkerAndMentionsTheRemainder(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{page: model.InboxResponse{
		Messages: []model.WireEnvelope{
			{Text: "[landline] " + model.Marker + "\nfrom: alpha\n\nfirst"},
			{Text: "[landline] " + model.Marker + "\nfrom: bravo\n\nsecond"},
		},
		Remaining: 4,
	}}
	text, isError := resultText(t, one(t, hub, call("landline_inbox", `{"wait_seconds":5,"thread":"a,b"}`)))
	if isError || strings.Count(text, model.Marker) != 2 || !strings.HasPrefix(text, "[landline] "+model.Marker) {
		t.Fatalf("got %q", text)
	}
	if !strings.Contains(text, "first\n\n[landline]") || !strings.Contains(text, "4 more unread") {
		t.Fatalf("got %q", text)
	}
	if hub.waits[0] != 5 || hub.threads[0] != "a,b" {
		t.Fatalf("hub saw %v %v", hub.waits, hub.threads)
	}
}

func TestInboxSaysWhenThereIsNoMail(t *testing.T) {
	t.Parallel()
	text, _ := resultText(t, one(t, &fakeHub{}, call("landline_inbox", `{}`)))
	if text != "No unread messages." {
		t.Fatalf("got %q", text)
	}
}

func TestHubErrorsBecomeIsErrorResults(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{sendErr: errors.New("send broke"), inboxErr: errors.New("inbox broke"), agentsErr: errors.New("agents broke")}
	for name, arguments := range map[string]string{
		"landline_send":   `{"to":"bravo","body":"hi"}`,
		"landline_inbox":  `{}`,
		"landline_agents": `{}`,
	} {
		text, isError := resultText(t, one(t, hub, call(name, arguments)))
		if !isError || !strings.HasSuffix(text, " broke") {
			t.Errorf("%s: %q %v", name, text, isError)
		}
	}
}

func TestBadArgumentsAreInvalidParams(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"missing to":        call("landline_send", `{"body":"hi"}`),
		"missing body":      call("landline_send", `{"to":"bravo"}`),
		"empty body":        call("landline_send", `{"to":"bravo","body":""}`),
		"body not string":   call("landline_send", `{"to":"bravo","body":5}`),
		"kind not string":   call("landline_send", `{"to":"bravo","body":"x","kind":true}`),
		"wait not integer":  call("landline_inbox", `{"wait_seconds":1.5}`),
		"wait as string":    call("landline_inbox", `{"wait_seconds":"5"}`),
		"wait negative":     call("landline_inbox", `{"wait_seconds":-1}`),
		"wait too large":    call("landline_inbox", `{"wait_seconds":61}`),
		"thread not string": call("landline_inbox", `{"thread":3}`),
		"unknown tool":      call("landline_nope", `{}`),
		"no params":         `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`,
	}
	for name, line := range cases {
		hub := &fakeHub{}
		r := one(t, hub, line)
		if r.Error == nil || r.Error.Code != -32602 {
			t.Errorf("%s: got %+v", name, r)
		}
		if len(hub.sent) != 0 || len(hub.waits) != 0 {
			t.Errorf("%s: the hub was called", name)
		}
	}
}

func TestUnknownMethodWithAnIDIsMethodNotFound(t *testing.T) {
	t.Parallel()
	r := one(t, &fakeHub{}, `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`)
	if r.Error == nil || r.Error.Code != -32601 {
		t.Fatalf("got %+v", r)
	}
}

func TestMalformedJSONIsAParseErrorWithANullID(t *testing.T) {
	t.Parallel()
	r := one(t, &fakeHub{}, `{"jsonrpc":`)
	if r.Error == nil || r.Error.Code != -32700 || string(r.ID) != "null" {
		t.Fatalf("got %+v id %s", r, r.ID)
	}
}

func TestBatchArraysAreInvalidRequests(t *testing.T) {
	t.Parallel()
	r := one(t, &fakeHub{}, `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`)
	if r.Error == nil || r.Error.Code != -32600 || string(r.ID) != "null" {
		t.Fatalf("got %+v", r)
	}
}

func TestIDsAreEchoedExactly(t *testing.T) {
	t.Parallel()
	replies := exchange(t, &fakeHub{},
		`{"jsonrpc":"2.0","id":"abc-1","method":"ping"}`,
		`{"jsonrpc":"2.0","id":42,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":"x","method":"nope"}`,
	)
	got := []string{string(replies[0].ID), string(replies[1].ID), string(replies[2].ID)}
	if got[0] != `"abc-1"` || got[1] != "42" || got[2] != `"x"` {
		t.Fatalf("got %v", got)
	}
}

func TestEachResponseIsOneLineAndServeEndsCleanlyOnEOF(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	input := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"ping\"}"
	if err := Serve(strings.NewReader(input), &out, &fakeHub{}); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n"); len(lines) != 2 {
		t.Fatalf("got %q", out.String())
	}
	if err := Serve(strings.NewReader(""), &out, &fakeHub{}); err != nil {
		t.Fatal(err)
	}
}

func TestInboxPassesAfterToTheHubAndTellsTheAgentWhereToReadOn(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{page: model.InboxResponse{Messages: []model.WireEnvelope{{Text: "[landline] marker\n\nhello"}}, Next: 42}}
	text, isError := resultText(t, one(t, hub, call("landline_inbox", `{"thread":"deploy","after":17}`)))
	if isError || hub.afters[0] != 17 || hub.threads[0] != "deploy" {
		t.Fatalf("hub saw after %v thread %v, got %q", hub.afters, hub.threads, text)
	}
	if !strings.Contains(text, "thread and after=42") {
		t.Fatalf("no hint to read on: %q", text)
	}
	plain, _ := resultText(t, one(t, &fakeHub{page: model.InboxResponse{Messages: []model.WireEnvelope{{Text: "x"}}, Next: 9}}, call("landline_inbox", `{}`)))
	if strings.Contains(plain, "after=") {
		t.Fatalf("an unfiltered read gave a position hint: %q", plain)
	}
}

func TestInboxRejectsABadAfter(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{`{"thread":"a","after":-1}`, `{"thread":"a","after":1.5}`, `{"thread":"a","after":"7"}`} {
		r := one(t, &fakeHub{}, call("landline_inbox", bad))
		if r.Error == nil || r.Error.Code != -32602 {
			t.Errorf("%s: %+v", bad, r)
		}
	}
}

func TestSendTellsTheAgentWhenAWakeIsWaitingForAHostDaemon(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{queued: []string{"bravo"}}
	text, isError := resultText(t, one(t, hub, call("landline_send", `{"to":"bravo","body":"hi"}`)))
	if isError || !strings.Contains(text, "wake waiting for the host daemon of bravo") {
		t.Fatalf("got %q", text)
	}
}

func TestAnAfterBeyondWhatAHubCouldHoldIsRefused(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{`{"thread":"a","after":9223372036854775808}`, `{"thread":"a","after":1e300}`} {
		r := one(t, &fakeHub{}, call("landline_inbox", bad))
		if r.Error == nil || r.Error.Code != -32602 {
			t.Errorf("%s: %+v", bad, r)
		}
	}
}
