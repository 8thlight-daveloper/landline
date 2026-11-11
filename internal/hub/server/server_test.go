package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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
	counter, suffix := 0, 0
	hub, err := New(Config{
		Secret:    secret,
		Host:      host,
		Now:       func() time.Time { h.mu.Lock(); defer h.mu.Unlock(); return h.now },
		NewID:     func() string { counter++; return "m-" + string(rune('a'+counter)) },
		NewCode:   func() string { return "ABC123" },
		NewSuffix: func() string { suffix++; return fmt.Sprintf("s%02d", suffix) },
		Wakers:    map[string]Waker{"codex": h.waker},
		Notify:    func(s string) { h.mu.Lock(); h.notes = append(h.notes, s); h.mu.Unlock() },
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

func TestAgentsMarksTheOnesNotHeardFromRecentlyIdleAndDropsThemAfterADay(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	h.register("bravo", "", "")
	h.mu.Lock()
	h.now = h.now.Add(2 * time.Minute)
	h.mu.Unlock()
	list := func() map[string]bool {
		status, raw := h.call("GET", "/v1/agents", alpha, nil)
		if status != 200 {
			t.Fatalf("status %d", status)
		}
		var out []model.AgentInfo
		json.Unmarshal(raw, &out)
		online := map[string]bool{}
		for _, a := range out {
			online[a.Name] = a.Online
		}
		return online
	}
	if got := list(); len(got) != 2 || !got["alpha"] || got["bravo"] {
		t.Fatalf("after two minutes: %v", got)
	}
	h.mu.Lock()
	h.now = h.now.Add(25 * time.Hour)
	h.mu.Unlock()
	if got := list(); len(got) != 1 || !got["alpha"] {
		t.Fatalf("after a day: %v", got)
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

func TestAThreadFilteredWaitDoesNotSuppressAWakeForAnotherThread(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "codex", "thread-9")
	done := make(chan struct{})
	go func() { h.inbox(bravo, "?thread=dev&wait=2"); close(done) }()
	time.Sleep(200 * time.Millisecond)
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Thread: "main", Body: "x"})
	<-done
	if h.waker.count() != 1 {
		t.Fatalf("the wake was suppressed: %d calls", h.waker.count())
	}
}

func TestAPageOfLargeMessagesIsCappedAndNothingIsSkipped(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "", "")
	body := strings.Repeat("a", 60000)
	for i := 0; i < 40; i++ {
		if status, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: body}); status != 200 {
			t.Fatalf("send %d: %d %s", i, status, raw)
		}
	}
	total, reads := 0, 0
	for total < 40 && reads < 40 {
		got := h.inbox(bravo, "")
		reads++
		if len(got.Messages) == 0 {
			break
		}
		if len(got.Messages) > 20 {
			t.Fatalf("a page held %d messages of 60 KB", len(got.Messages))
		}
		total += len(got.Messages)
		if want := 40 - total; got.Remaining != want {
			t.Fatalf("remaining %d, want %d", got.Remaining, want)
		}
	}
	if total != 40 {
		t.Fatalf("read %d of 40 messages in %d pages", total, reads)
	}
}

func TestTheWakeQueueForAHostWithNoDaemonIsBounded(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	alpha := h.register("alpha", "", "")
	if status, raw := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: "bravo", Harness: "x", Host: "linux", Session: "s", WakeKind: "codex", WakeTarget: "t"}); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	for i := 0; i < maxQueuedWakes+25; i++ {
		h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "x"})
	}
	status, raw := h.call("GET", "/v1/wakes?host=linux", secret, nil)
	var jobs []model.WakeJob
	json.Unmarshal(raw, &jobs)
	if status != 200 || len(jobs) != maxQueuedWakes {
		t.Fatalf("status %d with %d jobs", status, len(jobs))
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.notes) == 0 || !strings.Contains(h.notes[0], "queue for host linux is full") {
		t.Fatalf("notes %v", h.notes)
	}
}

func TestSendNamesTheAgentsWhoseWakeIsWaitingForAHostDaemon(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	alpha := h.register("alpha", "", "")
	h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: "bravo", Harness: "x", Host: "linux", Session: "s", WakeKind: "codex", WakeTarget: "t"})
	_, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "x"})
	var out model.SendResponse
	json.Unmarshal(raw, &out)
	if len(out.Queued) != 1 || out.Queued[0] != "bravo" || len(out.Woken) != 0 {
		t.Fatalf("%+v", out)
	}
}

func TestAnAgentWhosePollHasFinishedIsWokenByTheNextMessage(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "codex", "thread-9")
	done := make(chan model.InboxResponse, 1)
	go func() { done <- h.inbox(bravo, "?wait=5") }()
	time.Sleep(200 * time.Millisecond)
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "one"})
	got := <-done
	if len(got.Messages) != 1 {
		t.Fatalf("got %+v", got)
	}
	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "two"})
	if h.waker.count() != 1 {
		t.Fatalf("the second message did not wake an agent with no poll open: %d calls", h.waker.count())
	}
}

func TestPeekAndInboxReportTheHeadOfTheLog(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "", "")
	for i := 0; i < 3; i++ {
		h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "x"})
	}
	if got := h.inbox(bravo, "?peek=1"); got.Head != 3 {
		t.Fatalf("peek head %d", got.Head)
	}
	if got := h.inbox(bravo, ""); got.Head != 3 {
		t.Fatalf("inbox head %d", got.Head)
	}
}

func TestAStateFileThatCannotBeReadIsReported(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read any file")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, []byte("{}"), 0o000)
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

func TestTheStateFileKeysAreSnakeCase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	log, _ := store.Open(filepath.Join(dir, "log.jsonl"))
	defer log.Close()
	hub, _ := New(Config{Secret: secret, StatePath: filepath.Join(dir, "state.json"), Wakers: map[string]Waker{"codex": &fakeWaker{}}}, log)
	srv := httptest.NewServer(hub.Handler())
	defer srv.Close()
	h := &harness{t: t, srv: srv, waker: &fakeWaker{}, now: time.Now()}
	h.register("alpha", "codex", "t1")
	raw, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	for _, key := range []string{`"agents"`, `"tokens"`, `"cursors"`, `"name":"alpha"`, `"wake_kind":"codex"`, `"wake_target":"t1"`, `"last_seen"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("the state lacks %s: %s", key, raw)
		}
	}
}

func TestAReadPositionPastTheEndOfTheLogIsMovedBack(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	os.WriteFile(state, []byte(`{"agents":[],"tokens":{},"cursors":{"bravo":99}}`), 0o600)
	log, _ := store.Open(filepath.Join(dir, "log.jsonl"))
	defer log.Close()
	var notes []string
	hub, err := New(Config{Secret: secret, StatePath: state, Notify: func(s string) { notes = append(notes, s) }}, log)
	if err != nil {
		t.Fatal(err)
	}
	if hub.cursors["bravo"] != 0 || len(notes) != 1 || !strings.Contains(notes[0], "moved back to 0") {
		t.Fatalf("cursor %d notes %v", hub.cursors["bravo"], notes)
	}
}

func TestNoWakeIsLostWhenASendLandsAsALongPollEnds(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	const agents = 60
	names := make([]string, agents)
	tokens := make([]string, agents)
	for i := range names {
		names[i] = fmt.Sprintf("poller%02d", i)
		tokens[i] = h.register(names[i], "codex", "t-"+names[i])
	}
	type outcome struct {
		polled int
		woken  bool
	}
	results := make([]outcome, agents)
	var group sync.WaitGroup
	for i := range names {
		group.Add(2)
		go func() {
			defer group.Done()
			results[i].polled = len(h.inbox(tokens[i], "?wait=1").Messages)
		}()
		go func() {
			defer group.Done()
			time.Sleep(995*time.Millisecond + time.Duration(i%10)*time.Millisecond)
			_, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: names[i], Body: "tight"})
			var out model.SendResponse
			json.Unmarshal(raw, &out)
			results[i].woken = len(out.Woken) == 1
		}()
	}
	group.Wait()
	time.Sleep(300 * time.Millisecond)
	for i, name := range names {
		unread := h.inbox(tokens[i], "?peek=1").Remaining
		if !results[i].woken && results[i].polled == 0 && unread > 0 {
			t.Errorf("%s: no wake, the poll returned nothing, and %d message is unread", name, unread)
		}
	}
}

func TestTheSecretMinimumIsSixteenCharactersExactly(t *testing.T) {
	t.Parallel()
	log, _ := store.Open(filepath.Join(t.TempDir(), "log.jsonl"))
	defer log.Close()
	if _, err := New(Config{Secret: strings.Repeat("s", 15)}, log); err == nil {
		t.Fatal("15 characters accepted")
	}
	if _, err := New(Config{Secret: strings.Repeat("s", 16)}, log); err != nil {
		t.Fatalf("16 characters refused: %v", err)
	}
}

func TestThePollLimitAndTheQueueLimitAreTheDocumentedNumbers(t *testing.T) {
	t.Parallel()
	if maxWaitSeconds != 60 || maxQueuedWakes != 200 || minSecretLength != 16 {
		t.Fatalf("wait %d queue %d secret %d", maxWaitSeconds, maxQueuedWakes, minSecretLength)
	}
}

func TestAFullWakeQueueDropsTheOldestAndKeepsTheNewest(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	alpha := h.register("alpha", "", "")
	h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: "bravo", Harness: "x", Host: "linux", Session: "s", WakeKind: "codex", WakeTarget: "t"})
	total := maxQueuedWakes + 7
	for i := 0; i < total; i++ {
		h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: fmt.Sprintf("n%d", i)})
	}
	_, raw := h.call("GET", "/v1/wakes?host=linux", secret, nil)
	var jobs []model.WakeJob
	json.Unmarshal(raw, &jobs)
	if len(jobs) != maxQueuedWakes {
		t.Fatalf("%d jobs", len(jobs))
	}
	if !strings.Contains(jobs[0].Text, "n7") || !strings.Contains(jobs[len(jobs)-1].Text, fmt.Sprintf("n%d", total-1)) {
		t.Fatalf("the queue kept the wrong end: first %q last %q", jobs[0].Text, jobs[len(jobs)-1].Text)
	}
}

func TestAnIdleAgentWithAWakeMethodIsStillListedAfterMoreThanADay(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	h.register("bravo", "codex", "t")
	h.register("charlie", "", "")
	h.mu.Lock()
	h.now = h.now.Add(30 * 24 * time.Hour)
	h.mu.Unlock()
	status, raw := h.call("GET", "/v1/agents", alpha, nil)
	var out []model.AgentInfo
	json.Unmarshal(raw, &out)
	names := map[string]bool{}
	for _, a := range out {
		names[a.Name] = true
	}
	if status != 200 || !names["alpha"] || !names["bravo"] || names["charlie"] {
		t.Fatalf("got %v", names)
	}
}

func TestTheDefaultIdleWindowIsOneDay(t *testing.T) {
	t.Parallel()
	log, _ := store.Open(filepath.Join(t.TempDir(), "log.jsonl"))
	defer log.Close()
	hub, err := New(Config{Secret: secret}, log)
	if err != nil {
		t.Fatal(err)
	}
	if hub.cfg.IdleWindow != 24*time.Hour || hub.cfg.Window != 90*time.Second {
		t.Fatalf("idle %v active %v", hub.cfg.IdleWindow, hub.cfg.Window)
	}
}

func TestTheSecretMustMatchExactlyNotByPrefixOrAsAnAgentWithAnAsParameter(t *testing.T) {
	t.Parallel()
	h := start(t)
	h.register("alpha", "", "")
	for _, token := range []string{secret + "junk", secret[:len(secret)-1], strings.ToUpper(secret)} {
		if status, _ := h.call("GET", "/v1/log", token, nil); status != 401 {
			t.Errorf("token %q: %d", token, status)
		}
	}
	if status, _ := h.call("GET", "/v1/inbox?as=alpha", secret, nil); status != 401 {
		t.Errorf("the secret with ?as= read an inbox: %d", status)
	}
}

func TestASenderHeaderIsIgnoredAndTheTokenDecides(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "", "")
	body, _ := json.Marshal(model.SendBody{To: "bravo", Body: "who sent this"})
	req, _ := http.NewRequest("POST", h.srv.URL+"/v1/send", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+alpha)
	req.Header.Set("X-Landline-From", "bravo")
	req.Header.Set("X-Sender", "bravo")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	got := h.inbox(bravo, "")
	if len(got.Messages) != 1 || got.Messages[0].From != "alpha" {
		t.Fatalf("got %+v", got.Messages)
	}
}

func TestEveryKindIsDeliveredWithTheMarkerFirst(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	bravo := h.register("bravo", "", "")
	for _, kind := range []model.Kind{"", model.KindChat, model.KindTask, model.KindReply, model.KindReview} {
		h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Kind: kind, Body: "x"})
	}
	got := h.inbox(bravo, "")
	if len(got.Messages) != 5 {
		t.Fatalf("got %d", len(got.Messages))
	}
	for _, m := range got.Messages {
		if !strings.HasPrefix(m.Text, "[landline] "+model.Marker+"\n") {
			t.Errorf("kind %q: %q", m.Kind, m.Text)
		}
	}
}

func TestASecondJSONValueInARequestIsRefused(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha", "", "")
	h.register("bravo", "", "")
	req, _ := http.NewRequest("POST", h.srv.URL+"/v1/send", strings.NewReader(`{"to":"bravo","body":"one"}{"to":"bravo","body":"two"}`))
	req.Header.Set("Authorization", "Bearer "+alpha)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if status, raw := h.call("GET", "/v1/log", secret, nil); status != 200 || strings.Contains(string(raw), "one") {
		t.Fatalf("a message from a refused request was logged: %d %s", status, raw)
	}
}

const threadID = "01a1c829-7970-7083-af01-e69ca7fb7ce5"

func (h *harness) registerSeed(seed string, extra map[string]string) (string, string) {
	h.t.Helper()
	body := map[string]string{"seed": seed, "harness": "claude-code", "host": "mac", "session": "s"}
	for k, v := range extra {
		body[k] = v
	}
	status, raw := h.call("POST", "/v1/register", secret, body)
	if status != 200 {
		h.t.Fatalf("register seed %q: %d %s", seed, status, raw)
	}
	var out model.RegisterResponse
	json.Unmarshal(raw, &out)
	return out.Name, out.Token
}

func (h *harness) agents(token string) []model.AgentInfo {
	h.t.Helper()
	_, raw := h.call("GET", "/v1/agents", token, nil)
	var out []model.AgentInfo
	json.Unmarshal(raw, &out)
	return out
}

func TestASeedGetsAFreshHandleEveryTimeAndNeverAnOldOne(t *testing.T) {
	t.Parallel()
	h := start(t)
	first, _ := h.registerSeed("threnos", nil)
	second, _ := h.registerSeed("threnos", nil)
	third, _ := h.registerSeed("Repo Tutor!", nil)
	if first != "threnos-s01" || second != "threnos-s02" || third != "repo-tutor-s03" {
		t.Fatalf("%q %q %q", first, second, third)
	}
}

func TestASeedThatSanitizesToNothingStillGetsAHandleAndALongOneIsCut(t *testing.T) {
	t.Parallel()
	h := start(t)
	bare, _ := h.registerSeed("***", nil)
	long, _ := h.registerSeed(strings.Repeat("abcdefghij", 6), nil)
	if bare != "agent-s01" || len(long) > 32 || !strings.HasSuffix(long, "-s02") {
		t.Fatalf("%q %q", bare, long)
	}
}

func TestRegisterWantsEitherANameOrASeedAndNeverBoth(t *testing.T) {
	t.Parallel()
	h := start(t)
	for _, body := range []map[string]string{
		{"harness": "x"},
		{"harness": "x", "name": "alpha", "seed": "alpha"},
	} {
		if status, _ := h.call("POST", "/v1/register", secret, body); status != 400 {
			t.Errorf("%v: %d", body, status)
		}
	}
}

func TestAttributesAreRegisteredShownInTheListAndChecked(t *testing.T) {
	t.Parallel()
	h := start(t)
	handle, token := h.registerSeed("threnos", map[string]string{"folder": "threnos", "branch": "fix/login", "harness_version": "2.1.296", "model": "claude-opus-5-5", "effort": "medium", "label": "lead"})
	var found model.AgentInfo
	for _, a := range h.agents(token) {
		if a.Name == handle {
			found = a
		}
	}
	if found.Folder != "threnos" || found.Branch != "fix/login" || found.HarnessVersion != "2.1.296" || found.Model != "claude-opus-5-5" || found.Effort != "medium" || found.Label != "lead" {
		t.Fatalf("%+v", found)
	}
	if status, _ := h.call("POST", "/v1/register", secret, map[string]string{"seed": "x", "harness": "h", "branch": "bad\nbranch"}); status != 400 {
		t.Fatalf("a branch with a line break registered: %d", status)
	}
}

func TestSendResolvesALabelAPrefixAndAnExactHandleAndRefusesAnAmbiguousName(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, sender := h.registerSeed("sender", nil)
	tutorOne, tutorToken := h.registerSeed("repo-tutor", map[string]string{"label": "tutor"})
	_, otherToken := h.registerSeed("other", map[string]string{"label": "reviewer"})
	if status, raw := h.call("POST", "/v1/send", sender, model.SendBody{To: "tutor", Body: "by label"}); status != 200 {
		t.Fatalf("label: %d %s", status, raw)
	}
	if got := h.inbox(tutorToken, ""); len(got.Messages) != 1 || got.Messages[0].To != tutorOne {
		t.Fatalf("the label did not resolve to the handle: %+v", got.Messages)
	}
	if status, _ := h.call("POST", "/v1/send", sender, model.SendBody{To: "revi", Body: "by prefix"}); status != 200 {
		t.Fatalf("prefix: %d", status)
	}
	if got := h.inbox(otherToken, ""); len(got.Messages) != 1 {
		t.Fatalf("prefix delivery: %+v", got.Messages)
	}
	if status, _ := h.call("POST", "/v1/send", sender, model.SendBody{To: tutorOne, Body: "exact"}); status != 200 {
		t.Fatalf("exact: %d", status)
	}
	h.registerSeed("repo-tutor-two", map[string]string{"label": "tutor"})
	status, raw := h.call("POST", "/v1/send", sender, model.SendBody{To: "tutor", Body: "ambiguous"})
	if status != 409 || !strings.Contains(string(raw), tutorOne) || !strings.Contains(string(raw), "Use a handle") {
		t.Fatalf("ambiguous: %d %s", status, raw)
	}
	if status, _ := h.call("POST", "/v1/send", sender, model.SendBody{To: "nobody", Body: "x"}); status != 404 {
		t.Fatalf("unknown: %d", status)
	}
}

func TestAttachUpdatesTheClaimsAndNeedsAnAgentToken(t *testing.T) {
	t.Parallel()
	h := start(t)
	handle, token := h.registerSeed("threnos", nil)
	status, raw := h.call("POST", "/v1/attach", token, model.AttachRequest{Model: "claude-opus-5-5", Effort: "high", Label: "lead"})
	var out model.AttachResponse
	json.Unmarshal(raw, &out)
	if status != 200 || !reflect.DeepEqual(out.Updated, []string{"model", "effort", "label"}) || out.Challenge || out.Attached {
		t.Fatalf("%d %s", status, raw)
	}
	var found model.AgentInfo
	for _, a := range h.agents(token) {
		if a.Name == handle {
			found = a
		}
	}
	if found.Model != "claude-opus-5-5" || found.Effort != "high" || found.Label != "lead" {
		t.Fatalf("%+v", found)
	}
	for _, bad := range []string{"", "nope", secret} {
		if status, _ := h.call("POST", "/v1/attach", bad, model.AttachRequest{Label: "x"}); status != 401 {
			t.Errorf("token %q: %d", bad, status)
		}
	}
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{Label: "bad\nlabel"}); status != 400 {
		t.Fatalf("a label with a line break: %d", status)
	}
}

func TestACodexAttachSendsACodeThroughTheWakeAndFinishesOnlyWithThatCode(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	handle, token := h.registerSeed("tutor", nil)
	status, raw := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID})
	var started model.AttachResponse
	json.Unmarshal(raw, &started)
	if status != 200 || !started.Challenge || started.Attached {
		t.Fatalf("start: %d %s", status, raw)
	}
	if h.waker.count() != 1 {
		t.Fatalf("wake calls %d", h.waker.count())
	}
	call := h.waker.calls[0]
	if !strings.HasPrefix(call, threadID+"|[landline] "+model.Marker+"\n") || !strings.Contains(call, "code ABC123") || !strings.Contains(call, handle) {
		t.Fatalf("the challenge text: %q", call)
	}
	for _, agent := range h.agents(token) {
		if agent.Name == handle && agent.WakeKind != "" {
			t.Fatal("the wake method was set before the code was confirmed")
		}
	}
	status, raw = h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "abc123"})
	var done model.AttachResponse
	json.Unmarshal(raw, &done)
	if status != 200 || !done.Attached {
		t.Fatalf("finish: %d %s", status, raw)
	}
	var found model.AgentInfo
	for _, agent := range h.agents(token) {
		if agent.Name == handle {
			found = agent
		}
	}
	if found.WakeKind != "codex" {
		t.Fatalf("%+v", found)
	}
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "ABC123"}); status != 422 {
		t.Fatalf("a code was used twice: %d", status)
	}
}

func TestAWrongCodeAWrongThreadAnExpiredCodeAndFiveTriesAllFail(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	_, token := h.registerSeed("tutor", nil)
	other := "11111111-2222-3333-4444-555555555555"
	start := func() {
		if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID}); status != 200 {
			t.Fatalf("start %d", status)
		}
	}
	start()
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "WRONG1"}); status != 422 {
		t.Fatalf("wrong code: %d", status)
	}
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: other, Code: "ABC123"}); status != 422 {
		t.Fatalf("wrong thread: %d", status)
	}
	for i := 0; i < 3; i++ {
		h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "WRONG1"})
	}
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "ABC123"}); status != 422 {
		t.Fatalf("the right code after five tries: %d", status)
	}
	start()
	h.mu.Lock()
	h.now = h.now.Add(6 * time.Minute)
	h.mu.Unlock()
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "ABC123"}); status != 422 {
		t.Fatalf("an expired code: %d", status)
	}
}

func TestACodexAttachRefusesANonUUIDAndACodeWithoutAThreadAndAFailedWakeLeavesNoChallenge(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	_, token := h.registerSeed("tutor", nil)
	for _, req := range []model.AttachRequest{{CodexThread: "not-a-uuid"}, {CodexThread: "01A1C829-7970-7083-AF01-E69CA7FB7CE5"}, {Code: "ABC123"}} {
		if status, _ := h.call("POST", "/v1/attach", token, req); status != 400 {
			t.Errorf("%+v: %d", req, status)
		}
	}
	h.waker.fail = errors.New("no such thread")
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID}); status != 502 {
		t.Fatalf("a failed wake: %d", status)
	}
	h.waker.fail = nil
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "ABC123"}); status != 422 {
		t.Fatalf("a code from a failed wake worked: %d", status)
	}
}

func TestACodexAttachForAnAgentOnAnotherHostGoesThroughThatHostsDaemon(t *testing.T) {
	t.Parallel()
	h := startOn(t, "mac")
	status, raw := h.call("POST", "/v1/register", secret, map[string]string{"seed": "tutor", "harness": "codex", "host": "linux"})
	var reg model.RegisterResponse
	json.Unmarshal(raw, &reg)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if status, raw := h.call("POST", "/v1/attach", reg.Token, model.AttachRequest{CodexThread: threadID}); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if h.waker.count() != 0 {
		t.Fatal("the hub woke a session on another host itself")
	}
	_, polled := h.call("GET", "/v1/wakes?host=linux", secret, nil)
	var jobs []model.WakeJob
	json.Unmarshal(polled, &jobs)
	if len(jobs) != 1 || jobs[0].Kind != "codex" || jobs[0].Target != threadID || !strings.Contains(jobs[0].Text, "code ABC123") || !strings.HasPrefix(jobs[0].Text, "[landline] "+model.Marker) {
		t.Fatalf("jobs %+v", jobs)
	}
}
