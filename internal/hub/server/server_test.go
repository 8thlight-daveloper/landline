package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	"unicode/utf8"

	"github.com/8thlight-daveloper/landline/internal/log/store"
	"github.com/8thlight-daveloper/landline/internal/model"
)

const secret = "0123456789abcdef-secret"

const threadID = "01a1c829-7970-7083-af01-e69ca7fb7ce5"

const waitLimit = 5 * time.Second

type harness struct {
	t     *testing.T
	srv   *httptest.Server
	hub   *Server
	notes []string
	now   time.Time
	mu    sync.Mutex
}

func start(t *testing.T) *harness { return startWith(t, func(*Config) {}) }

func startWith(t *testing.T, adjust func(*Config)) *harness {
	t.Helper()
	log, err := store.Open(filepath.Join(t.TempDir(), "log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	h := &harness{t: t, now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	var counterMu sync.Mutex
	counter, suffix := 0, 0
	cfg := Config{
		Secret: secret,
		Host:   "mac",
		Now:    func() time.Time { h.mu.Lock(); defer h.mu.Unlock(); return h.now },
		NewID: func() string {
			counterMu.Lock()
			defer counterMu.Unlock()
			counter++
			return fmt.Sprintf("m-%02d", counter)
		},
		NewCode: func() string { return "ABC123" },
		NewSuffix: func() string {
			counterMu.Lock()
			defer counterMu.Unlock()
			suffix++
			return fmt.Sprintf("s%02d", suffix)
		},
		Notify: func(s string) { h.mu.Lock(); h.notes = append(h.notes, s); h.mu.Unlock() },
	}
	adjust(&cfg)
	hub, err := New(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	h.hub = hub
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

func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	h.now = h.now.Add(d)
	h.mu.Unlock()
}

func (h *harness) notesCopy() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string{}, h.notes...)
}

func (h *harness) register(name string) string {
	h.t.Helper()
	status, raw := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: name, Harness: "codex", Host: "mac", Session: "s-" + name})
	if status != 200 {
		h.t.Fatalf("register %s: %d %s", name, status, raw)
	}
	var out model.RegisterResponse
	json.Unmarshal(raw, &out)
	return out.Token
}

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

func (h *harness) agent(token, name string) (model.AgentInfo, bool) {
	h.t.Helper()
	for _, each := range h.agents(token) {
		if each.Name == name {
			return each, true
		}
	}
	return model.AgentInfo{}, false
}

func (h *harness) waitUntil(what string, condition func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(waitLimit)
	for !condition() {
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

type streamClient struct {
	t      *testing.T
	status int
	header http.Header
	lines  chan string
	cancel context.CancelFunc
}

func (h *harness) openStream(token string) *streamClient {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", h.srv.URL+"/v1/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		h.t.Fatal(err)
	}
	client := &streamClient{t: h.t, status: resp.StatusCode, header: resp.Header, lines: make(chan string, 256), cancel: cancel}
	go func() {
		defer resp.Body.Close()
		defer close(client.lines)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			client.lines <- scanner.Text()
		}
	}()
	h.t.Cleanup(cancel)
	return client
}

func (c *streamClient) nextLine() (string, bool) {
	c.t.Helper()
	select {
	case line, open := <-c.lines:
		return line, open
	case <-time.After(waitLimit):
		c.t.Fatal("timed out waiting for a stream line")
		return "", false
	}
}

func (c *streamClient) nextDelivery() model.Delivery {
	c.t.Helper()
	var event string
	for {
		line, open := c.nextLine()
		if !open {
			c.t.Fatal("the stream closed before a delivery arrived")
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if event != "delivery" {
				c.t.Fatalf("data after event %q", event)
			}
			var out model.Delivery
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &out); err != nil {
				c.t.Fatal(err)
			}
			return out
		}
	}
}

func (c *streamClient) waitClosed() {
	c.t.Helper()
	deadline := time.After(waitLimit)
	for {
		select {
		case _, open := <-c.lines:
			if !open {
				return
			}
		case <-deadline:
			c.t.Fatal("the stream did not close")
		}
	}
}

type sendResult struct {
	status int
	raw    []byte
}

func (h *harness) sendAsync(token string, body model.SendBody) <-chan sendResult {
	result := make(chan sendResult, 1)
	go func() {
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", h.srv.URL+"/v1/send", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			result <- sendResult{status: -1, raw: []byte(err.Error())}
			return
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		result <- sendResult{status: resp.StatusCode, raw: buf.Bytes()}
	}()
	return result
}

func (h *harness) await(result <-chan sendResult) sendResult {
	h.t.Helper()
	select {
	case out := <-result:
		return out
	case <-time.After(waitLimit):
		h.t.Fatal("timed out waiting for the send to answer")
		return sendResult{}
	}
}

func (h *harness) report(token, id string, ok bool, problem string) int {
	h.t.Helper()
	status, _ := h.call("POST", "/v1/deliveries/"+id+"/report", token, model.DeliveryReport{OK: ok, Error: problem})
	return status
}

func (h *harness) deliveredSend(sender string, receiver *streamClient, receiverToken string, body model.SendBody) (model.Delivery, sendResult) {
	h.t.Helper()
	result := h.sendAsync(sender, body)
	delivery := receiver.nextDelivery()
	if status := h.report(receiverToken, delivery.ID, true, ""); status != 200 {
		h.t.Fatalf("report: %d", status)
	}
	return delivery, h.await(result)
}

func (h *harness) logged() []model.Envelope {
	h.t.Helper()
	status, raw := h.call("GET", "/v1/log", secret, nil)
	if status != 200 {
		h.t.Fatalf("log: %d %s", status, raw)
	}
	var out []model.Envelope
	json.Unmarshal(raw, &out)
	return out
}

func TestTheSecretMustBeLongEnough(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{Secret: "short"}, nil); err == nil {
		t.Fatal("accepted a short secret")
	}
}

func TestTheSecretMinimumIsSixteenCharactersExactly(t *testing.T) {
	t.Parallel()
	log, _ := store.Open(filepath.Join(t.TempDir(), "log.jsonl"))
	defer log.Close()
	if _, err := New(Config{Secret: strings.Repeat("s", 15)}, log); err == nil {
		t.Fatal("accepted 15 characters")
	}
	if _, err := New(Config{Secret: strings.Repeat("s", 16)}, log); err != nil {
		t.Fatalf("refused 16 characters: %v", err)
	}
}

func TestTheDefaultTimingsAreTheDocumentedOnes(t *testing.T) {
	t.Parallel()
	h := start(t)
	if h.hub.cfg.KeepaliveInterval != 15*time.Second || h.hub.cfg.DeliveryTimeout != 20*time.Second || h.hub.cfg.DisconnectedRetention != 10*time.Minute {
		t.Fatalf("%+v", h.hub.cfg)
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

func TestRegisterRefusesABadNameAndAWakeField(t *testing.T) {
	t.Parallel()
	h := start(t)
	if status, _ := h.call("POST", "/v1/register", secret, model.RegisterRequest{Name: "Bad Name", Harness: "codex"}); status != 400 {
		t.Errorf("bad name: status %d", status)
	}
	if status, _ := h.call("POST", "/v1/register", secret, map[string]string{"name": "alpha", "harness": "codex", "wake_kind": "codex", "wake_target": "x"}); status != 400 {
		t.Errorf("wake fields: status %d", status)
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

func TestOpeningAStreamMarksTheAgentConnectedAndListsIt(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	if found, _ := h.agent(alpha, "bravo"); found.Connected {
		t.Fatal("connected before any stream")
	}
	stream := h.openStream(bravo)
	if stream.status != 200 || !strings.HasPrefix(stream.header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status %d header %v", stream.status, stream.header)
	}
	found, listed := h.agent(alpha, "bravo")
	if !listed || !found.Connected {
		t.Fatalf("%+v", found)
	}
	if mine, _ := h.agent(alpha, "alpha"); mine.Connected {
		t.Fatal("alpha has no stream")
	}
}

func TestAStreamNeedsAnAgentToken(t *testing.T) {
	t.Parallel()
	h := start(t)
	h.register("alpha")
	for _, token := range []string{"", "nope", secret} {
		if status, _ := h.call("GET", "/v1/stream", token, nil); status != 401 {
			t.Errorf("token %q: %d", token, status)
		}
	}
}

func TestASendToAConnectedAgentIsDeliveredWithTheMarkerAndSenderStamp(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	delivery, result := h.deliveredSend(alpha, stream, bravo, model.SendBody{To: "bravo", Body: "hello"})
	if result.status != 200 {
		t.Fatalf("send: %d %s", result.status, result.raw)
	}
	var response model.SendResponse
	json.Unmarshal(result.raw, &response)
	if response.ID != "m-01" || response.Seq != 1 || !reflect.DeepEqual(response.Recipients, []string{"bravo"}) {
		t.Fatalf("%+v", response)
	}
	if delivery.Kind != model.DeliveryMessage || delivery.ID == "" || delivery.Message == nil {
		t.Fatalf("%+v", delivery)
	}
	first, _, _ := strings.Cut(delivery.Text, "\n")
	if first != "[landline] "+model.Marker {
		t.Fatalf("first line %q", first)
	}
	m := delivery.Message
	if m.From != "alpha" || m.FromInfo != "codex on mac" || m.To != "bravo" || m.Body != "hello" || m.Thread != "main" || m.Kind != model.KindChat || m.Text != delivery.Text {
		t.Fatalf("message %+v", m)
	}
	if !strings.Contains(delivery.Text, "from: alpha (codex on mac)") || !strings.HasSuffix(delivery.Text, "\n\nhello") {
		t.Fatalf("text %q", delivery.Text)
	}
}

func TestEveryKindIsDeliveredWithTheMarkerFirstAndAUniqueDeliveryID(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	seen := map[string]bool{}
	for _, kind := range []model.Kind{"", model.KindChat, model.KindTask, model.KindReply, model.KindReview} {
		delivery, result := h.deliveredSend(alpha, stream, bravo, model.SendBody{To: "bravo", Kind: kind, Body: "x"})
		if result.status != 200 {
			t.Fatalf("kind %q: %d %s", kind, result.status, result.raw)
		}
		if !strings.HasPrefix(delivery.Text, "[landline] "+model.Marker+"\n") {
			t.Errorf("kind %q: %q", kind, delivery.Text)
		}
		if seen[delivery.ID] {
			t.Errorf("delivery id %s repeated", delivery.ID)
		}
		seen[delivery.ID] = true
	}
}

func TestANotOkReportIs502WithTheReportedError(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	result := h.sendAsync(alpha, model.SendBody{To: "bravo", Body: "hi"})
	delivery := stream.nextDelivery()
	h.report(bravo, delivery.ID, false, "channel closed")
	got := h.await(result)
	if got.status != 502 || !strings.Contains(string(got.raw), `"error":"channel closed"`) {
		t.Fatalf("%d %s", got.status, got.raw)
	}
}

func TestNoReportInTimeIs504(t *testing.T) {
	t.Parallel()
	h := startWith(t, func(c *Config) { c.DeliveryTimeout = 50 * time.Millisecond })
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	result := h.sendAsync(alpha, model.SendBody{To: "bravo", Body: "hi"})
	delivery := stream.nextDelivery()
	got := h.await(result)
	if got.status != 504 || !strings.Contains(string(got.raw), `"error":"no delivery report"`) {
		t.Fatalf("%d %s", got.status, got.raw)
	}
	if status := h.report(bravo, delivery.ID, true, ""); status != 404 {
		t.Fatalf("a late report: %d", status)
	}
}

func TestASendToADisconnectedAgentIs409AndDeliversNothing(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	status, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "anyone there"})
	if status != 409 || !strings.Contains(string(raw), `"error":"not connected"`) {
		t.Fatalf("%d %s", status, raw)
	}
	stream := h.openStream(bravo)
	select {
	case line := <-stream.lines:
		if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") {
			t.Fatalf("a held message was delivered on connect: %q", line)
		}
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTheLogRecordsTheEnvelopeEvenWhenDeliveryFails(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	h.register("bravo")
	if status, _ := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "kept anyway"}); status != 409 {
		t.Fatalf("status %d", status)
	}
	log := h.logged()
	if len(log) != 1 || log[0].Body != "kept anyway" || log[0].From != "alpha" || log[0].To != "bravo" || log[0].Seq != 1 {
		t.Fatalf("%+v", log)
	}
}

func TestClosingTheStreamMarksDisconnectedAndKeepsTheAgentListedUntilTheRetentionPasses(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	h.advance(3 * time.Minute)
	h.mu.Lock()
	closedAt := h.now
	h.mu.Unlock()
	stream.cancel()
	h.waitUntil("bravo to be disconnected", func() bool {
		found, _ := h.agent(alpha, "bravo")
		return !found.Connected
	})
	found, listed := h.agent(alpha, "bravo")
	if !listed || found.Connected || !found.LastSeen.Equal(closedAt) {
		t.Fatalf("%+v listed %v, want last seen %v", found, listed, closedAt)
	}
	h.advance(9 * time.Minute)
	if _, listed := h.agent(alpha, "bravo"); !listed {
		t.Fatal("dropped before the retention passed")
	}
	h.advance(2 * time.Minute)
	if _, listed := h.agent(alpha, "bravo"); listed {
		t.Fatal("still listed after the retention passed")
	}
}

func TestAnAgentThatClosedItsStreamIsStillAddressableAndAnswers409(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	h.openStream(bravo).cancel()
	h.waitUntil("bravo to be disconnected", func() bool {
		found, listed := h.agent(alpha, "bravo")
		return listed && !found.Connected
	})
	if status, raw := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "x"}); status != 409 || !strings.Contains(string(raw), "not connected") {
		t.Fatalf("%d %s", status, raw)
	}
}

func TestASecondStreamReplacesTheFirst(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	first := h.openStream(bravo)
	second := h.openStream(bravo)
	first.waitClosed()
	if found, _ := h.agent(alpha, "bravo"); !found.Connected {
		t.Fatal("the replaced stream marked the agent disconnected")
	}
	_, result := h.deliveredSend(alpha, second, bravo, model.SendBody{To: "bravo", Body: "to the new one"})
	if result.status != 200 {
		t.Fatalf("%d %s", result.status, result.raw)
	}
}

func TestAKeepaliveCommentArrivesOnAQuietStream(t *testing.T) {
	t.Parallel()
	h := startWith(t, func(c *Config) { c.KeepaliveInterval = 10 * time.Millisecond })
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	for {
		line, open := stream.nextLine()
		if !open {
			t.Fatal("the stream closed")
		}
		if line == ": keepalive" {
			break
		}
	}
	if blank, _ := stream.nextLine(); blank != "" {
		t.Fatalf("no blank line after the comment: %q", blank)
	}
}

func TestOnlyTheAddressedAgentMayReportAndAReportCountsOnce(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	charlie := h.register("charlie")
	stream := h.openStream(bravo)
	result := h.sendAsync(alpha, model.SendBody{To: "bravo", Body: "hi"})
	delivery := stream.nextDelivery()
	if status := h.report(charlie, delivery.ID, true, ""); status != 403 {
		t.Fatalf("another agent: %d", status)
	}
	if status := h.report(alpha, delivery.ID, true, ""); status != 403 {
		t.Fatalf("the sender: %d", status)
	}
	if status := h.report(bravo, "d-unknown", true, ""); status != 404 {
		t.Fatalf("unknown id: %d", status)
	}
	if status := h.report(bravo, delivery.ID, true, ""); status != 200 {
		t.Fatalf("the addressed agent: %d", status)
	}
	if got := h.await(result); got.status != 200 {
		t.Fatalf("send %d %s", got.status, got.raw)
	}
	if status := h.report(bravo, delivery.ID, true, ""); status != 404 {
		t.Fatalf("a second report: %d", status)
	}
	if status, _ := h.call("POST", "/v1/deliveries/x/report", secret, model.DeliveryReport{OK: true}); status != 401 {
		t.Fatalf("the hub secret reported: %d", status)
	}
}

func TestTheRecipientAllIsAnUnknownRecipient(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	h.registerSeed("alligator", nil)
	stream := h.openStream(bravo)
	if status, _ := h.call("POST", "/v1/send", alpha, model.SendBody{To: "all", Body: "everyone"}); status != 404 {
		t.Fatalf("status %d", status)
	}
	select {
	case line := <-stream.lines:
		t.Fatalf("a broadcast reached bravo: %q", line)
	case <-time.After(50 * time.Millisecond):
	}
	if len(h.logged()) != 0 {
		t.Fatal("a refused send was logged")
	}
}

func TestARequestCannotNameItsOwnSender(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	h.register("bravo")
	raw := map[string]string{"to": "bravo", "body": "hi", "from": "bravo"}
	if status, _ := h.call("POST", "/v1/send", alpha, raw); status != 400 {
		t.Fatalf("status %d", status)
	}
}

func TestSendNeedsAnAgentTokenAndTheSecretIsNotOne(t *testing.T) {
	t.Parallel()
	h := start(t)
	h.register("bravo")
	for _, token := range []string{"", "nope", secret} {
		if status, _ := h.call("POST", "/v1/send", token, model.SendBody{To: "bravo", Body: "x"}); status != 401 {
			t.Errorf("token %q: status %d", token, status)
		}
	}
}

func TestRegisteringAgainRevokesTheOldToken(t *testing.T) {
	t.Parallel()
	h := start(t)
	old := h.register("alpha")
	h.register("bravo")
	fresh := h.register("alpha")
	if status, _ := h.call("POST", "/v1/send", old, model.SendBody{To: "bravo", Body: "x"}); status != 401 {
		t.Fatalf("old token status %d", status)
	}
	if status, _ := h.call("POST", "/v1/send", fresh, model.SendBody{To: "bravo", Body: "x"}); status != 409 {
		t.Fatalf("new token status %d", status)
	}
}

func TestRefusalsMapToStatuses(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	for _, c := range []struct {
		body model.SendBody
		want int
	}{
		{model.SendBody{To: "ghost", Body: "x"}, 404},
		{model.SendBody{To: "alpha", Body: "x"}, 422},
		{model.SendBody{To: "alpha", Body: ""}, 422},
		{model.SendBody{To: "alpha", Body: strings.Repeat("x", model.MaxBodyBytes+1)}, 413},
	} {
		if status, _ := h.call("POST", "/v1/send", alpha, c.body); status != c.want {
			t.Errorf("%+v: status %d, want %d", c.body, status, c.want)
		}
	}
}

func TestASenderHeaderIsIgnoredAndTheTokenDecides(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	body, _ := json.Marshal(model.SendBody{To: "bravo", Body: "who sent this"})
	req, _ := http.NewRequest("POST", h.srv.URL+"/v1/send", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+alpha)
	req.Header.Set("X-Landline-From", "bravo")
	req.Header.Set("X-Sender", "bravo")
	done := make(chan struct{})
	go func() {
		defer close(done)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	delivery := stream.nextDelivery()
	h.report(bravo, delivery.ID, true, "")
	<-done
	if delivery.Message.From != "alpha" {
		t.Fatalf("%+v", delivery.Message)
	}
}

func TestASecondJSONValueInARequestIsRefused(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	h.register("bravo")
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

func TestTheLogNeedsTheSecretAndReturnsWhatWasSent(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	h.deliveredSend(alpha, stream, bravo, model.SendBody{To: "bravo", Body: "logged"})
	if status, _ := h.call("GET", "/v1/log", alpha, nil); status != 401 {
		t.Fatalf("an agent token read the log: %d", status)
	}
	status, raw := h.call("GET", "/v1/log", secret, nil)
	if status != 200 || !strings.Contains(string(raw), "logged") {
		t.Fatalf("status %d body %s", status, raw)
	}
}

func TestTheSecretMustMatchExactlyNotByPrefix(t *testing.T) {
	t.Parallel()
	h := start(t)
	h.register("alpha")
	for _, token := range []string{secret + "junk", secret[:len(secret)-1], strings.ToUpper(secret)} {
		if status, _ := h.call("GET", "/v1/log", token, nil); status != 401 {
			t.Errorf("token %q: %d", token, status)
		}
	}
}

func TestTheRemovedRoutesAreGone(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	for _, c := range []struct{ method, path string }{
		{"GET", "/v1/inbox"}, {"GET", "/v1/wakes?host=mac"}, {"POST", "/v1/wakes/x/ack"},
	} {
		for _, token := range []string{alpha, secret} {
			if status, _ := h.call(c.method, c.path, token, nil); status != 404 && status != 405 {
				t.Errorf("%s %s: %d", c.method, c.path, status)
			}
		}
	}
}

func TestAStateWithTokensSurvivesAHubRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	open := func() (*httptest.Server, func()) {
		log, err := store.Open(filepath.Join(dir, "log.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		hub, err := New(Config{Secret: secret, StatePath: filepath.Join(dir, "state.json")}, log)
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
	var alpha model.RegisterResponse
	_, raw := call(first, "POST", "/v1/register", secret, model.RegisterRequest{Name: "alpha", Harness: "x"})
	json.Unmarshal(raw, &alpha)
	call(first, "POST", "/v1/register", secret, model.RegisterRequest{Name: "bravo", Harness: "x"})
	stop()

	second, stop := open()
	defer stop()
	status, raw := call(second, "GET", "/v1/agents", alpha.Token, nil)
	var listed []model.AgentInfo
	json.Unmarshal(raw, &listed)
	if status != 200 || len(listed) != 2 || listed[0].Connected || listed[1].Connected {
		t.Fatalf("after restart: %d %s", status, raw)
	}
	if status, _ := call(second, "POST", "/v1/send", alpha.Token, model.SendBody{To: "bravo", Body: "three"}); status != 409 {
		t.Fatalf("the old token stopped working or the agent looked connected: %d", status)
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

func TestTheStateFileKeysAreSnakeCaseAndHoldNothingAboutWakes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	h := startWith(t, func(c *Config) { c.StatePath = statePath })
	h.register("alpha")
	raw, _ := os.ReadFile(statePath)
	for _, key := range []string{`"agents"`, `"tokens"`, `"name":"alpha"`, `"last_seen"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("the state lacks %s: %s", key, raw)
		}
	}
	if strings.Contains(string(raw), "cursors") {
		t.Errorf("the state keeps read positions: %s", raw)
	}
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

func TestAttributesAreRegisteredShownInTheListAndChecked(t *testing.T) {
	t.Parallel()
	h := start(t)
	handle, token := h.registerSeed("threnos", map[string]string{"folder": "threnos", "branch": "fix/login", "harness_version": "2.1.296", "model": "claude-opus-5-5", "effort": "medium", "label": "lead"})
	found, _ := h.agent(token, handle)
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
	otherName, otherToken := h.registerSeed("other", map[string]string{"label": "reviewer"})
	tutorStream := h.openStream(tutorToken)
	otherStream := h.openStream(otherToken)
	delivery, result := h.deliveredSend(sender, tutorStream, tutorToken, model.SendBody{To: "tutor", Body: "by label"})
	if result.status != 200 || delivery.Message.To != tutorOne {
		t.Fatalf("label: %d %s %+v", result.status, result.raw, delivery.Message)
	}
	delivery, result = h.deliveredSend(sender, otherStream, otherToken, model.SendBody{To: "revi", Body: "by prefix"})
	if result.status != 200 || delivery.Message.To != otherName {
		t.Fatalf("prefix: %d %s", result.status, result.raw)
	}
	if _, result = h.deliveredSend(sender, tutorStream, tutorToken, model.SendBody{To: tutorOne, Body: "exact"}); result.status != 200 {
		t.Fatalf("exact: %d", result.status)
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
	found, _ := h.agent(token, handle)
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

type attachAnswer struct {
	status int
	raw    []byte
}

func (h *harness) attachAsync(token string, body model.AttachRequest) <-chan attachAnswer {
	answer := make(chan attachAnswer, 1)
	go func() {
		status, raw := h.call("POST", "/v1/attach", token, body)
		answer <- attachAnswer{status, raw}
	}()
	return answer
}

func awaitAttach(t *testing.T, answer <-chan attachAnswer) attachAnswer {
	t.Helper()
	select {
	case out := <-answer:
		return out
	case <-time.After(waitLimit):
		t.Fatal("timed out waiting for the attach to answer")
		return attachAnswer{}
	}
}

func (h *harness) startChallenge(token string, stream *streamClient) (model.Delivery, attachAnswer) {
	h.t.Helper()
	answer := h.attachAsync(token, model.AttachRequest{CodexThread: threadID})
	delivery := stream.nextDelivery()
	if status := h.report(token, delivery.ID, true, ""); status != 200 {
		h.t.Fatalf("report: %d", status)
	}
	return delivery, awaitAttach(h.t, answer)
}

func TestACodexChallengeGoesDownTheCallersOwnStreamAndFinishesOnlyWithThatCode(t *testing.T) {
	t.Parallel()
	h := start(t)
	handle, token := h.registerSeed("tutor", nil)
	_, otherToken := h.registerSeed("other", nil)
	other := h.openStream(otherToken)
	stream := h.openStream(token)
	delivery, started := h.startChallenge(token, stream)
	var response model.AttachResponse
	json.Unmarshal(started.raw, &response)
	if started.status != 200 || !response.Challenge || response.Attached {
		t.Fatalf("start: %d %s", started.status, started.raw)
	}
	if delivery.Kind != model.DeliveryChallenge || delivery.Target != threadID || delivery.Message != nil {
		t.Fatalf("%+v", delivery)
	}
	if !strings.HasPrefix(delivery.Text, "[landline] "+model.Marker+"\n") || !strings.Contains(delivery.Text, "code ABC123") || !strings.Contains(delivery.Text, handle) || !strings.Contains(delivery.Text, threadID) {
		t.Fatalf("the challenge text: %q", delivery.Text)
	}
	select {
	case line := <-other.lines:
		t.Fatalf("the challenge reached another agent: %q", line)
	case <-time.After(50 * time.Millisecond):
	}
	status, raw := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "abc123"})
	var done model.AttachResponse
	json.Unmarshal(raw, &done)
	if status != 200 || !done.Attached {
		t.Fatalf("finish: %d %s", status, raw)
	}
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "ABC123"}); status != 422 {
		t.Fatalf("a code was used twice: %d", status)
	}
}

func TestAProvedAttachRecordsNothingAboutTheThread(t *testing.T) {
	t.Parallel()
	statePath := filepath.Join(t.TempDir(), "state.json")
	h := startWith(t, func(c *Config) { c.StatePath = statePath })
	handle, token := h.registerSeed("tutor", nil)
	stream := h.openStream(token)
	h.startChallenge(token, stream)
	if status, raw := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "ABC123"}); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	h.hub.mu.Lock()
	agent, found := h.hub.reg.Find(h.hub.agents, handle)
	h.hub.mu.Unlock()
	if !found {
		t.Fatalf("%+v", agent)
	}
	raw, _ := os.ReadFile(statePath)
	if strings.Contains(string(raw), threadID) {
		t.Fatalf("the thread was saved: %s", raw)
	}
}

func TestACodexChallengeWithNoStreamIs409AndLeavesNoCode(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, token := h.registerSeed("tutor", nil)
	status, raw := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID})
	if status != 409 || !strings.Contains(string(raw), "not connected") {
		t.Fatalf("%d %s", status, raw)
	}
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "ABC123"}); status != 422 {
		t.Fatalf("a code from a challenge that was never delivered worked: %d", status)
	}
}

func TestAFailedChallengeReportIs502AndLeavesNoCode(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, token := h.registerSeed("tutor", nil)
	stream := h.openStream(token)
	answer := h.attachAsync(token, model.AttachRequest{CodexThread: threadID})
	delivery := stream.nextDelivery()
	h.report(token, delivery.ID, false, "no such thread")
	got := awaitAttach(t, answer)
	if got.status != 502 || !strings.Contains(string(got.raw), "no such thread") {
		t.Fatalf("%d %s", got.status, got.raw)
	}
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "ABC123"}); status != 422 {
		t.Fatalf("a code from a failed challenge worked: %d", status)
	}
}

func TestAWrongCodeAWrongThreadAnExpiredCodeAndFiveTriesAllFail(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, token := h.registerSeed("tutor", nil)
	stream := h.openStream(token)
	other := "11111111-2222-3333-4444-555555555555"
	begin := func() {
		if _, started := h.startChallenge(token, stream); started.status != 200 {
			t.Fatalf("start %d", started.status)
		}
	}
	begin()
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
	begin()
	h.advance(6 * time.Minute)
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Code: "ABC123"}); status != 422 {
		t.Fatalf("an expired code: %d", status)
	}
}

func TestACodexAttachRefusesANonUUIDAndACodeWithoutAThread(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, token := h.registerSeed("tutor", nil)
	for _, req := range []model.AttachRequest{{CodexThread: "not-a-uuid"}, {CodexThread: "01A1C829-7970-7083-AF01-E69CA7FB7CE5"}, {Code: "ABC123"}} {
		if status, _ := h.call("POST", "/v1/attach", token, req); status != 400 {
			t.Errorf("%+v: %d", req, status)
		}
	}
}

func TestAttachClaimsAreKeptWhenTheChallengeCannotBeDelivered(t *testing.T) {
	t.Parallel()
	h := start(t)
	handle, token := h.registerSeed("tutor", nil)
	status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Label: "lead"})
	if status != 409 {
		t.Fatalf("%d", status)
	}
	if found, _ := h.agent(token, handle); found.Label != "lead" {
		t.Fatalf("%+v", found)
	}
	if len(h.notesCopy()) == 0 {
		t.Fatal("the person was not told")
	}
}

func TestAStreamThatClosesWhileADeliveryWaitsAnswers409AtOnce(t *testing.T) {
	t.Parallel()
	h := startWith(t, func(c *Config) { c.DeliveryTimeout = time.Hour })
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	result := h.sendAsync(alpha, model.SendBody{To: "bravo", Body: "hi"})
	delivery := stream.nextDelivery()
	stream.cancel()
	got := h.await(result)
	if got.status != 409 || !strings.Contains(string(got.raw), `"error":"not connected"`) {
		t.Fatalf("%d %s", got.status, got.raw)
	}
	if status := h.report(bravo, delivery.ID, true, ""); status != 404 {
		t.Fatalf("a report for a lost delivery: %d", status)
	}
}

func TestAReplacedStreamDoesNotAbortADeliveryAlreadyWrittenToIt(t *testing.T) {
	t.Parallel()
	h := startWith(t, func(c *Config) { c.DeliveryTimeout = time.Hour })
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	first := h.openStream(bravo)
	result := h.sendAsync(alpha, model.SendBody{To: "bravo", Body: "hi"})
	delivery := first.nextDelivery()
	h.openStream(bravo)
	first.waitClosed()
	if status := h.report(bravo, delivery.ID, true, ""); status != 200 {
		t.Fatalf("report: %d", status)
	}
	if got := h.await(result); got.status != 200 {
		t.Fatalf("%d %s", got.status, got.raw)
	}
}

func TestADeliveryBlockedOnAStreamThatIsReplacedGoesToTheNewStream(t *testing.T) {
	t.Parallel()
	h := start(t)
	h.register("bravo")
	old := &stream{out: make(chan model.Delivery), done: make(chan struct{})}
	h.hub.mu.Lock()
	h.hub.streams["bravo"] = old
	h.hub.mu.Unlock()
	type answer struct {
		status  int
		problem string
	}
	answered := make(chan answer, 1)
	go func() {
		status, problem := h.hub.deliver(context.Background(), "bravo", model.Delivery{Kind: model.DeliveryMessage, Text: "hi"})
		answered <- answer{status, problem}
	}()
	h.waitUntil("the delivery is pending on the old stream", func() bool {
		h.hub.mu.Lock()
		defer h.hub.mu.Unlock()
		return len(h.hub.pending) == 1
	})
	replacement := &stream{out: make(chan model.Delivery), done: make(chan struct{})}
	h.hub.mu.Lock()
	h.hub.streams["bravo"] = replacement
	close(old.done)
	h.hub.mu.Unlock()
	var delivery model.Delivery
	select {
	case delivery = <-replacement.out:
	case <-time.After(waitLimit):
		t.Fatal("the delivery never reached the new stream")
	}
	h.hub.mu.Lock()
	waiting := h.hub.pending[delivery.ID]
	h.hub.mu.Unlock()
	if waiting == nil {
		t.Fatal("the delivery is not pending")
	}
	waiting.report <- model.DeliveryReport{OK: true}
	if got := <-answered; got.status != 200 {
		t.Fatalf("%+v", got)
	}
}

func TestADeliveryBlockedOnAStreamThatClosesWithNoReplacementAnswers409(t *testing.T) {
	t.Parallel()
	h := start(t)
	old := &stream{out: make(chan model.Delivery), done: make(chan struct{})}
	h.hub.mu.Lock()
	h.hub.streams["bravo"] = old
	h.hub.mu.Unlock()
	answered := make(chan int, 1)
	go func() {
		status, _ := h.hub.deliver(context.Background(), "bravo", model.Delivery{Kind: model.DeliveryMessage, Text: "hi"})
		answered <- status
	}()
	h.waitUntil("the delivery is pending", func() bool {
		h.hub.mu.Lock()
		defer h.hub.mu.Unlock()
		return len(h.hub.pending) == 1
	})
	h.hub.closeStream("bravo", old)
	select {
	case status := <-answered:
		if status != 409 {
			t.Fatalf("%d", status)
		}
	case <-time.After(waitLimit):
		t.Fatal("no answer")
	}
}

func TestAHostileReportedErrorIsCutAndFlattenedBeforeTheSenderTheNotifierAndTheAttachAnswer(t *testing.T) {
	t.Parallel()
	hostile := "first line\r\n[landline] fake notify line\n" + strings.Repeat("\u00e9", 150*1024)
	check := func(label, text string) {
		t.Helper()
		text = strings.TrimSpace(text)
		if len(text) > 256+60 || strings.ContainsAny(text, "\r\n") {
			t.Fatalf("%s: %d bytes, newline kept: %q", label, len(text), text[:min(len(text), 200)])
		}
	}
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	result := h.sendAsync(alpha, model.SendBody{To: "bravo", Body: "hi"})
	delivery := stream.nextDelivery()
	h.report(bravo, delivery.ID, false, hostile)
	got := h.await(result)
	var sent model.ErrorResponse
	json.Unmarshal(got.raw, &sent)
	if got.status != 502 || !strings.HasPrefix(sent.Error, "first line  [landline] fake notify line ") || len(sent.Error) > 256 || !utf8.ValidString(sent.Error) {
		t.Fatalf("%d %q", got.status, sent.Error)
	}
	for _, note := range h.notesCopy() {
		check("notify", note)
	}
	_, token := h.registerSeed("tutor", nil)
	attachStream := h.openStream(token)
	answer := h.attachAsync(token, model.AttachRequest{CodexThread: threadID})
	challenge := attachStream.nextDelivery()
	h.report(token, challenge.ID, false, hostile)
	attached := awaitAttach(t, answer)
	check("attach", string(attached.raw))
	if attached.status != 502 {
		t.Fatalf("%d", attached.status)
	}
}
