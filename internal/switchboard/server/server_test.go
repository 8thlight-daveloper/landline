package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	h := startNotListening(t, adjust)
	h.hub.Listening()
	return h
}

func startNotListening(t *testing.T, adjust func(*Config)) *harness {
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
		NewAgentID: func() string {
			counterMu.Lock()
			defer counterMu.Unlock()
			suffix++
			return fmt.Sprintf("a%03x", suffix)
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
	return h.registerVersioned(name, "0.2.0")
}

func (h *harness) registerVersioned(name, landlineVersion string) string {
	h.t.Helper()
	status, raw := h.call("POST", "/v1/register", secret, model.RegisterRequest{ID: name, Harness: "codex", Host: "mac", Session: "s-" + name, LandlineVersion: landlineVersion})
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
	return out.ID, out.Token
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
		if each.ID == name {
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
	if _, err := New(Config{Secret: "short"}, nil); err == nil || !strings.Contains(err.Error(), "the switchboard secret must be at least 16 characters") {
		t.Fatalf("a short secret: %v", err)
	}
}

func TestTheSecretMinimumIsSixteenCharactersExactly(t *testing.T) {
	t.Parallel()
	log, _ := store.Open(filepath.Join(t.TempDir(), "log.jsonl"))
	defer log.Close()
	if _, err := New(Config{Secret: strings.Repeat("s", 15)}, log); err == nil || !strings.Contains(err.Error(), "the switchboard secret must be at least 16 characters") {
		t.Fatalf("15 characters: %v", err)
	}
	if _, err := New(Config{Secret: strings.Repeat("s", 16)}, log); err != nil {
		t.Fatalf("refused 16 characters: %v", err)
	}
}

func TestTheDefaultTimingsAreTheDocumentedOnes(t *testing.T) {
	t.Parallel()
	h := start(t)
	if h.hub.cfg.KeepaliveInterval != 15*time.Second || h.hub.cfg.DeliveryTimeout != 20*time.Second || h.hub.cfg.Grace != 2*time.Minute {
		t.Fatalf("%+v", h.hub.cfg)
	}
}

func TestRegisterNeedsTheHubSecret(t *testing.T) {
	t.Parallel()
	h := start(t)
	if status, _ := h.call("POST", "/v1/register", "wrong", model.RegisterRequest{ID: "alpha", Harness: "codex"}); status != 401 {
		t.Fatalf("status %d", status)
	}
	if status, _ := h.call("POST", "/v1/register", "", model.RegisterRequest{ID: "alpha", Harness: "codex"}); status != 401 {
		t.Fatalf("status %d", status)
	}
}

func TestRegisterRefusesABadNameAndAWakeField(t *testing.T) {
	t.Parallel()
	h := start(t)
	if status, _ := h.call("POST", "/v1/register", secret, model.RegisterRequest{ID: "Bad Name", Harness: "codex"}); status != 400 {
		t.Errorf("bad name: status %d", status)
	}
	if status, _ := h.call("POST", "/v1/register", secret, map[string]string{"id": "alpha", "harness": "codex", "wake_kind": "codex", "wake_target": "x"}); status != 400 {
		t.Errorf("wake fields: status %d", status)
	}
}

func TestRegisterWantsEitherANameOrASeedAndNeverBoth(t *testing.T) {
	t.Parallel()
	h := start(t)
	for _, body := range []map[string]string{
		{"harness": "x"},
		{"harness": "x", "id": "alpha", "seed": "alpha"},
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

func TestClosingTheStreamMarksDisconnectedAndKeepsTheAgentListedUntilTheGracePasses(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	h.openStream(alpha)
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
	h.advance(2 * time.Minute)
	if _, listed := h.agent(alpha, "bravo"); !listed {
		t.Fatal("dropped at exactly the grace")
	}
	h.advance(time.Nanosecond)
	if _, listed := h.agent(alpha, "bravo"); listed {
		t.Fatal("still listed after the grace passed")
	}
	if status, _ := h.call("GET", "/v1/stream", bravo, nil); status != 401 {
		t.Fatalf("the token of a dropped agent answered %d", status)
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
	if status, body := h.call("GET", "/v1/log", alpha, nil); status != 401 || !strings.Contains(string(body), "missing or wrong switchboard secret") {
		t.Fatalf("an agent token read the log: %d %s", status, body)
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

func TestASeedGetsAFreshFourHexIDEveryTimeAndTheSeedTextDoesNotShapeIt(t *testing.T) {
	t.Parallel()
	h := start(t)
	first, _ := h.registerSeed("threnos", nil)
	second, _ := h.registerSeed("threnos", nil)
	third, _ := h.registerSeed("Repo Tutor!", nil)
	if first != "a001" || second != "a002" || third != "a003" {
		t.Fatalf("%q %q %q", first, second, third)
	}
}

func TestAttributesAreRegisteredShownInTheListAndChecked(t *testing.T) {
	t.Parallel()
	h := start(t)
	handle, token := h.registerSeed("threnos", map[string]string{"folder": "threnos", "branch": "fix/login", "harness_version": "2.1.296", "landline_version": "0.2.0", "model": "claude-opus-5-5", "effort": "medium", "project": "lead"})
	found, _ := h.agent(token, handle)
	if found.Folder != "threnos" || found.Branch != "fix/login" || found.HarnessVersion != "2.1.296" || found.LandlineVersion != "0.2.0" || found.Model != "claude-opus-5-5" || found.Effort != "medium" || found.Project != "lead" {
		t.Fatalf("%+v", found)
	}
	if status, _ := h.call("POST", "/v1/register", secret, map[string]string{"seed": "x", "harness": "h", "branch": "bad\nbranch"}); status != 400 {
		t.Fatalf("a branch with a line break registered: %d", status)
	}
	if status, _ := h.call("POST", "/v1/register", secret, map[string]string{"seed": "x", "harness": "h", "landline_version": "0.2\u202e.0"}); status != 400 {
		t.Fatalf("a landline version with an override character registered: %d", status)
	}
}

func TestASendToAnAgentWithNoLandlineVersionOnRecordSaysItMustBeRestarted(t *testing.T) {
	t.Parallel()
	h := start(t)
	sender := h.register("alpha")
	h.registerVersioned("old", "")
	h.registerVersioned("current", "0.2.0")
	status, raw := h.call("POST", "/v1/send", sender, model.SendBody{To: "old", Body: "x"})
	if status != 409 || !strings.Contains(string(raw), "not connected: no landline version is on record") || !strings.Contains(string(raw), "hand-registered id, which can send and cannot receive") || !strings.Contains(string(raw), "Restart its session if it is a landline session") {
		t.Fatalf("an unversioned agent: %d %s", status, raw)
	}
	status, raw = h.call("POST", "/v1/send", sender, model.SendBody{To: "current", Body: "x"})
	if status != 409 || !strings.Contains(string(raw), `"error":"not connected"`) {
		t.Fatalf("a versioned agent must get the plain message: %d %s", status, raw)
	}
	if got := h.resultOf(1); !strings.HasPrefix(got, "not connected: no landline version") {
		t.Fatalf("the log result: %q", got)
	}
}

func TestSendTakesAnExactIDAndNothingElse(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, sender := h.registerSeed("sender", nil)
	tutorID, tutorToken := h.registerSeed("repo-tutor", map[string]string{"project": "tutor"})
	tutorStream := h.openStream(tutorToken)
	if delivery, result := h.deliveredSend(sender, tutorStream, tutorToken, model.SendBody{To: tutorID, Body: "exact"}); result.status != 200 || delivery.Message.To != tutorID {
		t.Fatalf("exact: %d %s", result.status, result.raw)
	}
	for _, to := range []string{tutorID[:2], "tutor", "tut", "nobody", "all", ""} {
		if status, raw := h.call("POST", "/v1/send", sender, model.SendBody{To: to, Body: "x"}); status != 404 {
			t.Errorf("%q: %d %s", to, status, raw)
		}
	}
}

func (h *harness) errorOf(token string, to string) (int, string) {
	h.t.Helper()
	status, raw := h.call("POST", "/v1/send", token, model.SendBody{To: to, Body: "x"})
	var out model.ErrorResponse
	json.Unmarshal(raw, &out)
	return status, out.Error
}

func TestTheHintNamesTheConnectedHoldersOfAProjectOtherThanTheSenderSortedAndCutAtTen(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, outsider := h.registerSeed("outsider", nil)
	plain := model.ErrUnknownRecipient.Error()
	var ids []string
	var tokens []string
	for i := 0; i < 12; i++ {
		id, token := h.registerSeed("w", map[string]string{"project": "crew"})
		h.openStream(token)
		ids = append(ids, id)
		tokens = append(tokens, token)
	}
	sorted := append([]string{}, ids...)
	slices.Sort(sorted)
	status, text := h.errorOf(outsider, "crew")
	if want := plain + ". crew is a project, not an agent. Send to one of: " + strings.Join(sorted[:10], ", ") + ", and more"; status != 404 || text != want {
		t.Fatalf("%d %q, want %q", status, text, want)
	}
	status, text = h.errorOf(tokens[0], "crew")
	others := slices.DeleteFunc(slices.Clone(sorted), func(id string) bool { return id == ids[0] })
	if want := plain + ". crew is a project, not an agent. Send to one of: " + strings.Join(others[:10], ", ") + ", and more"; status != 404 || text != want {
		t.Fatalf("%d %q, want %q", status, text, want)
	}
	if status, text := h.errorOf(outsider, "all"); status != 404 || text != plain {
		t.Fatalf("all: %d %q", status, text)
	}
}

func TestTheHintIsLeftOutForAProjectHeldOnlyByTheSenderOrByDisconnectedAgentsAndForAnUnknownSender(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, outsider := h.registerSeed("outsider", nil)
	_, soloToken := h.registerSeed("solo", map[string]string{"project": "solo"})
	h.openStream(soloToken)
	_, ghostToken := h.registerSeed("ghost", map[string]string{"project": "ghosts"})
	h.openStream(ghostToken).cancel()
	h.waitUntil("the ghost to disconnect", func() bool {
		h.hub.mu.Lock()
		defer h.hub.mu.Unlock()
		return len(h.hub.streams) == 1
	})
	plain := model.ErrUnknownRecipient.Error()
	for name, c := range map[string]struct{ token, to string }{
		"only the sender": {soloToken, "solo"}, "only disconnected agents": {outsider, "ghosts"},
	} {
		if status, text := h.errorOf(c.token, c.to); status != 404 || text != plain {
			t.Errorf("%s: %d %q", name, status, text)
		}
	}
	if status, text := h.errorOf(outsider, "solo"); status != 404 || !strings.HasSuffix(text, "solo is a project, not an agent. Send to one of: "+h.idOf(soloToken)) {
		t.Errorf("%d %q", status, text)
	}
}

func (h *harness) idOf(token string) string {
	h.t.Helper()
	for _, each := range h.agents(token) {
		if each.Connected {
			return each.ID
		}
	}
	h.t.Fatal("no connected agent")
	return ""
}

func TestAnAttachCountsTheOtherConnectedHoldersOfTheCallersExactProject(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, first := h.registerSeed("a", map[string]string{"project": "crew"})
	h.openStream(first)
	_, second := h.registerSeed("b", map[string]string{"project": "crew"})
	_, ghost := h.registerSeed("c", map[string]string{"project": "crew"})
	h.openStream(ghost).cancel()
	_, bare := h.registerSeed("d", nil)
	h.waitUntil("the ghost to disconnect", func() bool {
		h.hub.mu.Lock()
		defer h.hub.mu.Unlock()
		return len(h.hub.streams) == 1
	})
	peers := func(token string, request model.AttachRequest) int {
		status, raw := h.call("POST", "/v1/attach", token, request)
		var out model.AttachResponse
		json.Unmarshal(raw, &out)
		if status != 200 || !strings.Contains(string(raw), `"project_peers"`) {
			t.Fatalf("%d %s", status, raw)
		}
		return out.ProjectPeers
	}
	if got := peers(second, model.AttachRequest{Model: "m"}); got != 1 {
		t.Errorf("a project with one connected peer: %d", got)
	}
	if got := peers(first, model.AttachRequest{Model: "m"}); got != 0 {
		t.Errorf("the caller is not counted: %d", got)
	}
	if got := peers(bare, model.AttachRequest{Model: "m"}); got != 0 {
		t.Errorf("no project: %d", got)
	}
	if got := peers(bare, model.AttachRequest{Project: "crew"}); got != 1 {
		t.Errorf("the project claimed in the same call: %d", got)
	}
}

func TestStatusSetsAndClearsTheTaskStampsItWithTheClockAndKeepsTheTimeForTheSameText(t *testing.T) {
	t.Parallel()
	h := start(t)
	id, token := h.registerSeed("a", nil)
	set := func(body any) (int, model.StatusResponse) {
		status, raw := h.call("POST", "/v1/status", token, body)
		var out model.StatusResponse
		json.Unmarshal(raw, &out)
		return status, out
	}
	status, first := set(map[string]string{"task": "ship ADR 0160"})
	if status != 200 || first.Task != "ship ADR 0160" || !first.TaskAt.Equal(h.now) {
		t.Fatalf("set: %d %+v", status, first)
	}
	h.advance(time.Minute)
	if status, same := set(map[string]string{"task": "ship ADR 0160"}); status != 200 || !same.TaskAt.Equal(first.TaskAt) {
		t.Fatalf("the same text: %d %+v", status, same)
	}
	h.advance(time.Minute)
	if status, changed := set(map[string]string{"task": "other"}); status != 200 || !changed.TaskAt.Equal(h.now) {
		t.Fatalf("a change: %d %+v", status, changed)
	}
	if listed, _ := h.agent(token, id); listed.Task != "other" || !listed.TaskAt.Equal(h.now) {
		t.Fatalf("the list: %+v", listed)
	}
	status, raw := h.call("POST", "/v1/status", token, map[string]string{"task": ""})
	if status != 200 || string(bytes.TrimSpace(raw)) != `{"task":""}` {
		t.Fatalf("clear: %d %s", status, raw)
	}
	_, rawList := h.call("GET", "/v1/agents", token, nil)
	if strings.Contains(string(rawList), "task") {
		t.Fatalf("a cleared task is listed: %s", rawList)
	}
}

func TestStatusRefusesABodyThatIsNotExactlyAStringTaskAndATokenThatIsNotAnAgents(t *testing.T) {
	t.Parallel()
	h := start(t)
	id, token := h.registerSeed("a", nil)
	for name, body := range map[string]any{
		"no task": map[string]any{}, "null": map[string]any{"task": nil}, "a number": map[string]any{"task": 5},
		"an unknown field": map[string]any{"task": "x", "extra": 1}, "65 bytes": map[string]any{"task": strings.Repeat("a", 65)},
		"a line break": map[string]any{"task": "a\nb"},
	} {
		if status, raw := h.call("POST", "/v1/status", token, body); status != 400 {
			t.Errorf("%s: %d %s", name, status, raw)
		}
	}
	if listed, _ := h.agent(token, id); listed.Task != "" {
		t.Fatalf("a refused call set %q", listed.Task)
	}
	observer := h.registerObserver("tui")
	if status, _ := h.call("POST", "/v1/status", observer, map[string]string{"task": "x"}); status != 403 {
		t.Errorf("observer: %d", status)
	}
	for _, bad := range []string{"", "unknown", secret} {
		if status, _ := h.call("POST", "/v1/status", bad, map[string]string{"task": "x"}); status != 401 {
			t.Errorf("%q: %d", bad, status)
		}
	}
}

func TestARegistrationByIDClearsTheTask(t *testing.T) {
	t.Parallel()
	h := start(t)
	token := h.registerVersioned("alpha", "0.2.0")
	h.call("POST", "/v1/status", token, map[string]string{"task": "x"})
	h.registerVersioned("alpha", "0.2.0")
	if listed, _ := h.agent(h.registerVersioned("bravo", "0.2.0"), "alpha"); listed.Task != "" || !listed.TaskAt.IsZero() {
		t.Fatalf("%+v", listed)
	}
}

func TestAttachUpdatesTheClaimsAndNeedsAnAgentToken(t *testing.T) {
	t.Parallel()
	h := start(t)
	handle, token := h.registerSeed("threnos", nil)
	status, raw := h.call("POST", "/v1/attach", token, model.AttachRequest{Model: "claude-opus-5-5", Effort: "high", Project: "lead"})
	var out model.AttachResponse
	json.Unmarshal(raw, &out)
	if status != 200 || !reflect.DeepEqual(out.Updated, []string{"model", "effort", "project"}) || out.Challenge || out.Attached {
		t.Fatalf("%d %s", status, raw)
	}
	found, _ := h.agent(token, handle)
	if found.Model != "claude-opus-5-5" || found.Effort != "high" || found.Project != "lead" {
		t.Fatalf("%+v", found)
	}
	for _, bad := range []string{"", "nope", secret} {
		if status, _ := h.call("POST", "/v1/attach", bad, model.AttachRequest{Project: "x"}); status != 401 {
			t.Errorf("token %q: %d", bad, status)
		}
	}
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{Project: "bad\nlabel"}); status != 400 {
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
	h := start(t)
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
	if raw, _ := json.Marshal(agent); strings.Contains(string(raw), threadID) {
		t.Fatalf("the thread was recorded: %s", raw)
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
	status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID, Project: "lead"})
	if status != 409 {
		t.Fatalf("%d", status)
	}
	if found, _ := h.agent(token, handle); found.Project != "lead" {
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

func (h *harness) challengeStatus(token string, stream *streamClient, ok bool) int {
	h.t.Helper()
	answer := h.attachAsync(token, model.AttachRequest{CodexThread: threadID})
	delivery := stream.nextDelivery()
	if status := h.report(token, delivery.ID, ok, "nope"); status != 200 {
		h.t.Fatalf("report: %d", status)
	}
	return awaitAttach(h.t, answer).status
}

func (h *harness) assertNothingSent(stream *streamClient) {
	h.t.Helper()
	deadline := time.After(100 * time.Millisecond)
	for {
		select {
		case line := <-stream.lines:
			if line != "" {
				h.t.Fatalf("the hub sent %q", line)
			}
		case <-deadline:
			return
		}
	}
}

func TestOnlyFiveProofChallengesPerAgentPerWindowAreSentAndTheSixthIs429WithNothingSent(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, token := h.registerSeed("tutor", nil)
	stream := h.openStream(token)
	for i := 1; i <= 5; i++ {
		if status := h.challengeStatus(token, stream, true); status != 200 {
			t.Fatalf("challenge %d: %d", i, status)
		}
	}
	status, raw := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID})
	if status != 429 || strings.TrimSpace(string(raw)) != `{"error":"too many proof challenges, try again later"}` {
		t.Fatalf("%d %s", status, raw)
	}
	h.assertNothingSent(stream)
}

func TestFailedChallengesCountTowardTheCap(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, token := h.registerSeed("tutor", nil)
	stream := h.openStream(token)
	for i := 1; i <= 5; i++ {
		if status := h.challengeStatus(token, stream, false); status != 502 {
			t.Fatalf("challenge %d: %d", i, status)
		}
	}
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID}); status != 429 {
		t.Fatalf("%d", status)
	}
}

func TestTheChallengeWindowPassingAllowsChallengesAgain(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, token := h.registerSeed("tutor", nil)
	stream := h.openStream(token)
	for i := 0; i < 5; i++ {
		h.challengeStatus(token, stream, true)
	}
	h.advance(5*time.Minute - time.Second)
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID}); status != 429 {
		t.Fatalf("just inside the window: %d", status)
	}
	h.advance(time.Second)
	if status := h.challengeStatus(token, stream, true); status != 200 {
		t.Fatalf("after the window: %d", status)
	}
}

func TestEachAgentHasItsOwnChallengeCount(t *testing.T) {
	t.Parallel()
	h := start(t)
	_, tutor := h.registerSeed("tutor", nil)
	_, coach := h.registerSeed("coach", nil)
	tutorStream := h.openStream(tutor)
	coachStream := h.openStream(coach)
	for i := 0; i < 5; i++ {
		h.challengeStatus(tutor, tutorStream, true)
	}
	if status, _ := h.call("POST", "/v1/attach", tutor, model.AttachRequest{CodexThread: threadID}); status != 429 {
		t.Fatalf("%d", status)
	}
	if status := h.challengeStatus(coach, coachStream, true); status != 200 {
		t.Fatalf("another agent was refused: %d", status)
	}
}

func TestTheChallengeLimitAndWindowCanBeConfigured(t *testing.T) {
	t.Parallel()
	h := startWith(t, func(c *Config) { c.ChallengeLimit = 1; c.ChallengeWindow = time.Minute })
	_, token := h.registerSeed("tutor", nil)
	stream := h.openStream(token)
	h.challengeStatus(token, stream, true)
	if status, _ := h.call("POST", "/v1/attach", token, model.AttachRequest{CodexThread: threadID}); status != 429 {
		t.Fatalf("%d", status)
	}
	h.advance(time.Minute)
	if status := h.challengeStatus(token, stream, true); status != 200 {
		t.Fatalf("%d", status)
	}
}

func (h *harness) resultOf(seq int64) string {
	h.t.Helper()
	for _, e := range h.logged() {
		if e.Seq == seq {
			return e.Result
		}
	}
	h.t.Fatalf("no message %d in the log", seq)
	return ""
}

func TestTheLogRecordsWhatBecameOfEachSend(t *testing.T) {
	t.Parallel()
	h := startWith(t, func(c *Config) { c.DeliveryTimeout = 300 * time.Millisecond })
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	charlie := h.register("charlie")
	delta := h.register("delta")
	echo := h.register("echo")

	h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "offline"})
	if got := h.resultOf(1); got != "not connected" {
		t.Fatalf("409: %q", got)
	}

	bravoStream := h.openStream(bravo)
	if _, sent := h.deliveredSend(alpha, bravoStream, bravo, model.SendBody{To: "bravo", Body: "ok"}); sent.status != 200 {
		t.Fatalf("%d", sent.status)
	}
	if got := h.resultOf(2); got != "delivered" {
		t.Fatalf("200: %q", got)
	}

	charlieStream := h.openStream(charlie)
	pending := h.sendAsync(alpha, model.SendBody{To: "charlie", Body: "bad"})
	delivery := charlieStream.nextDelivery()
	h.report(charlie, delivery.ID, false, "line one\nline two")
	if got := h.await(pending); got.status != 502 {
		t.Fatalf("%d", got.status)
	}
	if got := h.resultOf(3); got != "line one line two" {
		t.Fatalf("502: %q", got)
	}

	deltaStream := h.openStream(delta)
	pending = h.sendAsync(alpha, model.SendBody{To: "delta", Body: "silent"})
	deltaStream.nextDelivery()
	if got := h.await(pending); got.status != 504 {
		t.Fatalf("%d", got.status)
	}
	if got := h.resultOf(4); got != "no delivery report" {
		t.Fatalf("504: %q", got)
	}

	echoStream := h.openStream(echo)
	pending = h.sendAsync(alpha, model.SendBody{To: "echo", Body: "second"})
	second := echoStream.nextDelivery()
	h.report(echo, second.ID, true, "")
	h.await(pending)
	if h.resultOf(5) != "delivered" || h.resultOf(2) != "delivered" || h.resultOf(3) != "line one line two" {
		t.Fatalf("results landed on the wrong messages: %+v", h.logged())
	}
}

func TestAHostileReportedErrorIsStoredAsOneLogEntryResult(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "log.jsonl")
	log, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	hub, err := New(Config{Secret: secret}, log)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(hub.Handler())
	t.Cleanup(server.Close)
	h := &harness{t: t, srv: server, hub: hub}
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	pending := h.sendAsync(alpha, model.SendBody{To: "bravo", Body: "hi"})
	delivery := stream.nextDelivery()
	h.report(bravo, delivery.ID, false, "x\"}\n{\"seq\":9,\"id\":\"forged\",\"result\":\"delivered\"}")
	h.await(pending)
	raw, _ := os.ReadFile(path)
	if lines := strings.Split(strings.TrimSpace(string(raw)), "\n"); len(lines) != 2 {
		t.Fatalf("%q", lines)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("%v", info.Mode())
	}
}

type failingResultStore struct{ *store.Store }

func (failingResultStore) Result(int64, string) error { return fmt.Errorf("disk full") }

func TestAFailureToRecordTheResultIsReportedAndTheSendAnswerStandsUnchanged(t *testing.T) {
	t.Parallel()
	log, err := store.Open(filepath.Join(t.TempDir(), "log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	h := &harness{t: t}
	hub, err := New(Config{Secret: secret, Notify: func(s string) { h.mu.Lock(); h.notes = append(h.notes, s); h.mu.Unlock() }}, failingResultStore{log})
	if err != nil {
		t.Fatal(err)
	}
	h.hub = hub
	h.srv = httptest.NewServer(hub.Handler())
	t.Cleanup(h.srv.Close)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)
	if _, sent := h.deliveredSend(alpha, stream, bravo, model.SendBody{To: "bravo", Body: "hi"}); sent.status != 200 {
		t.Fatalf("%d %s", sent.status, sent.raw)
	}
	if notes := h.notesCopy(); len(notes) != 1 || !strings.Contains(notes[0], "disk full") {
		t.Fatalf("%q", notes)
	}
}

func (h *harness) waitDisconnected(viewer, name string) {
	h.t.Helper()
	h.waitUntil(name+" to be disconnected", func() bool {
		found, listed := h.agent(viewer, name)
		return listed && !found.Connected
	})
}

func TestAnAgentWithAnOpenStreamIsKeptHoweverLongItStaysQuiet(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	h.openStream(alpha)
	h.openStream(bravo)
	h.advance(time.Hour)
	found, listed := h.agent(alpha, "bravo")
	if !listed || !found.Connected {
		t.Fatalf("%+v %v", found, listed)
	}
}

func TestAnAgentThatReconnectsWithinTheGraceKeepsItsHandleAndToken(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	handle, token := h.registerSeed("tutor", nil)
	h.openStream(alpha)
	h.openStream(token).cancel()
	h.waitDisconnected(alpha, handle)
	h.advance(2 * time.Minute)
	again := h.openStream(token)
	if again.status != 200 {
		t.Fatalf("the stream answered %d", again.status)
	}
	h.waitUntil(handle+" to be connected", func() bool {
		found, listed := h.agent(alpha, handle)
		return listed && found.Connected
	})
}

func TestAnAgentPastTheGraceIsDroppedItsTokenAnswers401AndItsHandleMayBeIssuedAgain(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	h.openStream(alpha)
	bravo := h.register("bravo")
	h.openStream(bravo).cancel()
	h.waitDisconnected(alpha, "bravo")
	h.advance(2*time.Minute + time.Nanosecond)
	if _, listed := h.agent(alpha, "bravo"); listed {
		t.Fatal("bravo is still listed")
	}
	if status, _ := h.call("GET", "/v1/stream", bravo, nil); status != 401 {
		t.Fatalf("the token answered %d", status)
	}
	if status, _ := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "x"}); status != 404 {
		t.Fatalf("a send to a dropped agent answered %d", status)
	}
	if status, raw := h.call("POST", "/v1/register", secret, model.RegisterRequest{ID: "bravo", Harness: "codex"}); status != 200 {
		t.Fatalf("the handle was not free: %d %s", status, raw)
	}
}

func TestADroppedAgentIsDroppedAtTheFirstRequestOfAnyKind(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	h.openStream(alpha)
	h.register("bravo")
	h.advance(2*time.Minute + time.Nanosecond)
	h.call("GET", "/v1/log", secret, nil)
	h.hub.mu.Lock()
	_, kept := h.hub.reg.Find(h.hub.agents, "bravo")
	tokens := len(h.hub.tokens)
	h.hub.mu.Unlock()
	if kept || tokens != 1 {
		t.Fatalf("bravo kept %v, tokens %d", kept, tokens)
	}
}

func TestAHandRegisteredAgentIsKeptByItsRequestsAndDroppedAfterTheGraceWithNone(t *testing.T) {
	t.Parallel()
	h := start(t)
	viewer := h.register("viewer")
	h.openStream(viewer)
	script := h.register("script")
	for i := 0; i < 4; i++ {
		h.advance(time.Minute + 30*time.Second)
		if status, _ := h.call("GET", "/v1/agents", script, nil); status != 200 {
			t.Fatalf("request %d answered %d", i, status)
		}
	}
	if _, listed := h.agent(viewer, "script"); !listed {
		t.Fatal("dropped while it kept making requests")
	}
	h.advance(2*time.Minute + time.Nanosecond)
	if _, listed := h.agent(viewer, "script"); listed {
		t.Fatal("still listed after the grace with no request")
	}
	if status, _ := h.call("GET", "/v1/agents", script, nil); status != 401 {
		t.Fatalf("the token answered %d", status)
	}
}

func TestTheIDOfAnAgentInItsGraceAnswers409NotConnectedAndAForgottenIDAnswers404(t *testing.T) {
	t.Parallel()
	h := start(t)
	sender := h.register("sender")
	h.openStream(sender)
	oldID, oldToken := h.registerSeed("old", map[string]string{"project": "tutor"})
	h.openStream(oldToken).cancel()
	h.waitDisconnected(sender, oldID)
	if status, raw := h.call("POST", "/v1/send", sender, model.SendBody{To: oldID, Body: "x"}); status != 409 || !strings.Contains(string(raw), "not connected") {
		t.Fatalf("%d %s", status, raw)
	}
	if status, _ := h.call("POST", "/v1/send", sender, model.SendBody{To: "tutor", Body: "x"}); status != 404 {
		t.Fatalf("a project held only by a disconnected agent: %d", status)
	}
	h.advance(2*time.Minute + time.Nanosecond)
	if status, _ := h.call("POST", "/v1/send", sender, model.SendBody{To: oldID, Body: "x"}); status != 404 {
		t.Fatalf("a forgotten id: %d", status)
	}
}

func TestAnIDIsNotIssuedWhileAPresentAgentHoldsItAndIsIssuedOnceItIsGone(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	draws := []string{"aaaa", "aaaa", "bbbb", "aaaa"}
	h := startWith(t, func(c *Config) {
		c.NewAgentID = func() string {
			mu.Lock()
			defer mu.Unlock()
			next := draws[0]
			draws = draws[1:]
			return next
		}
	})
	first, firstToken := h.registerSeed("tutor", nil)
	if first != "aaaa" {
		t.Fatal(first)
	}
	h.openStream(firstToken).cancel()
	h.waitUntil("the stream to close", func() bool {
		h.hub.mu.Lock()
		defer h.hub.mu.Unlock()
		return h.hub.streams[first] == nil
	})
	second, secondToken := h.registerSeed("tutor", nil)
	if second != "bbbb" {
		t.Fatalf("an id held by an agent in its grace was issued again: %s", second)
	}
	h.advance(2*time.Minute + time.Nanosecond)
	h.call("GET", "/v1/agents", secondToken, nil)
	third, _ := h.registerSeed("tutor", nil)
	if third != "aaaa" {
		t.Fatalf("a forgotten id was not free: %s", third)
	}
}

func (h *harness) registerObserver(kind string) string {
	h.t.Helper()
	status, raw := h.call("POST", "/v1/observers", secret, ObserverRequest{Kind: kind})
	if status != 200 {
		h.t.Fatalf("register observer: %d %s", status, raw)
	}
	var out ObserverResponse
	json.Unmarshal(raw, &out)
	if out.Token == "" {
		h.t.Fatal("empty observer token")
	}
	return out.Token
}

func TestObserverRegistrationNeedsTheHubSecretAndAValidKind(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	for _, token := range []string{"", "wrong", alpha} {
		if status, _ := h.call("POST", "/v1/observers", token, ObserverRequest{Kind: "tui"}); status != 401 {
			t.Fatalf("token %q answered %d", token, status)
		}
	}
	token := h.registerObserver("tui")
	if status, _ := h.call("POST", "/v1/observers", token, ObserverRequest{Kind: "tui"}); status != 403 {
		t.Fatalf("an observer registering an observer answered %d", status)
	}
	bad := map[string]string{
		"empty": "", "blank": "  \t", "long": strings.Repeat("k", 65), "control": "t\nui",
		"line separator": "t ui", "format": "t​ui",
	}
	for name, kind := range bad {
		if status, _ := h.call("POST", "/v1/observers", secret, ObserverRequest{Kind: kind}); status != 400 {
			t.Fatalf("%s kind answered %d", name, status)
		}
	}
	if status, _ := h.call("POST", "/v1/observers", secret, map[string]string{}); status != 400 {
		t.Fatalf("a missing kind answered %d", status)
	}
	if status, _ := h.call("POST", "/v1/observers", secret, ObserverRequest{Kind: strings.Repeat("k", 64)}); status != 200 {
		t.Fatalf("a 64 byte kind answered %d", status)
	}
}

func TestAnObserverIsNeverListedAsAnAgent(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	h.openStream(alpha)
	observer := h.registerObserver("tui")
	for _, token := range []string{alpha, observer} {
		listed := h.agents(token)
		if len(listed) != 1 || listed[0].ID != "alpha" || !listed[0].Connected {
			t.Fatalf("listed %+v", listed)
		}
	}
	_, raw := h.call("GET", "/v1/agents", observer, nil)
	_, agentRaw := h.call("GET", "/v1/agents", alpha, nil)
	if string(raw) != string(agentRaw) {
		t.Fatalf("observer saw %s, agent saw %s", raw, agentRaw)
	}
}

func TestAnObserverTokenIsRefusedOnEveryOtherRoute(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	h.openStream(alpha)
	h.register("bravo")
	observer := h.registerObserver("tui")
	routes := []struct {
		method, path string
		body         any
	}{
		{"POST", "/v1/send", model.SendBody{To: "alpha", Body: "x"}},
		{"GET", "/v1/stream", nil},
		{"POST", "/v1/attach", model.AttachRequest{}},
		{"POST", "/v1/deliveries/d-1/report", model.DeliveryReport{OK: true}},
		{"GET", "/v1/log", nil},
		{"POST", "/v1/register", model.RegisterRequest{ID: "charlie", Harness: "codex"}},
		{"POST", "/v1/observers", ObserverRequest{Kind: "tui"}},
	}
	for _, route := range routes {
		status, raw := h.call(route.method, route.path, observer, route.body)
		var problem model.ErrorResponse
		json.Unmarshal(raw, &problem)
		if status != 403 || !strings.Contains(problem.Error, "read-only") {
			t.Fatalf("%s %s answered %d %s", route.method, route.path, status, raw)
		}
	}
	if len(h.logged()) != 0 {
		t.Fatal("a refused observer wrote to the log")
	}
	if len(h.agents(alpha)) != 2 {
		t.Fatal("a refused observer changed the agents")
	}
	for _, route := range routes[:5] {
		if status, _ := h.call(route.method, route.path, "unknown-token", route.body); status != 401 {
			t.Fatalf("%s %s answered %d to an unknown token", route.method, route.path, status)
		}
	}
}

func TestALeakedObserverTokenCannotSend(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	receiver := h.openStream(alpha)
	observer := h.registerObserver("tui")
	if status, _ := h.call("POST", "/v1/send", observer, model.SendBody{To: "alpha", Body: "do it"}); status != 403 {
		t.Fatalf("status %d", status)
	}
	if len(h.logged()) != 0 {
		t.Fatal("the message was logged")
	}
	select {
	case line := <-receiver.lines:
		t.Fatalf("the stream got %q", line)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAnObserverIsKeptForExactlyTheGraceAndDroppedAfterIt(t *testing.T) {
	t.Parallel()
	h := start(t)
	observer := h.registerObserver("tui")
	h.advance(2 * time.Minute)
	if status, _ := h.call("GET", "/v1/agents", observer, nil); status != 200 {
		t.Fatalf("at the grace the token answered %d", status)
	}
	h.advance(2*time.Minute + time.Nanosecond)
	if status, _ := h.call("GET", "/v1/agents", observer, nil); status != 401 {
		t.Fatalf("past the grace the token answered %d", status)
	}
	if status, _ := h.call("GET", "/v1/agents", observer, nil); status != 401 {
		t.Fatalf("the dropped token answered %d", status)
	}
	h.hub.mu.Lock()
	held := len(h.hub.observers)
	h.hub.mu.Unlock()
	if held != 0 {
		t.Fatalf("%d observers held", held)
	}
}

func TestAnExpiredObserverIsDroppedAtTheFirstRequestOfAnyKindAndANewOneWorks(t *testing.T) {
	t.Parallel()
	h := start(t)
	observer := h.registerObserver("tui")
	h.advance(2*time.Minute + time.Nanosecond)
	h.call("GET", "/v1/log", secret, nil)
	h.hub.mu.Lock()
	held := len(h.hub.observers)
	h.hub.mu.Unlock()
	if held != 0 {
		t.Fatal("the observer was kept")
	}
	if status, _ := h.call("GET", "/v1/agents", observer, nil); status != 401 {
		t.Fatalf("status %d", status)
	}
	fresh := h.registerObserver("tui")
	if status, _ := h.call("GET", "/v1/agents", fresh, nil); status != 200 {
		t.Fatalf("status %d", status)
	}
}

func TestEveryRequestRefreshesAnObserver(t *testing.T) {
	t.Parallel()
	h := start(t)
	observer := h.registerObserver("tui")
	for i := 0; i < 3; i++ {
		h.advance(90 * time.Second)
		if status, _ := h.call("GET", "/v1/agents", observer, nil); status != 200 {
			t.Fatalf("round %d answered %d", i, status)
		}
	}
	h.advance(90 * time.Second)
	if status, _ := h.call("POST", "/v1/send", observer, model.SendBody{To: "x", Body: "x"}); status != 403 {
		t.Fatalf("a refused request answered %d", status)
	}
	h.advance(90 * time.Second)
	if status, _ := h.call("GET", "/v1/agents", observer, nil); status != 200 {
		t.Fatalf("a refused request did not refresh: %d", status)
	}
}

func TestAnAgentTokenSentToASecretRouteRefreshesNothing(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	for _, route := range []string{"/v1/log", "/v1/register", "/v1/observers"} {
		method := "POST"
		if route == "/v1/log" {
			method = "GET"
		}
		h.advance(time.Minute + 30*time.Second)
		if status, _ := h.call(method, route, alpha, nil); status != 401 {
			t.Fatalf("%s answered %d", route, status)
		}
	}
	h.advance(time.Minute + 30*time.Second)
	if status, _ := h.call("GET", "/v1/agents", alpha, nil); status != 401 {
		t.Fatalf("requests refused on secret routes kept the agent alive: %d", status)
	}
}

func TestObserversAreIndependentAndAgentsAreUnaffected(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	h.openStream(alpha)
	first := h.registerObserver("tui")
	h.advance(time.Minute)
	second := h.registerObserver("web")
	if first == second {
		t.Fatal("two observers share a token")
	}
	h.advance(time.Minute + time.Nanosecond)
	if status, _ := h.call("GET", "/v1/agents", first, nil); status != 401 {
		t.Fatalf("the first observer answered %d", status)
	}
	if status, _ := h.call("GET", "/v1/agents", second, nil); status != 200 {
		t.Fatalf("the second observer answered %d", status)
	}
	if status, _ := h.call("GET", "/v1/agents", alpha, nil); status != 200 {
		t.Fatalf("the agent answered %d", status)
	}
	if status, _ := h.call("GET", "/v1/agents", secret, nil); status != 401 {
		t.Fatalf("the hub secret answered %d on an agent route", status)
	}
	h.register("bravo")
	if status, _ := h.call("POST", "/v1/send", alpha, model.SendBody{To: "bravo", Body: "x"}); status != 409 {
		t.Fatalf("an agent send answered %d", status)
	}
}

func TestAHubRestartForgetsObservers(t *testing.T) {
	t.Parallel()
	h := start(t)
	observer := h.registerObserver("tui")
	restarted := start(t)
	if status, _ := restarted.call("GET", "/v1/agents", observer, nil); status != 401 {
		t.Fatalf("status %d", status)
	}
}

func TestARedeemWhoseKeyIsNotCleanBase64Gets400AndKeepsTheInvite(t *testing.T) {
	h := start(t)
	status, raw := h.call("POST", "/v1/machines", secret, map[string]string{"name": "alpha"})
	if status != 200 {
		t.Fatalf("invite: %d %s", status, raw)
	}
	var made struct{ Code string }
	json.Unmarshal(raw, &made)
	good := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if status, _ := h.call("PATCH", "/v1/machines", "", map[string]string{"code": made.Code, "public_key": good + "!!!"}); status != 400 {
		t.Fatalf("garbage after a full key: %d", status)
	}
	if status, raw := h.call("PATCH", "/v1/machines", "", map[string]string{"code": made.Code, "public_key": good}); status != 200 {
		t.Fatalf("the invite was used up: %d %s", status, raw)
	}
}
