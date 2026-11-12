package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type run struct {
	out, err bytes.Buffer
	code     int
}

func do(t *testing.T, dir, url string, stdin string, args ...string) run {
	t.Helper()
	var r run
	env := Env{
		Stdin: strings.NewReader(stdin), Stdout: &r.out, Stderr: &r.err, ConfigDir: dir, Host: "mac",
		Getenv: func(k string) string {
			return map[string]string{"LANDLINE_URL": url, "LANDLINE_SECRET": "s3cret-s3cret-s3cret"}[k]
		},
	}
	r.code = Run(args, env)
	return r
}

func fakeHub(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/register", func(w http.ResponseWriter, r *http.Request) {
		var req model.RegisterRequest
		json.NewDecoder(r.Body).Decode(&req)
		*seen = append(*seen, "register "+req.Name+" "+req.Host+" "+r.Header.Get("Authorization"))
		json.NewEncoder(w).Encode(model.RegisterResponse{Name: req.Name, Token: "tok-" + req.Name})
	})
	mux.HandleFunc("/v1/send", func(w http.ResponseWriter, r *http.Request) {
		var req model.SendBody
		json.NewDecoder(r.Body).Decode(&req)
		*seen = append(*seen, "send "+req.To+" "+req.Body+" "+r.Header.Get("Authorization"))
		json.NewEncoder(w).Encode(model.SendResponse{ID: "m-1", Seq: 3, Recipients: []string{req.To}})
	})
	mux.HandleFunc("/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]model.AgentInfo{{Name: "bravo", Harness: "codex", Host: "mac", Connected: true}, {Name: "alpha", Harness: "claude-code", Host: "linux", Connected: true}, {Name: "charlie", Harness: "codex", Host: "mac"}})
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestRegisterSavesAPrivateSessionAndSendUsesIt(t *testing.T) {
	t.Parallel()
	var seen []string
	hub := fakeHub(t, &seen)
	dir := t.TempDir()
	if r := do(t, dir, hub.URL, "", "register", "--name", "alpha", "--harness", "codex"); r.code != 0 || !strings.Contains(r.out.String(), "registered alpha") {
		t.Fatalf("register: %d %s %s", r.code, r.out.String(), r.err.String())
	}
	info, err := os.Stat(filepath.Join(dir, "session-alpha.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("session file: %v %v", info, err)
	}
	r := do(t, dir, hub.URL, "", "send", "--as", "alpha", "--to", "bravo", "hello", "there")
	if r.code != 0 || !strings.Contains(r.out.String(), "sent m-1 (seq 3) to bravo") {
		t.Fatalf("send: %d %s %s", r.code, r.out.String(), r.err.String())
	}
	if want := "send bravo hello there Bearer tok-alpha"; seen[1] != want {
		t.Fatalf("seen %v", seen)
	}
	if !strings.HasSuffix(seen[0], "Bearer s3cret-s3cret-s3cret") || !strings.Contains(seen[0], "alpha mac") {
		t.Fatalf("register call %q", seen[0])
	}
}

func TestSendReadsTheBodyFromStandardInputForADash(t *testing.T) {
	t.Parallel()
	var seen []string
	hub := fakeHub(t, &seen)
	dir := t.TempDir()
	do(t, dir, hub.URL, "", "register", "--name", "alpha", "--harness", "codex")
	do(t, dir, hub.URL, "piped body", "send", "--as", "alpha", "--to", "bravo", "-")
	if seen[1] != "send bravo piped body Bearer tok-alpha" {
		t.Fatalf("seen %v", seen)
	}
}

func TestAgentsListsOnePerLine(t *testing.T) {
	t.Parallel()
	var seen []string
	hub := fakeHub(t, &seen)
	dir := t.TempDir()
	do(t, dir, hub.URL, "", "register", "--name", "alpha", "--harness", "codex")
	r := do(t, dir, hub.URL, "", "agents", "--as", "alpha")
	if !strings.Contains(r.out.String(), "bravo\tcodex\tmac\tconnected\n") || !strings.Contains(r.out.String(), "alpha\tclaude-code\tlinux\tconnected\n") || !strings.Contains(r.out.String(), "charlie\tcodex\tmac\tdisconnected\n") {
		t.Fatalf("got %q", r.out.String())
	}
}

func TestRemovedCommandsAreUnknownAndSendHelpDoesNotOfferAll(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"inbox", "listen", "hook", "agentd"} {
		r := do(t, t.TempDir(), "", "", name, "--as", "alpha")
		if r.code != 2 || !strings.Contains(r.err.String(), "unknown command") {
			t.Errorf("%s: %d %q", name, r.code, r.err.String())
		}
	}
	helpRun := do(t, t.TempDir(), "", "", "help")
	help := helpRun.err.String()
	if strings.Contains(help, "or all") || strings.Contains(help, "inbox") || strings.Contains(help, "--wake") {
		t.Fatalf("help still offers removed things: %q", help)
	}
	if !strings.Contains(help, "connected") {
		t.Fatalf("help does not mention connected: %q", help)
	}
	send := do(t, t.TempDir(), "", "", "send", "-h")
	if strings.Contains(send.err.String(), "or all") {
		t.Fatalf("send flag help offers all: %q", send.err.String())
	}
}

func TestMissingPiecesGiveClearErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"send", "--to", "bravo", "x"}, "say who you are"},
		{[]string{"send", "--as", "alpha", "--to", "bravo", "x"}, "no session here"},
		{[]string{"send", "--as", "../etc", "--to", "bravo", "x"}, "bad agent name"},
		{[]string{"register", "--name", "alpha", "--harness", "codex"}, "LANDLINE_URL"},
	} {
		r := do(t, dir, "", "", c.args...)
		if r.code != 1 || !strings.Contains(r.err.String(), c.want) {
			t.Errorf("%v: %d %q", c.args, r.code, r.err.String())
		}
	}
}

func TestUnknownCommandsAndNoArgumentsShowUsage(t *testing.T) {
	t.Parallel()
	if r := do(t, t.TempDir(), "", ""); r.code != 2 || !strings.Contains(r.err.String(), "landline <command>") {
		t.Fatalf("%d %q", r.code, r.err.String())
	}
	if r := do(t, t.TempDir(), "", "", "frobnicate"); r.code != 2 || !strings.Contains(r.err.String(), "unknown command") {
		t.Fatalf("%d %q", r.code, r.err.String())
	}
}

func TestRegisterRefusesAHubThatReturnsADifferentNameAndWritesNothing(t *testing.T) {
	t.Parallel()
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(model.RegisterResponse{Name: "../../../../victim/pwned", Token: "stolen"})
	}))
	defer hub.Close()
	root := t.TempDir()
	dir := filepath.Join(root, "a", "b", "config")
	r := do(t, dir, hub.URL, "", "register", "--name", "innocent", "--harness", "codex")
	if r.code != 1 || !strings.Contains(r.err.String(), "not the name that was asked for") {
		t.Fatalf("%d %q", r.code, r.err.String())
	}
	var found []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found = append(found, path)
		}
		return nil
	})
	if len(found) != 0 {
		t.Fatalf("files were written: %v", found)
	}
}

func TestRegisterTightensAnExistingLooseSessionFile(t *testing.T) {
	t.Parallel()
	var seen []string
	hub := fakeHub(t, &seen)
	dir := t.TempDir()
	path := filepath.Join(dir, "session-alpha.json")
	os.WriteFile(path, []byte("{}"), 0o644)
	do(t, dir, hub.URL, "", "register", "--name", "alpha", "--harness", "codex")
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestLogPrintsEachMessagesTextSoLostMailCanBeRecovered(t *testing.T) {
	t.Parallel()
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]model.Envelope{{Seq: 1, ID: "m-1", From: "alpha", To: "bravo", Thread: "main", Kind: model.KindChat, Body: "first line\nsecond \x1b[31mline"}})
	}))
	defer hub.Close()
	r := do(t, t.TempDir(), hub.URL, "", "log")
	out := r.out.String()
	if r.code != 0 || !strings.Contains(out, "alpha -> bravo") || !strings.Contains(out, "    first line\n") || !strings.Contains(out, "second  [31mline") || strings.ContainsRune(out, 0x1b) {
		t.Fatalf("%d %q", r.code, out)
	}
}

func TestHubFieldsPrintedByLogSendAndAgentsCarryNoControlSequences(t *testing.T) {
	t.Parallel()
	evil := "\x1b]0;PWNED\x07\x1b[2J"
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/register":
			json.NewEncoder(w).Encode(model.RegisterResponse{Name: "alpha", Token: "t"})
		case "/v1/send":
			json.NewEncoder(w).Encode(model.SendResponse{ID: evil, Seq: 1, Recipients: []string{evil}})
		case "/v1/agents":
			json.NewEncoder(w).Encode([]model.AgentInfo{{Name: "alpha", Harness: "h", Host: "m", Label: evil, Connected: true}})
		}
	}))
	defer hub.Close()
	dir := t.TempDir()
	do(t, dir, hub.URL, "", "register", "--name", "alpha", "--harness", "codex")
	for _, args := range [][]string{{"send", "--as", "alpha", "--to", "bravo", "hi"}, {"agents", "--as", "alpha"}} {
		r := do(t, dir, hub.URL, "", args...)
		if strings.ContainsAny(r.out.String(), "\x1b\x07") {
			t.Errorf("%v printed a control sequence: %q", args, r.out.String())
		}
	}
}

func TestLogPrintsTheDeliveryResultAfterTheKindAndNothingWhenThereIsNone(t *testing.T) {
	t.Parallel()
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]model.Envelope{
			{Seq: 1, ID: "m-1", From: "alpha", To: "bravo", Thread: "main", Kind: model.KindChat, Body: "one", Result: "delivered"},
			{Seq: 2, ID: "m-2", From: "alpha", To: "bravo", Thread: "main", Kind: model.KindChat, Body: "two"},
			{Seq: 3, ID: "m-3", From: "alpha", To: "bravo", Thread: "main", Kind: model.KindChat, Body: "three", Result: "bad \x1b[2J reply"},
		})
	}))
	defer hub.Close()
	r := do(t, t.TempDir(), hub.URL, "", "log")
	lines := strings.Split(r.out.String(), "\n")
	if r.code != 0 || !strings.HasSuffix(lines[0], "\tmain\tchat\tdelivered") || !strings.HasSuffix(lines[2], "\tmain\tchat") || !strings.HasSuffix(lines[4], "\tchat\tbad  [2J reply") {
		t.Fatalf("%d %q", r.code, lines)
	}
}

func TestACommandWhoseTokenTheHubForgotSaysToRegisterAgain(t *testing.T) {
	t.Parallel()
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(model.ErrorResponse{Error: "missing or unknown agent token"})
	}))
	t.Cleanup(hub.Close)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "session-alpha.json"), []byte(`{"url":"`+hub.URL+`","token":"gone"}`), 0o600)
	want := "the hub does not know this agent any more (it forgets an agent 2 minutes after it was last seen). Run landline register again"
	for _, args := range [][]string{{"send", "--as", "alpha", "--to", "bravo", "hi"}, {"agents", "--as", "alpha"}} {
		r := do(t, dir, hub.URL, "", args...)
		if r.code == 0 || !strings.Contains(r.err.String(), want) || strings.Contains(r.err.String(), "(401)") {
			t.Errorf("%v: %d %q", args, r.code, r.err.String())
		}
	}
}
