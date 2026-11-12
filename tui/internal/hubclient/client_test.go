package hubclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/tui/internal/clean"
)

type stubHub struct {
	mu            sync.Mutex
	secret        string
	valid         map[string]bool
	issued        int
	registrations int
	polls         int
	agents        string
}

func newStubHub(secret string) *stubHub {
	return &stubHub{secret: secret, valid: map[string]bool{}, agents: `[{"name":"alpha","harness":"claude","host":"mac","connected":true}]`}
}

func (s *stubHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	switch r.Method + " " + r.URL.Path {
	case "POST /v1/observers":
		var body struct{ Kind string }
		json.NewDecoder(r.Body).Decode(&body)
		if bearer != s.secret || body.Kind != "tui" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		s.registrations++
		s.issued++
		token := "observer-token-" + string(rune('a'+s.issued))
		s.valid[token] = true
		json.NewEncoder(w).Encode(map[string]string{"token": token})
	case "GET /v1/agents":
		s.polls++
		if !s.valid[bearer] {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(s.agents))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestAgentsRegistersAnObserverOnceAndPollsWithItsToken(t *testing.T) {
	stub := newStubHub("s3cret-s3cret-s3cret")
	server := httptest.NewServer(stub)
	defer server.Close()
	client := New(server.URL, "s3cret-s3cret-s3cret")
	for range 3 {
		agents, err := client.Agents()
		if err != nil || len(agents) != 1 || agents[0].Name != "alpha" || !agents[0].Connected {
			t.Fatalf("got %v %v", agents, err)
		}
	}
	if stub.registrations != 1 || stub.polls != 3 {
		t.Fatalf("registrations %d polls %d", stub.registrations, stub.polls)
	}
}

func TestAgentsRegistersAgainOnceOn401AndRetries(t *testing.T) {
	stub := newStubHub("s3cret-s3cret-s3cret")
	server := httptest.NewServer(stub)
	defer server.Close()
	client := New(server.URL, "s3cret-s3cret-s3cret")
	if _, err := client.Agents(); err != nil {
		t.Fatal(err)
	}
	stub.mu.Lock()
	stub.valid = map[string]bool{}
	stub.mu.Unlock()
	agents, err := client.Agents()
	if err != nil || len(agents) != 1 {
		t.Fatalf("got %v %v", agents, err)
	}
	if stub.registrations != 2 || stub.polls != 3 {
		t.Fatalf("registrations %d polls %d", stub.registrations, stub.polls)
	}
}

func TestAgentsRegistersAgainOnlyOncePerCall(t *testing.T) {
	stub := newStubHub("s3cret-s3cret-s3cret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/agents" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		stub.ServeHTTP(w, r)
	}))
	defer server.Close()
	client := New(server.URL, "s3cret-s3cret-s3cret")
	if _, err := client.Agents(); err == nil {
		t.Fatal("want an error")
	}
	if stub.registrations != 2 {
		t.Fatalf("registrations %d", stub.registrations)
	}
}

func TestAgentsReportsARefusedSecretWithoutPrintingIt(t *testing.T) {
	server := httptest.NewServer(newStubHub("right-right-right-right"))
	defer server.Close()
	_, err := New(server.URL, "wrong-wrong-wrong-wrong").Agents()
	if err == nil || !strings.Contains(err.Error(), "refused the hub secret") || strings.Contains(err.Error(), "wrong-wrong") {
		t.Fatalf("got %v", err)
	}
}

func TestAgentsNeedsASecret(t *testing.T) {
	if _, err := New("http://127.0.0.1:1", "").Agents(); err != ErrNoSecret {
		t.Fatalf("got %v", err)
	}
}

func TestAgentsSaysWhenTheHubIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	address := server.URL
	server.Close()
	_, err := New(address, "s3cret-s3cret-s3cret").Agents()
	if err == nil || !strings.Contains(err.Error(), "cannot reach the hub at "+address) {
		t.Fatalf("got %v", err)
	}
}

func TestAgentsDecodesEveryField(t *testing.T) {
	stub := newStubHub("s3cret-s3cret-s3cret")
	stub.agents = `[{"name":"bravo","harness":"codex","host":"mac","session":"s1","connected":false,"last_seen":"2026-11-12T10:00:00Z","folder":"/w","branch":"main","worktree":"wt","harness_version":"1.2","landline_version":"0.9","model":"m","effort":"high","label":"review"}]`
	server := httptest.NewServer(stub)
	defer server.Close()
	agents, err := New(server.URL, "s3cret-s3cret-s3cret").Agents()
	if err != nil || len(agents) != 1 {
		t.Fatal(agents, err)
	}
	a := agents[0]
	if a.Name != "bravo" || a.Harness != "codex" || a.Host != "mac" || a.Session != "s1" || a.Connected || a.Folder != "/w" ||
		a.Branch != "main" || a.Worktree != "wt" || a.HarnessVersion != "1.2" || a.LandlineVersion != "0.9" ||
		a.Model != "m" || a.Effort != "high" || a.Label != "review" || a.LastSeen.IsZero() {
		t.Fatalf("got %+v", a)
	}
}

func TestTheDefaultTimeoutIsFiveSeconds(t *testing.T) {
	if got := New("http://hub", "s").http.Timeout; got != 5*time.Second {
		t.Fatalf("got %s", got)
	}
}

func TestAgentsFailsWithinTheTimeoutWhenTheHubNeverAnswers(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	client := newWithTimeout(server.URL, "s3cret-s3cret-s3cret", 100*time.Millisecond)
	start := time.Now()
	_, err := client.Agents()
	if err == nil || !strings.Contains(err.Error(), "cannot reach the hub") {
		t.Fatalf("got %v", err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("took %s", took)
	}
}

func oversizedHub(size int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/observers" {
			w.Write([]byte(`{"token":"observer-token-xyz"}`))
			return
		}
		body := []byte("[" + strings.Repeat(" ", size-2) + "]")
		w.Write(body)
	}))
}

func TestAgentsRejectsAnAnswerOverTheLimitWithAnExplicitError(t *testing.T) {
	server := oversizedHub(maxResponseBytes + 1)
	defer server.Close()
	_, err := New(server.URL, "s3cret-s3cret-s3cret").Agents()
	if err == nil || !strings.Contains(err.Error(), "the hub answered with more than 8 MiB") {
		t.Fatalf("got %v", err)
	}
}

func TestAgentsAcceptsAnAnswerOfExactlyTheLimit(t *testing.T) {
	server := oversizedHub(maxResponseBytes)
	defer server.Close()
	agents, err := New(server.URL, "s3cret-s3cret-s3cret").Agents()
	if err != nil || len(agents) != 0 {
		t.Fatalf("got %v %v", agents, err)
	}
}

func TestARedirectIsAnErrorThatNeverEchoesLocationAndIsNeverFollowed(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests += 100
		mu.Unlock()
	}))
	defer target.Close()
	hostile := target.URL + "/pwned\u202e\u0085"
	for _, status := range []int{301, 302, 303, 307, 308} {
		requests = 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			requests++
			mu.Unlock()
			w.Header().Set("Location", hostile)
			w.WriteHeader(status)
		}))
		client := New(server.URL, "s3cret-s3cret-s3cret")
		_, err := client.Agents()
		server.Close()
		if err == nil || err.Error() != "the hub answered with a redirect" || strings.Contains(err.Error(), "pwned") {
			t.Fatalf("%d: got %v", status, err)
		}
		if requests != 1 {
			t.Fatalf("%d: %d requests, the redirect was followed or retried", status, requests)
		}
	}
}

func TestARedirectOnAPollNeverReplaysTheBearerTokenToAnotherHost(t *testing.T) {
	var leaked bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked = true
		}
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/observers" {
			w.Write([]byte(`{"token":"observer-token-xyz"}`))
			return
		}
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()
	if _, err := New(server.URL, "s3cret-s3cret-s3cret").Agents(); err == nil || leaked {
		t.Fatalf("err %v leaked %v", err, leaked)
	}
}

func TestErrorsFromTheClientCarryNoControlRune(t *testing.T) {
	client := New("http://127.0.0.1:1", "s3cret-s3cret-s3cret")
	_, err := client.Agents()
	if err == nil || err.Error() != clean.Text(err.Error()) {
		t.Fatalf("got %q", err)
	}
	hostile := New("http://h\x1b[2J\u0085‮\n:1", "s3cret-s3cret-s3cret")
	_, err = hostile.Agents()
	if err == nil || err.Error() != clean.Text(err.Error()) {
		t.Fatalf("got %q", err)
	}
}

func TestAMalformedHostileHeaderErrorIsCleaned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Write([]byte("HTTP/1.1 302 Found\r\nLocation: http://evil/\x1b]0;pwned\x07\u0085\r\n\r\n"))
		conn.Close()
	}))
	defer server.Close()
	_, err := New(server.URL, "s3cret-s3cret-s3cret").Agents()
	if err == nil || err.Error() != clean.Text(err.Error()) {
		t.Fatalf("got %q", err)
	}
}

type zeroBody struct {
	mu      sync.Mutex
	written int
}

func (z *zeroBody) add(n int) {
	z.mu.Lock()
	z.written += n
	z.mu.Unlock()
}

func (z *zeroBody) total() int {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.written
}

func TestAnAnswerOneHundredTimesTheLimitIsRejectedWithoutReadingItAll(t *testing.T) {
	sent := &zeroBody{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/observers" {
			w.Write([]byte(`{"token":"observer-token-xyz"}`))
			return
		}
		chunk := make([]byte, 1<<20)
		for sent.total() < 100*maxResponseBytes {
			n, err := w.Write(chunk)
			sent.add(n)
			if err != nil {
				return
			}
		}
	}))
	defer server.Close()
	start := time.Now()
	_, err := New(server.URL, "s3cret-s3cret-s3cret").Agents()
	if err == nil || err.Error() != "the hub answered with more than 8 MiB" {
		t.Fatalf("got %v", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("took %s", took)
	}
	if got := sent.total(); got >= 10*maxResponseBytes {
		t.Fatalf("the hub got to send %d bytes, the client kept reading", got)
	}
}

func TestASecretWithAControlCharacterIsRefusedWithoutEchoingIt(t *testing.T) {
	server := httptest.NewServer(newStubHub("s3cret-s3cret"))
	defer server.Close()
	for _, secret := range []string{"s3cret\nx-injected: 1", "s3cret\x00", "s3cret\x1b[2J", "s3cret\r"} {
		_, err := New(server.URL, secret).Agents()
		if err == nil || err.Error() != "the hub secret contains a control character" || strings.Contains(err.Error(), "s3cret") {
			t.Fatalf("%q: got %v", secret, err)
		}
	}
}

const proxyChildEnv = "LANDLINE_TEST_PROXY_CHILD"

func TestProxyChild(t *testing.T) {
	if os.Getenv(proxyChildEnv) != "1" {
		t.Skip("runs only as the child of TestTheClientUsesTheDefaultTransportAndHonoursHTTPProxy")
	}
	New("http://hub.landline-proxy-test.example:8787", "s3cret-s3cret-s3cret").Agents()
}

func TestTheClientUsesTheDefaultTransportAndHonoursHTTPProxy(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.String())
		mu.Unlock()
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestProxyChild$")
	cmd.Env = append(os.Environ(), proxyChildEnv+"=1", "HTTP_PROXY="+proxy.URL, "http_proxy="+proxy.URL, "NO_PROXY=", "no_proxy=", "REQUEST_METHOD=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 || seen[0] != "POST http://hub.landline-proxy-test.example:8787/v1/observers" {
		t.Fatalf("the request did not go through the proxy: %v", seen)
	}
}
