package hubclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/tui/internal/clean"
)

const operatorSecret = "s3cret-s3cret-s3cret"

type operatorStub struct {
	mu            sync.Mutex
	valid         map[string]bool
	registrations []string
	observers     int
	sends         []map[string]string
	sendTokens    []string
	status        int
	answer        string
	delay         time.Duration
	issued        int
}

func newOperatorStub() *operatorStub {
	return &operatorStub{valid: map[string]bool{}}
}

func (s *operatorStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	delay := s.delay
	s.mu.Unlock()
	if delay > 0 && r.URL.Path == "/v1/operator/send" {
		time.Sleep(delay)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	switch r.Method + " " + r.URL.Path {
	case "POST /v1/operators":
		var body struct{ Kind string }
		json.NewDecoder(r.Body).Decode(&body)
		s.registrations = append(s.registrations, body.Kind)
		if bearer != operatorSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		s.issued++
		token := "operator-token-" + string(rune('a'+s.issued))
		s.valid[token] = true
		json.NewEncoder(w).Encode(map[string]string{"token": token})
	case "POST /v1/observers":
		s.observers++
		s.issued++
		json.NewEncoder(w).Encode(map[string]string{"token": "observer-token-" + string(rune('a'+s.issued))})
	case "GET /v1/agents":
		w.Write([]byte(`[]`))
	case "POST /v1/operator/send":
		s.sendTokens = append(s.sendTokens, bearer)
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if !s.valid[bearer] {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unknown operator token"})
			return
		}
		s.sends = append(s.sends, body)
		if s.status != 0 {
			w.WriteHeader(s.status)
			json.NewEncoder(w).Encode(map[string]string{"error": s.answer})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "m-1", "seq": len(s.sends), "recipients": []string{body["to"]}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestSendRegistersAnOperatorOnTheFirstSendOnlyAndSendsAChatMessageWithItsToken(t *testing.T) {
	stub := newOperatorStub()
	server := httptest.NewServer(stub)
	defer server.Close()
	client := New(server.URL, operatorSecret)
	line, err := client.Send("bravo", "hello there")
	if err != nil || line != "sent m-1 (seq 1) to bravo" {
		t.Fatalf("got %q, %v", line, err)
	}
	if line, err := client.Send("alpha", "again"); err != nil || line != "sent m-1 (seq 2) to alpha" {
		t.Fatalf("got %q, %v", line, err)
	}
	if len(stub.registrations) != 1 || stub.registrations[0] != "tui" {
		t.Fatalf("registrations %v", stub.registrations)
	}
	if len(stub.sends) != 2 || stub.sends[0]["to"] != "bravo" || stub.sends[0]["body"] != "hello there" || stub.sends[0]["kind"] != "chat" || stub.sends[1]["to"] != "alpha" {
		t.Fatalf("sends %v", stub.sends)
	}
	for _, token := range stub.sendTokens {
		if token == operatorSecret || token != stub.sendTokens[0] || !strings.HasPrefix(token, "operator-token-") {
			t.Fatalf("a send used the token %q", token)
		}
	}
	for _, body := range stub.sends {
		for key := range body {
			if key != "to" && key != "body" && key != "kind" {
				t.Errorf("an unexpected field %q in %v", key, body)
			}
		}
	}
}

func TestPollingNeverRegistersAnOperatorAndSendingNeverRegistersAnObserver(t *testing.T) {
	stub := newOperatorStub()
	server := httptest.NewServer(stub)
	defer server.Close()
	client := New(server.URL, operatorSecret)
	if _, err := client.Agents(); err != nil {
		t.Fatal(err)
	}
	if len(stub.registrations) != 0 {
		t.Fatalf("a poll registered an operator: %v", stub.registrations)
	}
	if _, err := client.Send("bravo", "x"); err != nil {
		t.Fatal(err)
	}
	if stub.observers != 1 || len(stub.registrations) != 1 {
		t.Fatalf("observers %d operators %v", stub.observers, stub.registrations)
	}
	if _, err := client.Agents(); err != nil || stub.observers != 1 {
		t.Fatalf("observers %d, %v", stub.observers, err)
	}
}

func TestSendRegistersAgainOnA401AndRetriesOnceDeliveringTheMessageOnce(t *testing.T) {
	stub := newOperatorStub()
	server := httptest.NewServer(stub)
	defer server.Close()
	client := New(server.URL, operatorSecret)
	if _, err := client.Send("bravo", "first"); err != nil {
		t.Fatal(err)
	}
	stub.mu.Lock()
	stub.valid = map[string]bool{}
	stub.mu.Unlock()
	line, err := client.Send("bravo", "second")
	if err != nil || line == "" {
		t.Fatalf("got %q, %v", line, err)
	}
	if len(stub.registrations) != 2 {
		t.Fatalf("registrations %v", stub.registrations)
	}
	delivered := 0
	for _, body := range stub.sends {
		if body["body"] == "second" {
			delivered++
		}
	}
	if delivered != 1 {
		t.Fatalf("the message was delivered %d times", delivered)
	}
}

func TestSendRegistersAgainOnlyOncePerCallWhenEveryTokenIsRefused(t *testing.T) {
	stub := newOperatorStub()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/operator/send" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unknown operator token"})
			return
		}
		stub.ServeHTTP(w, r)
	}))
	defer server.Close()
	if _, err := New(server.URL, operatorSecret).Send("bravo", "x"); err == nil {
		t.Fatal("want an error")
	}
	if len(stub.registrations) != 2 {
		t.Fatalf("registrations %v", stub.registrations)
	}
}

func TestSendReportsTheSwitchboardsReasonForEveryRefusal(t *testing.T) {
	for status, reason := range map[int]string{
		404: "the recipient is not a registered agent",
		409: "not connected",
		422: "a message needs a body",
		502: "the session said no",
		504: "no delivery report",
		413: "the body is larger than 64 KiB",
	} {
		stub := newOperatorStub()
		stub.status, stub.answer = status, reason
		server := httptest.NewServer(stub)
		line, err := New(server.URL, operatorSecret).Send("bravo", "x")
		server.Close()
		if err == nil || line != "" || !strings.Contains(err.Error(), reason) {
			t.Errorf("%d: got %q, %v", status, line, err)
		}
	}
}

func TestSendReportsARefusedSecretWithoutPrintingIt(t *testing.T) {
	server := httptest.NewServer(newOperatorStub())
	defer server.Close()
	_, err := New(server.URL, "wrong-wrong-wrong-wrong").Send("bravo", "x")
	if err == nil || !strings.Contains(err.Error(), "refused the switchboard secret") || strings.Contains(err.Error(), "wrong-wrong") {
		t.Fatalf("got %v", err)
	}
}

func TestSendNeedsASecretAndRefusesAControlCharacterInIt(t *testing.T) {
	if _, err := New("http://127.0.0.1:1", "").Send("bravo", "x"); err != ErrNoSecret {
		t.Fatalf("got %v", err)
	}
	server := httptest.NewServer(newOperatorStub())
	defer server.Close()
	for _, secret := range []string{"s3cret\nx-injected: 1", "s3cret\x00", "s3cret\r"} {
		if _, err := New(server.URL, secret).Send("bravo", "x"); err != ErrControlSecret {
			t.Errorf("%q: got %v", secret, err)
		}
	}
}

func TestSendSaysWhenTheSwitchboardIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	address := server.URL
	server.Close()
	_, err := New(address, operatorSecret).Send("bravo", "x")
	if err == nil || !strings.Contains(err.Error(), "cannot reach the switchboard at "+address) {
		t.Fatalf("got %v", err)
	}
}

func TestSendWaitsLongerThanAPollMayForTheDeliveryReport(t *testing.T) {
	stub := newOperatorStub()
	stub.delay = 600 * time.Millisecond
	server := httptest.NewServer(stub)
	defer server.Close()
	client := newWithTimeout(server.URL, operatorSecret, 100*time.Millisecond)
	if line, err := client.Send("bravo", "slow"); err != nil || !strings.HasPrefix(line, "sent ") {
		t.Fatalf("got %q, %v", line, err)
	}
}

func TestTheResultLineAndTheErrorsOfASendCarryNoControlRune(t *testing.T) {
	evil := "\x1b]0;pwned\x07 \u0085 line1\nline2‮"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/operators":
			w.Write([]byte(`{"token":"operator-token-x"}`))
		case "/v1/operator/send":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["body"] == "fail" {
				w.WriteHeader(http.StatusBadGateway)
				json.NewEncoder(w).Encode(map[string]string{"error": evil})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"id": evil, "seq": 1, "recipients": []string{evil}})
		}
	}))
	defer server.Close()
	client := New(server.URL, operatorSecret)
	line, err := client.Send("bravo", "ok")
	if err != nil || line != clean.Text(line) || !strings.Contains(line, "pwned") {
		t.Fatalf("got %q, %v", line, err)
	}
	_, err = client.Send("bravo", "fail")
	if err == nil || err.Error() != clean.Text(err.Error()) || !strings.Contains(err.Error(), "pwned") {
		t.Fatalf("got %v", err)
	}
}

func TestARedirectOnASendIsAnErrorAndNeverReplaysTheOperatorTokenToAnotherHost(t *testing.T) {
	var leaked bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked = true
		}
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/operators" {
			w.Write([]byte(`{"token":"operator-token-x"}`))
			return
		}
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	if _, err := New(server.URL, operatorSecret).Send("bravo", "x"); err == nil || leaked {
		t.Fatalf("err %v leaked %v", err, leaked)
	}
}

func TestTheAgentsAnswerCarriesTheTmuxSocketAndPane(t *testing.T) {
	stub := newStubHub("s3cret-s3cret-s3cret")
	stub.agents = `[{"name":"bravo","harness":"codex","host":"mac","connected":true,"tmux_socket":"default","tmux_pane":"%3"},{"name":"alpha","harness":"codex","host":"mac"}]`
	server := httptest.NewServer(stub)
	defer server.Close()
	agents, err := New(server.URL, "s3cret-s3cret-s3cret").Agents()
	if err != nil || len(agents) != 2 {
		t.Fatal(agents, err)
	}
	if agents[0].TmuxSocket != "default" || agents[0].TmuxPane != "%3" || agents[1].TmuxSocket != "" || agents[1].TmuxPane != "" {
		t.Fatalf("got %+v", agents)
	}
}
