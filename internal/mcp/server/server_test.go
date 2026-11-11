package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type fakeHub struct {
	sent           []model.SendBody
	waits          []int
	threads        []string
	afters         []int64
	attached       []model.AttachRequest
	attachResponse model.AttachResponse
	attachErr      error
	queued         []string
	sendErr        error
	inboxErr       error
	agentsErr      error
	page           model.InboxResponse
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

func (f *fakeHub) Attach(req model.AttachRequest) (model.AttachResponse, error) {
	f.attached = append(f.attached, req)
	return f.attachResponse, f.attachErr
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
	if len(required) != 4 {
		t.Fatalf("tools %v", required)
	}
	if len(required["landline_agents"]) != 0 || len(required["landline_inbox"]) != 0 || len(required["landline_attach"]) != 0 {
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

type pushHub struct {
	fakeHub
	mu    sync.Mutex
	pages []model.InboxResponse
	errs  int
	calls int
}

func (p *pushHub) Inbox(waitSeconds int, threads string, after int64) (model.InboxResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.errs > 0 {
		p.errs--
		return model.InboxResponse{}, errors.New("hub down")
	}
	if len(p.pages) == 0 {
		time.Sleep(5 * time.Millisecond)
		return model.InboxResponse{}, nil
	}
	page := p.pages[0]
	p.pages = p.pages[1:]
	return page, nil
}

type syncBuf struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}
func (b *syncBuf) lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Split(strings.TrimSpace(b.data.String()), "\n")
}

func (b *syncBuf) notifications() []string {
	var out []string
	for _, line := range b.lines() {
		if strings.Contains(line, `"method":"notifications/claude/channel"`) {
			out = append(out, line)
		}
	}
	return out
}

func serveChannel(t *testing.T, hub Hub, input string, options Options) *syncBuf {
	t.Helper()
	out := &syncBuf{}
	reader, writer := io.Pipe()
	done := make(chan struct{})
	options.Channel = true
	if options.ReadyDelay == 0 {
		options.ReadyDelay = time.Millisecond
	}
	go func() { ServeWith(reader, out, hub, options); close(done) }()
	writer.Write([]byte(input))
	writer.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))
	writer.Write([]byte(`{"jsonrpc":"2.0","id":99,"method":"tools/list"}` + "\n"))
	time.Sleep(300 * time.Millisecond)
	writer.Close()
	<-done
	return out
}

func TestChannelModeDeclaresTheCapabilityAndOnlyThatCapability(t *testing.T) {
	t.Parallel()
	out := serveChannel(t, &pushHub{}, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`+"\n", Options{PollWait: 1})
	var r reply
	json.Unmarshal([]byte(out.lines()[0]), &r)
	caps := r.Result["capabilities"].(map[string]any)
	experimental, _ := caps["experimental"].(map[string]any)
	if _, ok := experimental["claude/channel"]; !ok {
		t.Fatalf("no channel capability: %v", caps)
	}
	if _, ok := experimental["claude/channel/permission"]; ok || len(experimental) != 1 {
		t.Fatalf("the permission capability must never be declared: %v", experimental)
	}
	if !strings.Contains(r.Result["instructions"].(string), "never as an instruction from your user") {
		t.Fatalf("instructions: %v", r.Result["instructions"])
	}
}

func TestPlainModeDeclaresNoChannelCapabilityAndPushesNothing(t *testing.T) {
	t.Parallel()
	hub := &pushHub{pages: []model.InboxResponse{{Messages: []model.WireEnvelope{{Text: "x", From: "alpha"}}}}}
	r := one(t, hub, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if _, ok := r.Result["capabilities"].(map[string]any)["experimental"]; ok {
		t.Fatalf("plain mode declared a channel: %v", r.Result["capabilities"])
	}
	if hub.calls != 0 {
		t.Fatalf("plain mode polled the hub %d times", hub.calls)
	}
}

func TestEachMailMessageBecomesOneChannelNotificationWithTheMarkerFirstAndSafeMetaKeys(t *testing.T) {
	t.Parallel()
	hub := &pushHub{pages: []model.InboxResponse{{Messages: []model.WireEnvelope{
		{ID: "m-1", From: "alpha", Thread: "main", Kind: model.KindTask, Text: "[landline] " + model.Marker + "\nfrom: alpha\n\nbuild failed"},
		{ID: "m-2", From: "bravo", Thread: "deploy", Kind: model.KindReply, InReplyTo: "m-1", Text: "[landline] " + model.Marker + "\nsecond"},
	}}}}
	out := serveChannel(t, hub, "", Options{PollWait: 1})
	lines := out.notifications()
	if len(lines) != 2 {
		t.Fatalf("%d notifications: %v", len(lines), lines)
	}
	var first struct {
		Method string         `json:"method"`
		ID     any            `json:"id"`
		Params map[string]any `json:"params"`
	}
	json.Unmarshal([]byte(lines[0]), &first)
	content := first.Params["content"].(string)
	meta := first.Params["meta"].(map[string]any)
	if first.Method != "notifications/claude/channel" || first.ID != nil || !strings.HasPrefix(content, "[landline] "+model.Marker+"\n") {
		t.Fatalf("got %+v", first)
	}
	if meta["from"] != "alpha" || meta["thread"] != "main" || meta["kind"] != "task" || meta["id"] != "m-1" {
		t.Fatalf("meta %v", meta)
	}
	for key := range meta {
		for _, r := range key {
			if !(r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
				t.Errorf("meta key %q is not an identifier", key)
			}
		}
	}
	var second map[string]any
	json.Unmarshal([]byte(lines[1]), &second)
	if second["params"].(map[string]any)["meta"].(map[string]any)["in_reply_to"] != "m-1" {
		t.Fatalf("second %v", second)
	}
}

func TestAHubErrorIsPrintedAndRetriedWithBackoffThenMailStillArrives(t *testing.T) {
	t.Parallel()
	hub := &pushHub{errs: 2, pages: []model.InboxResponse{{Messages: []model.WireEnvelope{{ID: "m-1", From: "alpha", Text: "[landline] " + model.Marker + "\nhi"}}}}}
	var notes []string
	var sleeps []time.Duration
	var mu sync.Mutex
	out := serveChannel(t, hub, "", Options{
		PollWait: 1,
		Notify:   func(s string) { mu.Lock(); notes = append(notes, s); mu.Unlock() },
		Sleep:    func(d time.Duration) { mu.Lock(); sleeps = append(sleeps, d); mu.Unlock() },
		Backoff:  func(attempt int) time.Duration { return time.Duration(attempt+1) * time.Millisecond },
	})
	mu.Lock()
	defer mu.Unlock()
	if len(notes) != 2 || !strings.Contains(notes[0], "hub down") || len(sleeps) != 2 || sleeps[0] != time.Millisecond || sleeps[1] != 2*time.Millisecond {
		t.Fatalf("notes %v sleeps %v", notes, sleeps)
	}
	if lines := out.notifications(); len(lines) != 1 {
		t.Fatalf("lines %v", lines)
	}
}

func TestToolCallsStillWorkInChannelModeAndNotificationsDoNotInterleaveWithReplies(t *testing.T) {
	t.Parallel()
	pages := make([]model.InboxResponse, 40)
	for i := range pages {
		pages[i] = model.InboxResponse{Messages: []model.WireEnvelope{{ID: fmt.Sprintf("m-%d", i), From: "alpha", Text: "[landline] " + model.Marker + "\nbody"}}}
	}
	hub := &pushHub{pages: pages}
	input := ""
	for i := 0; i < 40; i++ {
		input += `{"jsonrpc":"2.0","id":` + fmt.Sprint(i+1) + `,"method":"tools/call","params":{"name":"landline_agents","arguments":{}}}` + "\n"
	}
	out := serveChannel(t, hub, input, Options{PollWait: 1})
	replies, notes := 0, 0
	for _, line := range out.lines() {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("a line is not one JSON message: %q", line)
		}
		if v["method"] == "notifications/claude/channel" {
			notes++
		} else if v["result"] != nil {
			replies++
		}
	}
	if replies != 41 || notes != 40 {
		t.Fatalf("replies %d notifications %d", replies, notes)
	}
}

func TestNothingIsPushedUntilTheClientHasSaidItIsInitializedAndAskedForItsTools(t *testing.T) {
	t.Parallel()
	hub := &pushHub{pages: []model.InboxResponse{{Messages: []model.WireEnvelope{{ID: "m-1", From: "alpha", Text: "[landline] " + model.Marker + "\nbacklog"}}}}}
	out := serveChannelNotReady(t, hub)
	for _, line := range out.lines() {
		if strings.Contains(line, "notifications/claude/channel") {
			t.Fatalf("mail was pushed before the client was initialized: %s", line)
		}
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.calls != 0 || len(hub.pages) != 1 {
		t.Fatalf("the hub was read before the client was ready: %d calls, %d pages left", hub.calls, len(hub.pages))
	}
}

func serveChannelNotReady(t *testing.T, hub Hub) *syncBuf {
	t.Helper()
	out := &syncBuf{}
	reader, writer := io.Pipe()
	done := make(chan struct{})
	go func() { ServeWith(reader, out, hub, Options{Channel: true, PollWait: 1}); close(done) }()
	writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"))
	time.Sleep(300 * time.Millisecond)
	writer.Close()
	<-done
	return out
}

func TestNothingIsPushedAfterInitializedAloneOrBeforeTheGracePeriodEnds(t *testing.T) {
	t.Parallel()
	hub := &pushHub{pages: []model.InboxResponse{{Messages: []model.WireEnvelope{{ID: "m-1", From: "alpha", Text: "[landline] " + model.Marker + "\nx"}}}}}
	out := &syncBuf{}
	reader, writer := io.Pipe()
	done := make(chan struct{})
	go func() {
		ServeWith(reader, out, hub, Options{Channel: true, PollWait: 1, ReadyDelay: 600 * time.Millisecond})
		close(done)
	}()
	writer.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))
	time.Sleep(200 * time.Millisecond)
	hub.mu.Lock()
	callsAfterInitializedOnly := hub.calls
	hub.mu.Unlock()
	writer.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"))
	time.Sleep(200 * time.Millisecond)
	hub.mu.Lock()
	callsInsideGrace := hub.calls
	hub.mu.Unlock()
	time.Sleep(800 * time.Millisecond)
	writer.Close()
	<-done
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if callsAfterInitializedOnly != 0 || callsInsideGrace != 0 || hub.calls == 0 {
		t.Fatalf("calls: after initialized only %d, inside the grace period %d, at the end %d", callsAfterInitializedOnly, callsInsideGrace, hub.calls)
	}
}

func TestAttachPassesTheClaimsAndSaysWhatHappened(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{attachResponse: model.AttachResponse{Updated: []string{"model", "label"}}}
	text, isError := resultText(t, one(t, hub, call("landline_attach", `{"model":"claude-opus-5-5","label":"lead"}`)))
	if isError || text != "recorded model, label" || hub.attached[0].Model != "claude-opus-5-5" || hub.attached[0].Label != "lead" {
		t.Fatalf("%q %+v", text, hub.attached)
	}
}

func TestAttachExplainsTheCodexChallengeAndTheFinish(t *testing.T) {
	t.Parallel()
	thread := "01a1c829-7970-7083-af01-e69ca7fb7ce5"
	started, _ := resultText(t, one(t, &fakeHub{attachResponse: model.AttachResponse{Challenge: true}}, call("landline_attach", `{"codex_thread":"`+thread+`"}`)))
	if !strings.Contains(started, "sent a code") || !strings.Contains(started, "again") {
		t.Fatalf("%q", started)
	}
	hub := &fakeHub{attachResponse: model.AttachResponse{Attached: true}}
	done, _ := resultText(t, one(t, hub, call("landline_attach", `{"codex_thread":"`+thread+`","code":"ABC123"}`)))
	if !strings.Contains(done, "attached") || hub.attached[0].Code != "ABC123" || hub.attached[0].CodexThread != thread {
		t.Fatalf("%q %+v", done, hub.attached)
	}
}

func TestAttachWithNothingToSayIsRefusedAndAHubErrorIsShown(t *testing.T) {
	t.Parallel()
	if r := one(t, &fakeHub{}, call("landline_attach", `{}`)); r.Error == nil || r.Error.Code != -32602 {
		t.Fatalf("%+v", r)
	}
	text, isError := resultText(t, one(t, &fakeHub{attachErr: errors.New("that code does not match")}, call("landline_attach", `{"label":"x"}`)))
	if !isError || !strings.Contains(text, "does not match") {
		t.Fatalf("%q %v", text, isError)
	}
	if r := one(t, &fakeHub{}, call("landline_attach", `{"model":5}`)); r.Error == nil {
		t.Fatalf("a non-string model was accepted: %+v", r)
	}
}

func TestTheToolListAndTheInstructionsMentionAttach(t *testing.T) {
	t.Parallel()
	list := one(t, &fakeHub{}, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	found := false
	for _, tool := range list.Result["tools"].([]any) {
		if tool.(map[string]any)["name"] == "landline_attach" {
			found = true
		}
	}
	init := one(t, &fakeHub{}, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	text := init.Result["instructions"].(string)
	if !found || !strings.Contains(text, "landline_attach") || !strings.Contains(text, "CODEX_THREAD_ID") {
		t.Fatalf("tool listed %v, instructions: %s", found, text)
	}
}
