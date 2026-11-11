package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
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

func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	h.now = h.now.Add(d)
	h.mu.Unlock()
}

func (h *harness) registerOn(name, host, wakeKind, wakeTarget string) string {
	h.t.Helper()
	status, raw := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: name, Harness: "codex", Host: host, Session: "s-" + name, WakeKind: wakeKind, WakeTarget: wakeTarget})
	if status != 200 {
		h.t.Fatalf("register %s: %d %s", name, status, raw)
	}
	var out model.RegisterResponse
	json.Unmarshal(raw, &out)
	return out.Token
}

func (h *harness) wakes(host string) []model.WakeJob {
	h.t.Helper()
	status, raw := h.call("GET", "/v1/wakes?host="+host, secret, nil)
	if status != 200 {
		h.t.Fatalf("wakes: %d %s", status, raw)
	}
	var out []model.WakeJob
	json.Unmarshal(raw, &out)
	return out
}

func (h *harness) notesCopy() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string{}, h.notes...)
}

func remoteHub(t *testing.T) (*harness, string) {
	h := startOn(t, "mac")
	alpha := h.registerOn("alpha", "mac", "", "")
	h.registerOn("bravo", "linux", "codex", "thread-9")
	return h, alpha
}

func TestAMessageForAnAgentOnAnotherHostIsQueuedForThatHost(t *testing.T) {
	t.Parallel()
	h, alpha := remoteHub(t)
	_, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "hi"})
	var sent model.SendResponse
	json.Unmarshal(raw, &sent)
	if len(sent.Woken) != 0 || h.waker.count() != 0 {
		t.Fatalf("woken %v calls %v", sent.Woken, h.waker.calls)
	}
	jobs := h.wakes("linux")
	if len(jobs) != 1 {
		t.Fatalf("jobs %+v", jobs)
	}
	job := jobs[0]
	if job.Agent != "bravo" || job.Kind != "codex" || job.Target != "thread-9" || job.ID == "" ||
		!strings.HasPrefix(job.Text, "[landline] "+model.Marker) || !strings.Contains(job.Text, "hi") {
		t.Fatalf("job %+v", job)
	}
	if other := h.wakes("elsewhere"); len(other) != 0 {
		t.Fatalf("another host saw %+v", other)
	}
}

func TestALeasedWakeIsNotReturnedAgainUntilTheLeaseEnds(t *testing.T) {
	t.Parallel()
	h, alpha := remoteHub(t)
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "hi"})
	if len(h.wakes("linux")) != 1 {
		t.Fatal("no first job")
	}
	h.advance(59 * time.Second)
	if again := h.wakes("linux"); len(again) != 0 {
		t.Fatalf("returned inside the lease: %+v", again)
	}
	h.advance(2 * time.Second)
	if again := h.wakes("linux"); len(again) != 1 {
		t.Fatalf("not returned after the lease: %+v", again)
	}
}

func TestAnAckRemovesTheWakeAndNotifiesThePerson(t *testing.T) {
	t.Parallel()
	h, alpha := remoteHub(t)
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "hi"})
	job := h.wakes("linux")[0]
	if status, raw := h.call("POST", "/v1/wakes/"+job.ID+"/ack", secret, model.WakeAck{OK: true}); status != 200 {
		t.Fatalf("ack: %d %s", status, raw)
	}
	h.advance(2 * time.Minute)
	if again := h.wakes("linux"); len(again) != 0 {
		t.Fatalf("still queued: %+v", again)
	}
	notes := h.notesCopy()
	if len(notes) != 1 || !strings.Contains(notes[0], "host daemon woke bravo") {
		t.Fatalf("notes %v", notes)
	}
	if status, _ := h.call("POST", "/v1/wakes/"+job.ID+"/ack", secret, model.WakeAck{OK: true}); status != 404 {
		t.Fatalf("second ack status %d", status)
	}
}

func TestAFailedAckReportsTheReason(t *testing.T) {
	t.Parallel()
	h, alpha := remoteHub(t)
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "hi"})
	job := h.wakes("linux")[0]
	h.call("POST", "/v1/wakes/"+job.ID+"/ack", secret, model.WakeAck{OK: false, Error: "no such thread"})
	notes := h.notesCopy()
	if len(notes) != 1 || !strings.Contains(notes[0], "failed") || !strings.Contains(notes[0], "no such thread") {
		t.Fatalf("notes %v", notes)
	}
}

func TestAckingAnUnknownWakeIs404(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	if status, _ := h.call("POST", "/v1/wakes/ghost/ack", secret, model.WakeAck{OK: true}); status != 404 {
		t.Fatalf("status %d", status)
	}
}

func TestTheWakeEndpointsRefuseAnAgentTokenAndNoToken(t *testing.T) {
	t.Parallel()
	h, alpha := remoteHub(t)
	for _, token := range []string{alpha, "", "nope"} {
		if status, _ := h.call("GET", "/v1/wakes?host=linux", token, nil); status != 401 {
			t.Errorf("poll with %q: status %d", token, status)
		}
		if status, _ := h.call("POST", "/v1/wakes/x/ack", token, model.WakeAck{OK: true}); status != 401 {
			t.Errorf("ack with %q: status %d", token, status)
		}
	}
}

func TestNothingIsQueuedForARecipientWaitingOnTheInbox(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	alpha := h.registerOn("alpha", "mac", "", "")
	bravo := h.registerOn("bravo", "linux", "codex", "thread-9")
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
	if jobs := h.wakes("linux"); len(jobs) != 0 {
		t.Fatalf("queued for a listener: %+v", jobs)
	}
}

func TestAWakePollWaitsForAJobAndClampsTheWait(t *testing.T) {
	t.Parallel()
	h, alpha := remoteHub(t)
	done := make(chan []model.WakeJob, 1)
	go func() {
		_, raw := h.call("GET", "/v1/wakes?host=linux&wait=5", secret, nil)
		var out []model.WakeJob
		json.Unmarshal(raw, &out)
		done <- out
	}()
	time.Sleep(200 * time.Millisecond)
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "hi"})
	select {
	case got := <-done:
		if len(got) != 1 {
			t.Fatalf("got %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the wake poll did not return")
	}
	if status, _ := h.call("GET", "/v1/wakes?wait=1", secret, nil); status != 400 {
		t.Fatalf("a poll without a host: status %d", status)
	}
}

func TestAHubWithNoHostQueuesNothing(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	h.registerOn("bravo", "linux", "codex", "t")
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "hi"})
	if jobs := h.wakes("linux"); len(jobs) != 0 {
		t.Fatalf("queued %+v", jobs)
	}
	if h.waker.count() != 1 {
		t.Fatalf("local wake count %d", h.waker.count())
	}
}

func TestARemoteAgentMayNameAWakeMethodOnlyItsHostDaemonHas(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	for kind, want := range map[string]int{"claude-remote": 200, "Bad Kind": 400} {
		status, raw := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: "bravo", Harness: "x", Host: "linux", Session: "s", WakeKind: kind, WakeTarget: "t"})
		if status != want {
			t.Errorf("%q: status %d %s, want %d", kind, status, raw, want)
		}
	}
	status, _ := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: "charlie", Harness: "x", Host: "mac", Session: "s", WakeKind: "claude-remote", WakeTarget: "t"})
	if status != 400 {
		t.Fatalf("a local agent named a wake method this hub lacks: %d", status)
	}
}

func TestPeekCountsUnreadMailWithoutConsumingIt(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "", "")
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "one"})
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "two"})
	for i := 0; i < 2; i++ {
		if got := h.inbox(bravo, "?peek=1"); got.Remaining != 2 || len(got.Messages) != 0 {
			t.Fatalf("peek %d: %+v", i, got)
		}
	}
	if got := h.inbox(bravo, ""); len(got.Messages) != 2 {
		t.Fatalf("the peek consumed mail: %+v", got)
	}
	if got := h.inbox(bravo, "?peek=1"); got.Remaining != 0 {
		t.Fatalf("after reading: %+v", got)
	}
}

func TestTokensAndCursorsSurviveAHubRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	open := func() (*httptest.Server, func()) {
		log, err := store.Open(filepath.Join(dir, "log.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		hub, err := New(Config{Secret: secret, StatePath: filepath.Join(dir, "state.json"), NewID: func() string { return "m-" + time.Now().Format("150405.000000") }}, log)
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(hub.Handler())
		return srv, func() { srv.Close(); log.Close() }
	}
	call := func(srv *httptest.Server, method, path, token string, body any) (int, []byte) {
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return resp.StatusCode, buf.Bytes()
	}
	first, stop := open()
	var alpha, bravo model.RegisterResponse
	_, raw := call(first, "POST", "/v1/register", secret, model.RegisterRequest{Name: "alpha", Harness: "x"})
	json.Unmarshal(raw, &alpha)
	_, raw = call(first, "POST", "/v1/register", secret, model.RegisterRequest{Name: "bravo", Harness: "x"})
	json.Unmarshal(raw, &bravo)
	call(first, "POST", "/v1/send", alpha.Token, model.SendBody{To: "bravo", Body: "one"})
	call(first, "GET", "/v1/inbox", bravo.Token, nil)
	call(first, "POST", "/v1/send", alpha.Token, model.SendBody{To: "bravo", Body: "two"})
	stop()

	second, stop := open()
	defer stop()
	status, raw := call(second, "GET", "/v1/inbox", bravo.Token, nil)
	var got model.InboxResponse
	json.Unmarshal(raw, &got)
	if status != 200 || len(got.Messages) != 1 || got.Messages[0].Body != "two" {
		t.Fatalf("after restart: %d %s", status, raw)
	}
	if status, _ := call(second, "POST", "/v1/send", alpha.Token, model.SendBody{To: "bravo", Body: "three"}); status != 200 {
		t.Fatalf("the old token stopped working: %d", status)
	}
}

func TestAStateFileThatIsNotReadableIsReportedAndIgnored(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte("not json"), 0o600)
	log, _ := store.Open(filepath.Join(dir, "log.jsonl"))
	defer log.Close()
	var notes []string
	if _, err := New(Config{Secret: secret, StatePath: path, Notify: func(s string) { notes = append(notes, s) }}, log); err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "register again") {
		t.Fatalf("notes %v", notes)
	}
}

type slowWaker struct {
	delay time.Duration
	mu    sync.Mutex
	calls int
}

func (w *slowWaker) Wake(string, string) error {
	time.Sleep(w.delay)
	w.mu.Lock()
	w.calls++
	w.mu.Unlock()
	return nil
}

func TestWakesToSeveralLocalAgentsRunAtTheSameTimeNotOneAfterAnother(t *testing.T) {
	t.Parallel()
	log, _ := store.Open(filepath.Join(t.TempDir(), "log.jsonl"))
	defer log.Close()
	slow := &slowWaker{delay: 400 * time.Millisecond}
	hub, err := New(Config{Secret: secret, Wakers: map[string]Waker{"codex": slow}}, log)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(hub.Handler())
	defer srv.Close()
	h := &harness{t: t, srv: srv, waker: &fakeWaker{}, now: time.Now()}
	alpha := h.register("alpha", "", "")
	for _, name := range []string{"bravo", "charlie", "delta", "echo"} {
		h.register(name, "codex", "t-"+name)
	}
	started := time.Now()
	_, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: "all", Body: "wake everyone"})
	var out model.SendResponse
	json.Unmarshal(raw, &out)
	if len(out.Woken) != 4 || time.Since(started) > 1200*time.Millisecond {
		t.Fatalf("woken %v after %v", out.Woken, time.Since(started))
	}
}

func TestAWakeForAnAgentWhoseMethodTheHubLacksIsPrinted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	log, _ := store.Open(filepath.Join(dir, "log.jsonl"))
	defer log.Close()
	state := filepath.Join(dir, "state.json")
	first, _ := New(Config{Secret: secret, StatePath: state, Wakers: map[string]Waker{"codex": &fakeWaker{}}}, log)
	srv := httptest.NewServer(first.Handler())
	h := &harness{t: t, srv: srv, waker: &fakeWaker{}, now: time.Now()}
	alpha := h.register("alpha", "", "")
	h.register("bravo", "codex", "t")
	srv.Close()
	var notes []string
	second, _ := New(Config{Secret: secret, StatePath: state, Notify: func(s string) { notes = append(notes, s) }}, log)
	srv = httptest.NewServer(second.Handler())
	defer srv.Close()
	h.srv = srv
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "x"})
	if len(notes) != 1 || !strings.Contains(notes[0], "no wake method codex") {
		t.Fatalf("notes %v", notes)
	}
}
