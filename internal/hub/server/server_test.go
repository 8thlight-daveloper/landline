package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/log/store"
	"github.com/8thlight-daveloper/landline/internal/model"
)

const secret = "0123456789abcdef-secret"

type fakeWaker struct {
	mu    sync.Mutex
	calls []string
	fail  error
}

func (f *fakeWaker) Wake(target, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, target+"|"+text)
	return f.fail
}

func (f *fakeWaker) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type harness struct {
	t     *testing.T
	srv   *httptest.Server
	waker *fakeWaker
	notes []string
	now   time.Time
	mu    sync.Mutex
}

func start(t *testing.T) *harness { return startOn(t, "") }

func startOn(t *testing.T, host string) *harness {
	t.Helper()
	log, err := store.Open(filepath.Join(t.TempDir(), "log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	h := &harness{t: t, waker: &fakeWaker{}, now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	counter := 0
	hub, err := New(Config{
		Secret: secret,
		Host:   host,
		Now:    func() time.Time { h.mu.Lock(); defer h.mu.Unlock(); return h.now },
		NewID:  func() string { counter++; return "m-" + string(rune('a'+counter)) },
		Wakers: map[string]Waker{"codex": h.waker},
		Notify: func(s string) { h.mu.Lock(); h.notes = append(h.notes, s); h.mu.Unlock() },
	}, log)
	if err != nil {
		t.Fatal(err)
	}
	h.srv = httptest.NewServer(hub.Handler())
	t.Cleanup(h.srv.Close)
	return h
}

func (h *harness) call(method, path, token string, body any) (int, []byte) {
	h.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes()
}

func (h *harness) register(name, wakeKind, wakeTarget string) string {
	h.t.Helper()
	status, raw := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: name, Harness: "codex", Host: "mac", Session: "s-" + name, WakeKind: wakeKind, WakeTarget: wakeTarget})
	if status != 200 {
		h.t.Fatalf("register %s: %d %s", name, status, raw)
	}
	var out model.RegisterResponse
	json.Unmarshal(raw, &out)
	return out.Token
}

func (h *harness) inbox(token, query string) model.InboxResponse {
	h.t.Helper()
	status, raw := h.call("GET", "/v1/inbox"+query, token, nil)
	if status != 200 {
		h.t.Fatalf("inbox: %d %s", status, raw)
	}
	var out model.InboxResponse
	json.Unmarshal(raw, &out)
	return out
}

func TestTheSecretMustBeLongEnough(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{Secret: "short"}, nil); err == nil {
		t.Fatal("accepted a short secret")
	}
}

func TestRegisterNeedsTheHubSecret(t *testing.T) {
	t.Parallel()
	h := start(t)
	if status, _ := h.call("POST", "/v1/register", "wrong", model.RegisterRequest{Name: "alpha", Harness: "codex"}); status != 401 {
		t.Fatalf("status %d", status)
	}
	if status, _ := h.call("POST", "/v1/register", "", model.RegisterRequest{Name: "alpha", Harness: "codex"}); status != 401 {
		t.Fatalf("status %d", status)
	}
}

func TestRegisterRefusesABadNameAnUnknownWakeMethodAndAWakeWithoutATarget(t *testing.T) {
	t.Parallel()
	h := start(t)
	for _, body := range []model.RegisterRequest{
		{Name: "Bad Name", Harness: "codex"},
		{Name: "alpha", Harness: "codex", WakeKind: "pigeon", WakeTarget: "x"},
		{Name: "alpha", Harness: "codex", WakeKind: "codex"},
	} {
		if status, _ := h.call("POST", "/v1/register", secret, body); status != 400 {
			t.Errorf("%+v: status %d", body, status)
		}
	}
}

func TestAMessageIsStampedFromTheTokenAndReadFromTheInbox(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "", "")
	status, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "hello"})
	if status != 200 {
		t.Fatalf("send: %d %s", status, raw)
	}
	got := h.inbox(bravo, "")
	if len(got.Messages) != 1 {
		t.Fatalf("got %+v", got)
	}
	m := got.Messages[0]
	if m.From != "alpha" || m.Body != "hello" || m.Thread != "main" || !strings.HasPrefix(m.Text, "[landline] "+model.Marker) {
		t.Fatalf("message %+v", m)
	}
	if again := h.inbox(bravo, ""); len(again.Messages) != 0 {
		t.Fatalf("read twice: %+v", again)
	}
	if mine := h.inbox(alpha, ""); len(mine.Messages) != 0 {
		t.Fatalf("own message came back: %+v", mine)
	}
}

func TestARequestCannotNameItsOwnSender(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	h.register("bravo", "", "")
	raw := map[string]string{"to": "bravo", "body": "hi", "from": "bravo"}
	if status, _ := h.call("POST", "/v1/send", alpha, raw); status != 400 {
		t.Fatalf("status %d", status)
	}
}

func TestSendNeedsAnAgentTokenAndTheSecretIsNotOne(t *testing.T) {
	t.Parallel()
	h := start(t)
	h.register("bravo", "", "")
	for _, token := range []string{"", "nope", secret} {
		if status, _ := h.call("POST", "/v1/send", token, model.SendBody{To: "bravo", Body: "x"}); status != 401 {
			t.Errorf("token %q: status %d", token, status)
		}
	}
}

func TestRegisteringAgainRevokesTheOldToken(t *testing.T) {
	t.Parallel()
	h := start(t)
	old := h.register("alpha", "", "")
	h.register("bravo", "", "")
	fresh := h.register("alpha", "", "")
	if status, _ := h.call("POST", "/v1/send", old, model.SendBody{To: "bravo", Body: "x"}); status != 401 {
		t.Fatalf("old token status %d", status)
	}
	if status, _ := h.call("POST", "/v1/send", fresh, model.SendBody{To: "bravo", Body: "x"}); status != 200 {
		t.Fatalf("new token status %d", status)
	}
}

func TestRefusalsMapToStatuses(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	for _, c := range []struct {
		body model.SendBody
		want int
	}{
		{model.SendBody{To: "ghost", Body: "x"}, 404},
		{model.SendBody{To: "alpha", Body: "x"}, 422},
		{model.SendBody{To: "all", Body: ""}, 422},
		{model.SendBody{To: "all", Body: strings.Repeat("x", model.MaxBodyBytes+1)}, 413},
	} {
		if status, _ := h.call("POST", "/v1/send", alpha, c.body); status != c.want {
			t.Errorf("%+v: status %d, want %d", c.body, status, c.want)
		}
	}
}

func TestABroadcastReachesEveryoneElseAndANewAgentSkipsHistory(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "", "")
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "all", Body: "before"})
	charlie := h.register("charlie", "", "")
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "all", Body: "after"})
	if got := h.inbox(bravo, ""); len(got.Messages) != 2 {
		t.Fatalf("bravo got %+v", got)
	}
	if got := h.inbox(charlie, ""); len(got.Messages) != 1 || got.Messages[0].Body != "after" {
		t.Fatalf("charlie got %+v", got)
	}
}

func TestASendWakesARecipientWithAWakeMethodAndNotifiesThePerson(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	h.register("bravo", "codex", "thread-9")
	_, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "wake up"})
	var out model.SendResponse
	json.Unmarshal(raw, &out)
	if len(out.Woken) != 1 || out.Woken[0] != "bravo" || h.waker.count() != 1 {
		t.Fatalf("response %+v, calls %v", out, h.waker.calls)
	}
	call := h.waker.calls[0]
	if !strings.HasPrefix(call, "thread-9|[landline] "+model.Marker) || !strings.Contains(call, "wake up") {
		t.Fatalf("call %q", call)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.notes) != 1 || !strings.Contains(h.notes[0], "woke bravo") {
		t.Fatalf("notes %v", h.notes)
	}
}

func TestAnAgentOnAnotherHostIsLeftToThatHostsDaemon(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	alpha := h.register("alpha", "", "")
	status, raw := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: "bravo", Harness: "codex", Host: "linux", Session: "s", WakeKind: "codex", WakeTarget: "t"})
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "hi"})
	if h.waker.count() != 0 {
		t.Fatalf("the hub woke an agent on another host: %v", h.waker.calls)
	}
}

func TestAFailedWakeIsReportedAndTheMessageStillArrives(t *testing.T) {
	t.Parallel()
	h := start(t)
	h.waker.fail = errors.New("no such thread")
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "codex", "thread-9")
	_, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "hi"})
	var out model.SendResponse
	json.Unmarshal(raw, &out)
	if len(out.Woken) != 0 {
		t.Fatalf("woken %v", out.Woken)
	}
	if got := h.inbox(bravo, ""); len(got.Messages) != 1 {
		t.Fatalf("got %+v", got)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.notes) != 1 || !strings.Contains(h.notes[0], "failed") {
		t.Fatalf("notes %v", h.notes)
	}
}

func TestAnAgentWaitingOnTheInboxIsNotWoken(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "codex", "thread-9")
	done := make(chan model.InboxResponse, 1)
	go func() { done <- h.inbox(bravo, "?wait=5") }()
	time.Sleep(200 * time.Millisecond)
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "hi"})
	select {
	case got := <-done:
		if len(got.Messages) != 1 {
			t.Fatalf("got %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the long poll did not return")
	}
	if h.waker.count() != 0 {
		t.Fatalf("woke an agent that was listening: %v", h.waker.calls)
	}
}

func TestALongPollReturnsEmptyAtTheDeadline(t *testing.T) {
	t.Parallel()
	h := start(t)
	bravo := h.register("bravo", "", "")
	started := time.Now()
	got := h.inbox(bravo, "?wait=1")
	if len(got.Messages) != 0 || time.Since(started) < 900*time.Millisecond {
		t.Fatalf("got %+v after %v", got, time.Since(started))
	}
}

func TestAThreadFilterReadsWithoutMovingTheCursor(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "", "")
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Thread: "deploy", Body: "d"})
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Thread: "main", Body: "m"})
	if got := h.inbox(bravo, "?thread=deploy"); len(got.Messages) != 1 || got.Messages[0].Body != "d" {
		t.Fatalf("got %+v", got)
	}
	if got := h.inbox(bravo, ""); len(got.Messages) != 2 {
		t.Fatalf("the filtered read moved the cursor: %+v", got)
	}
}

func TestAgentsListsOnlyThoseHeardFromWithinTheWindow(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	h.register("bravo", "", "")
	h.mu.Lock()
	h.now = h.now.Add(2 * time.Minute)
	h.mu.Unlock()
	status, raw := h.call("GET", "/v1/agents", alpha, nil)
	if status != 200 {
		t.Fatalf("status %d", status)
	}
	var out []model.AgentInfo
	json.Unmarshal(raw, &out)
	if len(out) != 1 || out[0].Name != "alpha" {
		t.Fatalf("got %+v", out)
	}
}

func TestTheLogNeedsTheSecretAndReturnsWhatWasSent(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	h.register("bravo", "", "")
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "logged"})
	if status, _ := h.call("GET", "/v1/log", alpha, nil); status != 401 {
		t.Fatalf("an agent token read the log: %d", status)
	}
	status, raw := h.call("GET", "/v1/log", secret, nil)
	if status != 200 || !strings.Contains(string(raw), "logged") {
		t.Fatalf("status %d body %s", status, raw)
	}
}
