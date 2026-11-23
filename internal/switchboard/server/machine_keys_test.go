package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/log/store"
)

type settableClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *settableClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *settableClock) set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

func (h *harness) joinMachine(name string) ed25519.PrivateKey {
	h.t.Helper()
	status, raw := h.call("POST", "/v1/machines", secret, map[string]string{"name": name})
	if status != http.StatusOK {
		h.t.Fatalf("invite %s: %d %s", name, status, raw)
	}
	var made struct{ Code string }
	json.Unmarshal(raw, &made)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		h.t.Fatal(err)
	}
	if status, raw := h.call("PATCH", "/v1/machines", "", map[string]string{"code": made.Code, "public_key": base64.StdEncoding.EncodeToString(public)}); status != http.StatusOK {
		h.t.Fatalf("redeem %s: %d %s", name, status, raw)
	}
	return private
}

func newTestServer(t *testing.T, hub *Server) *httptest.Server {
	server := httptest.NewServer(hub.Handler())
	t.Cleanup(server.Close)
	return server
}

func randomNonce() string {
	raw := make([]byte, 16)
	rand.Read(raw)
	return base64.StdEncoding.EncodeToString(raw)
}

func machineAuthorization(key ed25519.PrivateKey, name string, unix int64, method, path string, body []byte) string {
	nonce := randomNonce()
	sum := sha256.Sum256(body)
	text := strings.Join([]string{"landline-machine-v1", name, strconv.FormatInt(unix, 10), nonce, method, path, hex.EncodeToString(sum[:])}, "\n")
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(text)))
	return fmt.Sprintf("Landline-Machine name=%s, time=%d, nonce=%s, signature=%s", name, unix, nonce, signature)
}

func (h *harness) sendWithAuthorization(method, path, authorization string, body []byte) (int, []byte) {
	req, _ := http.NewRequest(method, h.srv.URL+path, bytes.NewReader(body))
	req.Header.Set("Authorization", authorization)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func (h *harness) signedRegister(key ed25519.PrivateKey, machine, id string) string {
	h.t.Helper()
	body := []byte(`{"id":"` + id + `","harness":"claude-code","host":"mac"}`)
	status, raw := h.sendWithAuthorization("POST", "/v1/register", machineAuthorization(key, machine, h.now.Unix(), "POST", "/v1/register", body), body)
	var answer struct{ Token string }
	if status != http.StatusOK || json.Unmarshal(raw, &answer) != nil || answer.Token == "" {
		h.t.Fatalf("a signed registration of %s through %s: %d %s", id, machine, status, raw)
	}
	return answer.Token
}

func (h *harness) tokenAnswers(token string) bool {
	status, _ := h.call("GET", "/v1/agents", token, nil)
	return status == http.StatusOK
}

func paddedBody(fields string) []byte {
	return []byte("{" + fields + strings.Repeat(" ", 380*1024) + "}")
}

func TestASignedRegistrationOrObserverRacingARevokeOfItsMachineNeverLeavesALiveToken(t *testing.T) {
	const rounds = 120
	for _, route := range []struct{ path, fields string }{
		{"/v1/register", `"id":"alpha","harness":"claude-code","host":"mac"`},
		{"/v1/observers", `"kind":"tui"`},
	} {
		t.Run(route.path, func(t *testing.T) {
			h := start(t)
			h.advance(pastTheStartPeriod)
			body := paddedBody(route.fields)
			signedCall := func(key ed25519.PrivateKey) (int, []byte) {
				return h.sendWithAuthorization("POST", route.path, machineAuthorization(key, "linux-box", h.now.Unix(), "POST", route.path, body), body)
			}
			key := h.joinMachine("linux-box")
			began := time.Now()
			if status, raw := signedCall(key); status != http.StatusOK {
				t.Fatalf("a signed call with no revoke: %d %s", status, raw)
			}
			span := time.Since(began)
			if status, _ := h.call("DELETE", "/v1/machines/linux-box", secret, nil); status != http.StatusOK {
				t.Fatalf("revoke: %d", status)
			}
			alive := 0
			for round := 0; round < rounds; round++ {
				key := h.joinMachine("linux-box")
				answer := make(chan []byte, 1)
				go func() {
					status, raw := signedCall(key)
					if status != http.StatusOK {
						raw = nil
					}
					answer <- raw
				}()
				time.Sleep(span * time.Duration(round) / rounds)
				if status, _ := h.call("DELETE", "/v1/machines/linux-box", secret, nil); status != http.StatusOK {
					t.Fatalf("round %d: revoke: %d", round, status)
				}
				raw := <-answer
				var issued struct{ Token string }
				if raw != nil && json.Unmarshal(raw, &issued) == nil && issued.Token != "" && h.tokenAnswers(issued.Token) {
					alive++
				}
			}
			if alive > 0 {
				t.Errorf("%d of %d tokens issued through a machine still answered after the revoke of that machine finished", alive, rounds)
			}
		})
	}
}

func TestRevokingAMachineClosesTheStreamOpenedWithItsTokenAfterTheAgentRegisteredAgainElsewhere(t *testing.T) {
	for _, again := range []string{"through another machine", "with the shared secret"} {
		t.Run(again, func(t *testing.T) {
			h := start(t)
			h.advance(pastTheStartPeriod)
			alphaBox := h.joinMachine("alpha-box")
			bravoBox := h.joinMachine("bravo-box")
			first := h.signedRegister(alphaBox, "alpha-box", "alpha")
			stream := h.openStream(first)
			if stream.status != http.StatusOK {
				t.Fatalf("stream: %d", stream.status)
			}
			if again == "through another machine" {
				h.signedRegister(bravoBox, "bravo-box", "alpha")
			} else {
				h.register("alpha")
			}
			if status, _ := h.call("DELETE", "/v1/machines/alpha-box", secret, nil); status != http.StatusOK {
				t.Fatalf("revoke: %d", status)
			}
			ended := make(chan struct{})
			go func() {
				for range stream.lines {
				}
				close(ended)
			}()
			select {
			case <-ended:
			case <-time.After(2 * time.Second):
				t.Error("the stream opened with a token from alpha-box stayed open after alpha-box was revoked")
			}
		})
	}
}

func TestAStreamOpenedWhileItsMachineIsRevokedNeverStaysOpenAfterTheRevoke(t *testing.T) {
	const rounds, streams = 100, 24
	h := start(t)
	h.advance(pastTheStartPeriod)
	type opened struct {
		status int
		ended  <-chan struct{}
		cancel context.CancelFunc
	}
	openStream := func(token string) opened {
		ctx, cancel := context.WithCancel(context.Background())
		req, _ := http.NewRequestWithContext(ctx, "GET", h.srv.URL+"/v1/stream", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		ended := make(chan struct{})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			close(ended)
			return opened{-1, ended, cancel}
		}
		go func() { io.Copy(io.Discard, resp.Body); resp.Body.Close(); close(ended) }()
		return opened{resp.StatusCode, ended, cancel}
	}
	openAll := func(tokens []string) []opened {
		out := make([]opened, len(tokens))
		var wait sync.WaitGroup
		for i, token := range tokens {
			wait.Add(1)
			go func() { defer wait.Done(); out[i] = openStream(token) }()
		}
		wait.Wait()
		return out
	}
	registerAll := func() []string {
		key := h.joinMachine("linux-box")
		var tokens []string
		for i := 0; i < streams; i++ {
			tokens = append(tokens, h.signedRegister(key, "linux-box", fmt.Sprintf("a%03d", i)))
		}
		return tokens
	}
	revoke := func() {
		if status, _ := h.call("DELETE", "/v1/machines/linux-box", secret, nil); status != http.StatusOK {
			t.Fatalf("revoke: %d", status)
		}
	}
	began := time.Now()
	for _, stream := range openAll(registerAll()) {
		if stream.status != http.StatusOK {
			t.Fatalf("a stream with no revoke: %d", stream.status)
		}
		stream.cancel()
	}
	span := time.Since(began)
	revoke()
	open, total := 0, 0
	for round := 0; round < rounds; round++ {
		tokens := registerAll()
		result := make(chan []opened, 1)
		go func() { result <- openAll(tokens) }()
		time.Sleep(span * time.Duration(round) / (2 * rounds))
		revoke()
		deadline := time.Now().Add(time.Second)
		for _, stream := range <-result {
			if stream.status != http.StatusOK {
				continue
			}
			total++
			select {
			case <-stream.ended:
			case <-time.After(time.Until(deadline)):
				open++
			}
			stream.cancel()
		}
	}
	if open > 0 {
		t.Errorf("%d of %d streams opened with a machine's token stayed open after the revoke of that machine finished", open, total)
	}
}

func TestSavingTheMachineListNeverRemovesOrReplacesAFileItDidNotCreate(t *testing.T) {
	folder := t.TempDir()
	machinesPath := filepath.Join(folder, "machines.json")
	logPath := machinesPath + ".tmp"
	planted := map[string]string{
		machinesPath + ".new":                       "a person's file\n",
		machinesPath + "~":                          "a person's backup\n",
		filepath.Join(folder, ".machines.json.tmp"): "another program's file\n",
	}
	for path, content := range planted {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log, err := store.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	logBefore, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	hub, err := New(Config{Secret: secret, MachinesPath: machinesPath}, log)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, hub: hub}
	h.srv = newTestServer(t, hub)
	h.joinMachine("alpha-box")
	h.joinMachine("bravo-box")
	if status, _ := h.call("DELETE", "/v1/machines/alpha-box", secret, nil); status != http.StatusOK {
		t.Fatalf("revoke: %d", status)
	}
	logAfter, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("the open message log at %s is gone after the machine list was saved: %v", filepath.Base(logPath), err)
	}
	if !os.SameFile(logBefore, logAfter) {
		t.Errorf("the open message log at %s was replaced by another file", filepath.Base(logPath))
	}
	for path, content := range planted {
		if raw, err := os.ReadFile(path); err != nil || string(raw) != content {
			t.Errorf("%s changed: %q %v", filepath.Base(path), raw, err)
		}
	}
	raw, err := os.ReadFile(machinesPath)
	var stored []struct{ Name string }
	if err != nil || json.Unmarshal(raw, &stored) != nil || len(stored) != 1 || stored[0].Name != "bravo-box" {
		t.Errorf("machines.json: %q %v", raw, err)
	}
	entries, _ := os.ReadDir(folder)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	want := []string{".machines.json.tmp", "machines.json", "machines.json.new", "machines.json.tmp", "machines.json~"}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Errorf("files after the saves: %v, want %v", names, want)
	}
}

func TestASuccessfulInviteReadsTheClockOnce(t *testing.T) {
	var reads atomic.Int64
	clock := &settableClock{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	h := startWith(t, func(cfg *Config) {
		cfg.Now = func() time.Time {
			reads.Add(1)
			return clock.read()
		}
	})
	reads.Store(0)
	status, raw := h.call("POST", "/v1/machines", secret, map[string]string{"name": "linux-box"})
	if status != http.StatusOK {
		t.Fatalf("invite: %d %s", status, raw)
	}
	if got := reads.Load(); got != 1 {
		t.Errorf("one invite read the clock %d times, want 1", got)
	}
}

const pastTheStartPeriod = 6 * time.Second

func TestASignatureAcceptedAtTheEndOfTheStartPeriodOfARestartIsRefusedByTheNewProcess(t *testing.T) {
	machinesPath := filepath.Join(t.TempDir(), "machines.json")
	second := time.Date(2026, 11, 10, 12, 0, 10, 0, time.UTC)
	clock := &settableClock{now: second.Add(-10 * time.Second)}
	onClock := func(cfg *Config) {
		cfg.Now = clock.read
		cfg.MachinesPath = machinesPath
	}
	old := startWith(t, onClock)
	key := old.joinMachine("linux-box")
	clock.set(second.Add(4300 * time.Millisecond))
	body := []byte(`{"id":"alpha","harness":"claude-code","host":"mac"}`)
	authorization := machineAuthorization(key, "linux-box", second.Unix()+5, "POST", "/v1/register", body)
	if status, raw := old.sendWithAuthorization("POST", "/v1/register", authorization, body); status != http.StatusOK {
		t.Fatalf("the first process refused a signature timed %d: %d %s", second.Unix()+5, status, raw)
	}
	old.srv.Close()
	clock.set(second.Add(500 * time.Millisecond))
	restarted := startWith(t, onClock)
	clock.set(second.Add(5600 * time.Millisecond))
	if status, raw := restarted.sendWithAuthorization("POST", "/v1/register", authorization, body); status != http.StatusUnauthorized {
		t.Errorf("a process started in second %d accepted a copy of a signature timed 5 seconds after that second: %d %s", second.Unix(), status, raw)
	}
	clock.set(second.Add(6200 * time.Millisecond))
	later := machineAuthorization(key, "linux-box", second.Unix()+6, "POST", "/v1/register", body)
	if status, raw := restarted.sendWithAuthorization("POST", "/v1/register", later, body); status != http.StatusOK {
		t.Errorf("a signature timed 6 seconds after the start second: %d %s", status, raw)
	}
}
