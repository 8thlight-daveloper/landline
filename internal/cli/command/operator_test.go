package command

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

const testSecret = "s3cret-s3cret-s3cret"

type operatorHub struct {
	mu            sync.Mutex
	registrations []string
	sends         []string
	sendStatus    int
	sendError     string
}

func newOperatorHub(t *testing.T) (*operatorHub, *httptest.Server) {
	t.Helper()
	hub := &operatorHub{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/operators", func(w http.ResponseWriter, r *http.Request) {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		var body struct{ Kind string }
		json.NewDecoder(r.Body).Decode(&body)
		hub.registrations = append(hub.registrations, body.Kind+" "+r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer "+testSecret {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(model.ErrorResponse{Error: "missing or wrong switchboard secret"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"token": "operator-token-1"})
	})
	mux.HandleFunc("/v1/operator/send", func(w http.ResponseWriter, r *http.Request) {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		var body model.SendBody
		json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body)
		hub.sends = append(hub.sends, r.Header.Get("Authorization")+" "+string(raw))
		if hub.sendStatus != 0 {
			w.WriteHeader(hub.sendStatus)
			json.NewEncoder(w).Encode(model.ErrorResponse{Error: hub.sendError})
			return
		}
		json.NewEncoder(w).Encode(model.SendResponse{ID: "m-1", Seq: 3, Recipients: []string{body.To}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return hub, server
}

func savedConfigDir(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	raw, _ := json.Marshal(map[string]string{"url": url, "secret": testSecret})
	if err := os.WriteFile(filepath.Join(dir, "config.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestOperatorSendRegistersAnOperatorWithTheSecretAndSendsWithItsTokenAndPrintsTheResultLikeSend(t *testing.T) {
	t.Parallel()
	hub, server := newOperatorHub(t)
	r := do(t, t.TempDir(), server.URL, "", "operator", "send", "--to", "bravo", "--body", "hello there")
	if r.code != 0 || r.out.String() != "sent m-1 (seq 3) to bravo\n" || r.err.String() != "" {
		t.Fatalf("%d %q %q", r.code, r.out.String(), r.err.String())
	}
	if len(hub.registrations) != 1 || hub.registrations[0] != "cli Bearer "+testSecret {
		t.Fatalf("registrations %v", hub.registrations)
	}
	if len(hub.sends) != 1 || !strings.HasPrefix(hub.sends[0], "Bearer operator-token-1 ") || strings.Contains(hub.sends[0], testSecret) {
		t.Fatalf("sends %v", hub.sends)
	}
	var sent model.SendBody
	json.Unmarshal([]byte(strings.TrimPrefix(hub.sends[0], "Bearer operator-token-1 ")), &sent)
	if sent.To != "bravo" || sent.Body != "hello there" || sent.Kind != "" || sent.Thread != "" || sent.InReplyTo != "" {
		t.Fatalf("sent %+v", sent)
	}
}

func TestOperatorSendPassesTheKindTheThreadAndInReplyTo(t *testing.T) {
	t.Parallel()
	hub, server := newOperatorHub(t)
	r := do(t, t.TempDir(), server.URL, "", "operator", "send", "--to", "bravo", "--body", "review this", "--kind", "review", "--thread", "deploy", "--in-reply-to", "m-7")
	if r.code != 0 {
		t.Fatalf("%d %q", r.code, r.err.String())
	}
	var sent model.SendBody
	json.Unmarshal([]byte(strings.TrimPrefix(hub.sends[0], "Bearer operator-token-1 ")), &sent)
	if sent.To != "bravo" || sent.Body != "review this" || sent.Kind != model.KindReview || sent.Thread != "deploy" || sent.InReplyTo != "m-7" {
		t.Fatalf("sent %+v", sent)
	}
}

func TestOperatorSendRegistersAgainForEachCommandAndSavesNothing(t *testing.T) {
	t.Parallel()
	hub, server := newOperatorHub(t)
	dir := t.TempDir()
	for range 2 {
		if r := do(t, dir, server.URL, "", "operator", "send", "--to", "bravo", "--body", "x"); r.code != 0 {
			t.Fatalf("%d %q", r.code, r.err.String())
		}
	}
	if len(hub.registrations) != 2 {
		t.Fatalf("registrations %v", hub.registrations)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files were written: %v", entries)
	}
}

func TestOperatorSendTakesTheAddressAndSecretFromTheSavedConfig(t *testing.T) {
	t.Parallel()
	hub, server := newOperatorHub(t)
	dir := savedConfigDir(t, server.URL)
	var r run
	env := Env{Stdin: strings.NewReader(""), Stdout: &r.out, Stderr: &r.err, ConfigDir: dir, Host: "mac", Getenv: func(string) string { return "" }}
	r.code = Run([]string{"operator", "send", "--to", "bravo", "--body", "x"}, env)
	if r.code != 0 || r.out.String() != "sent m-1 (seq 3) to bravo\n" {
		t.Fatalf("%d %q %q", r.code, r.out.String(), r.err.String())
	}
	if len(hub.registrations) != 1 || !strings.HasSuffix(hub.registrations[0], "Bearer "+testSecret) {
		t.Fatalf("registrations %v", hub.registrations)
	}
}

func TestOperatorSendWithoutASecretExitsOneNamingTheVariablesAndSendsNothing(t *testing.T) {
	t.Parallel()
	hub, _ := newOperatorHub(t)
	r := do(t, t.TempDir(), "", "", "operator", "send", "--to", "bravo", "--body", "x")
	if r.code != 1 || r.out.String() != "" || !strings.Contains(r.err.String(), "LANDLINE_URL") || !strings.Contains(r.err.String(), "LANDLINE_SECRET") {
		t.Fatalf("%d %q %q", r.code, r.out.String(), r.err.String())
	}
	if len(hub.registrations)+len(hub.sends) != 0 {
		t.Fatal("a request was made")
	}
}

func TestOperatorSendExitsWithTheCodeOfSendAndPrintsTheSwitchboardsReasonOnStandardError(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		status int
		reason string
	}{
		"an unknown recipient": {404, "the recipient is not a registered agent"},
		"not connected":        {409, "not connected"},
		"a failed delivery":    {502, "the session said no"},
		"no delivery report":   {504, "no delivery report"},
		"a send to oneself":    {422, "an agent cannot send a message to itself"},
	} {
		hub, server := newOperatorHub(t)
		hub.sendStatus, hub.sendError = c.status, c.reason
		r := do(t, t.TempDir(), server.URL, "", "operator", "send", "--to", "bravo", "--body", "x")
		if r.code != 1 || r.out.String() != "" || !strings.Contains(r.err.String(), c.reason) {
			t.Errorf("%s: %d %q %q", name, r.code, r.out.String(), r.err.String())
		}
	}
}

func TestOperatorSendWithAWrongSecretExitsOneWithoutPrintingIt(t *testing.T) {
	t.Parallel()
	_, server := newOperatorHub(t)
	var r run
	env := Env{Stdin: strings.NewReader(""), Stdout: &r.out, Stderr: &r.err, ConfigDir: t.TempDir(), Host: "mac", Getenv: func(k string) string {
		return map[string]string{"LANDLINE_URL": server.URL, "LANDLINE_SECRET": "wrong-secret-wrong-secret"}[k]
	}}
	r.code = Run([]string{"operator", "send", "--to", "bravo", "--body", "x"}, env)
	if r.code != 1 || r.out.String() != "" || strings.TrimSpace(r.err.String()) == "" || strings.Contains(r.err.String(), "wrong-secret-wrong-secret") {
		t.Fatalf("%d %q %q", r.code, r.out.String(), r.err.String())
	}
}

func TestOperatorSendPrintsNoControlSequenceFromTheSwitchboard(t *testing.T) {
	t.Parallel()
	evil := "\x1b]0;PWNED\x07\x1b[2J"
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/operators":
			json.NewEncoder(w).Encode(map[string]string{"token": "t"})
		case "/v1/operator/send":
			json.NewEncoder(w).Encode(model.SendResponse{ID: evil, Seq: 1, Recipients: []string{evil}})
		}
	}))
	t.Cleanup(hub.Close)
	r := do(t, t.TempDir(), hub.URL, "", "operator", "send", "--to", "bravo", "--body", "x")
	if r.code != 0 || strings.ContainsAny(r.out.String(), "\x1b\x07") {
		t.Fatalf("%d %q", r.code, r.out.String())
	}
}

func TestOperatorSendFlagFailuresExitWithTheCodeOfSendFlagFailures(t *testing.T) {
	t.Parallel()
	_, server := newOperatorHub(t)
	dir := t.TempDir()
	reference := do(t, dir, server.URL, "", "send", "--bogus")
	for _, args := range [][]string{{"operator", "send", "--bogus"}, {"operator", "send", "--to"}} {
		r := do(t, dir, server.URL, "", args...)
		if r.code != reference.code || r.code == 0 || r.out.String() != "" {
			t.Errorf("%v: %d %q, and send exits %d", args, r.code, r.out.String(), reference.code)
		}
	}
}

func TestOperatorWithNoSubcommandOrAnUnknownOneExitsNonZeroAndMakesNoRequest(t *testing.T) {
	t.Parallel()
	hub, server := newOperatorHub(t)
	for _, args := range [][]string{{"operator"}, {"operator", "frobnicate"}} {
		r := do(t, t.TempDir(), server.URL, "", args...)
		if r.code == 0 || r.out.String() != "" || strings.TrimSpace(r.err.String()) == "" {
			t.Errorf("%v: %d %q %q", args, r.code, r.out.String(), r.err.String())
		}
	}
	if len(hub.registrations)+len(hub.sends) != 0 {
		t.Fatal("a request was made")
	}
}
