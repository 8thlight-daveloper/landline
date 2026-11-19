package command

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	client "github.com/8thlight-daveloper/landline/internal/client/http"
	"github.com/8thlight-daveloper/landline/internal/model"
)

func fakeGit(answers map[string]string) GitRunner {
	return func(dir string, args ...string) (string, error) {
		if out, ok := answers[strings.Join(args, " ")]; ok {
			return out, nil
		}
		return "", errors.New("not a repository")
	}
}

func TestDetectIdentityTakesTheFolderBranchWorktreeAndEffort(t *testing.T) {
	t.Parallel()
	env := Env{Getenv: func(k string) string { return map[string]string{"CLAUDE_EFFORT": "high"}[k] }}
	plain := DetectIdentity(env, "/Users/me/Projects/threnos", fakeGit(map[string]string{
		"rev-parse --abbrev-ref HEAD": "fix/login", "rev-parse --git-dir": ".git", "rev-parse --git-common-dir": ".git",
	}))
	if plain.Folder != "threnos" || plain.Seed != "threnos" || plain.Branch != "fix/login" || plain.Worktree != "" || plain.Effort != "high" {
		t.Fatalf("%+v", plain)
	}
	tree := DetectIdentity(env, "/Users/me/Projects/threnos-wt", fakeGit(map[string]string{
		"rev-parse --abbrev-ref HEAD": "HEAD", "rev-parse --git-dir": "/r/.git/worktrees/wt", "rev-parse --git-common-dir": "/r/.git", "rev-parse --show-toplevel": "/Users/me/Projects/threnos-wt",
	}))
	if tree.Branch != "" || tree.Worktree != "threnos-wt" {
		t.Fatalf("a detached worktree: %+v", tree)
	}
	none := DetectIdentity(Env{Getenv: func(string) string { return "" }}, "/tmp/x", fakeGit(nil))
	if none.Folder != "x" || none.Branch != "" || none.Worktree != "" || none.Effort != "" {
		t.Fatalf("no git: %+v", none)
	}
}

func TestWithPrefersTheOverridesFieldByField(t *testing.T) {
	t.Parallel()
	got := Identity{Seed: "a", Folder: "a", Branch: "b", Model: "m"}.With(Identity{Branch: "x", Project: "lead"})
	if got.Seed != "a" || got.Branch != "x" || got.Model != "m" || got.Project != "lead" {
		t.Fatalf("%+v", got)
	}
}

func TestTheHarnessNameComesFromTheClientThenTheEnvironment(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, ai, want string }{
		{"claude-code", "", "claude-code"}, {"codex-mcp-client", "", "codex"}, {"cursor", "", "cursor"},
		{"", "claude-code_2-1-296_harness", "claude-code"}, {"", "", "mcp"},
	} {
		if got := HarnessFromClient(c.name, c.ai); got != c.want {
			t.Errorf("%+v: %q", c, got)
		}
	}
}

type recordingHub struct {
	mu          sync.Mutex
	register    []model.RegisterRequest
	tokens      int
	rejected    int
	reports     []string
	streams     []string
	failReports bool
}

func (r *recordingHub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/register", func(w http.ResponseWriter, req *http.Request) {
		var body model.RegisterRequest
		json.NewDecoder(req.Body).Decode(&body)
		r.mu.Lock()
		r.register = append(r.register, body)
		r.tokens++
		n := r.tokens
		r.mu.Unlock()
		json.NewEncoder(w).Encode(model.RegisterResponse{ID: body.Seed + "-" + string(rune('a'+n-1)), Token: "tok" + string(rune('0'+n))})
	})
	mux.HandleFunc("/v1/attach", func(w http.ResponseWriter, req *http.Request) {
		json.NewEncoder(w).Encode(model.AttachResponse{ID: "x"})
	})
	mux.HandleFunc("/v1/send", func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		reject := r.rejected > 0
		if reject {
			r.rejected--
		}
		r.mu.Unlock()
		if reject {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(model.ErrorResponse{Error: "missing or unknown agent token"})
			return
		}
		json.NewEncoder(w).Encode(model.SendResponse{ID: "m-1", Seq: 1})
	})
	mux.HandleFunc("/v1/deliveries/", func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		reject := r.rejected > 0
		if reject {
			r.rejected--
		}
		r.reports = append(r.reports, req.URL.Path+" "+req.Header.Get("Authorization"))
		r.mu.Unlock()
		if reject {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(model.ErrorResponse{Error: "missing or unknown agent token"})
			return
		}
		if r.failReports {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(model.ErrorResponse{Error: "no such delivery"})
			return
		}
		w.Write([]byte("{}"))
	})
	mux.HandleFunc("/v1/stream", func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		reject := r.rejected > 0
		if reject {
			r.rejected--
		}
		r.streams = append(r.streams, req.Header.Get("Authorization"))
		r.mu.Unlock()
		if reject {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(model.ErrorResponse{Error: "missing or unknown agent token"})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: delivery\ndata: {\"id\":\"d-1\",\"kind\":\"challenge\",\"text\":\"code\"}\n\n"))
	})
	return mux
}

func newSession(t *testing.T, hub *httptest.Server, secret string) (*Session, Env) {
	t.Helper()
	dir := t.TempDir()
	env := Env{ConfigDir: dir, Host: "mac", Getenv: func(k string) string {
		return map[string]string{"LANDLINE_URL": hub.URL, "LANDLINE_SECRET": secret, "CLAUDE_CODE_SESSION_ID": "cc-1"}[k]
	}}
	detect := func() Identity { return Identity{Seed: "threnos", Folder: "threnos", Branch: "main"} }
	return NewSession(env, detect, Identity{Project: "lead", LandlineVersion: "0.2.0"}), env
}

func TestTheSessionRegistersOnFirstUseWithTheSeedTheAttributesAndTheClientInfo(t *testing.T) {
	t.Parallel()
	hub := &recordingHub{}
	srv := httptest.NewServer(hub.handler())
	defer srv.Close()
	s, env := newSession(t, srv, "0123456789abcdef")
	s.SetClient("claude-code", "2.1.296")
	if len(hub.register) != 0 {
		t.Fatal("registered before it was needed")
	}
	if _, err := s.Send(model.SendBody{To: "x", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	got := hub.register[0]
	if got.Seed != "threnos" || got.ID != "" || got.Harness != "claude-code" || got.HarnessVersion != "2.1.296" || got.Host != "mac" ||
		got.Session != "cc-1" || got.Folder != "threnos" || got.Branch != "main" || got.Project != "lead" || got.LandlineVersion != "0.2.0" {
		t.Fatalf("%+v", got)
	}
	if s.ID() != "threnos-a" {
		t.Fatalf("id %q", s.ID())
	}
	path := filepath.Join(env.ConfigDir, "session-threnos-a.json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("session file %v %v", info, err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "0123456789abcdef") {
		t.Fatalf("the session file holds the hub secret: %s", raw)
	}
	s.Close()
	if _, err := os.Stat(path); err == nil {
		t.Fatal("the session file was not removed on close")
	}
}

func TestAnExpiredTokenIsRenewedOnceAndTheCallRetried(t *testing.T) {
	t.Parallel()
	hub := &recordingHub{rejected: 1}
	srv := httptest.NewServer(hub.handler())
	defer srv.Close()
	s, _ := newSession(t, srv, "0123456789abcdef")
	if _, err := s.Send(model.SendBody{To: "x", Body: "b"}); err != nil {
		t.Fatalf("%v", err)
	}
	if len(hub.register) != 2 || s.ID() != "threnos-b" {
		t.Fatalf("registrations %d id %q", len(hub.register), s.ID())
	}
	hub.mu.Lock()
	hub.rejected = 5
	hub.mu.Unlock()
	if _, err := s.Send(model.SendBody{To: "x", Body: "b"}); err == nil {
		t.Fatal("a permanent 401 should end as an error after one retry")
	}
}

func TestAHubThatIsNotRunningGivesAnErrorThatSaysWhatToDo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := Env{ConfigDir: dir, Host: "mac", Getenv: func(k string) string {
		return map[string]string{"LANDLINE_URL": "http://127.0.0.1:1", "LANDLINE_SECRET": "0123456789abcdef"}[k]
	}}
	s := NewSession(env, func() Identity { return Identity{Seed: "x", Folder: "x"} }, Identity{})
	_, err := s.Agents()
	if err == nil || !strings.Contains(err.Error(), "landline server up") || !strings.Contains(err.Error(), "http://127.0.0.1:1") {
		t.Fatalf("%v", err)
	}
}

func TestNoSecretGivesAnErrorThatSaysHowToGetOne(t *testing.T) {
	t.Parallel()
	env := Env{ConfigDir: t.TempDir(), Host: "mac", Getenv: func(string) string { return "" }}
	s := NewSession(env, func() Identity { return Identity{Seed: "x"} }, Identity{})
	if _, err := s.Agents(); err == nil || !strings.Contains(err.Error(), "landline server join") {
		t.Fatalf("%v", err)
	}
}

func TestDeliveriesRegistersLazilyAndHandsOverEachDelivery(t *testing.T) {
	t.Parallel()
	hub := &recordingHub{}
	srv := httptest.NewServer(hub.handler())
	defer srv.Close()
	s, _ := newSession(t, srv, "0123456789abcdef")
	var got []model.Delivery
	if err := s.Deliveries(context.Background(), func(d model.Delivery) { got = append(got, d) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "d-1" || got[0].Kind != model.DeliveryChallenge || len(hub.register) != 1 || hub.streams[0] != "Bearer tok1" {
		t.Fatalf("got %+v registrations %d streams %v", got, len(hub.register), hub.streams)
	}
}

func TestDeliveriesRenewsAnExpiredTokenOnceAndOpensTheStreamAgain(t *testing.T) {
	t.Parallel()
	hub := &recordingHub{rejected: 1}
	srv := httptest.NewServer(hub.handler())
	defer srv.Close()
	s, _ := newSession(t, srv, "0123456789abcdef")
	count := 0
	if err := s.Deliveries(context.Background(), func(model.Delivery) { count++ }); err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(hub.register) != 2 || hub.streams[1] != "Bearer tok2" {
		t.Fatalf("count %d registrations %d streams %v", count, len(hub.register), hub.streams)
	}
	hub.mu.Lock()
	hub.rejected = 5
	hub.mu.Unlock()
	if err := s.Deliveries(context.Background(), func(model.Delivery) {}); err == nil {
		t.Fatal("a permanent 401 should end as an error after one retry")
	}
}

func TestReportRegistersLazilyAndSendsTheReportWithTheToken(t *testing.T) {
	t.Parallel()
	hub := &recordingHub{}
	srv := httptest.NewServer(hub.handler())
	defer srv.Close()
	s, _ := newSession(t, srv, "0123456789abcdef")
	if err := s.Report("d-1", model.DeliveryReport{OK: true}); err != nil {
		t.Fatal(err)
	}
	if len(hub.register) != 1 || hub.reports[0] != "/v1/deliveries/d-1/report Bearer tok1" {
		t.Fatalf("registrations %d reports %v", len(hub.register), hub.reports)
	}
}

func TestReportRenewsAnExpiredTokenOnceAndRetries(t *testing.T) {
	t.Parallel()
	hub := &recordingHub{rejected: 1}
	srv := httptest.NewServer(hub.handler())
	defer srv.Close()
	s, _ := newSession(t, srv, "0123456789abcdef")
	if err := s.Report("d-1", model.DeliveryReport{OK: false, Error: "no way to receive"}); err != nil {
		t.Fatal(err)
	}
	if len(hub.register) != 2 || len(hub.reports) != 2 || !strings.HasSuffix(hub.reports[1], "Bearer tok2") {
		t.Fatalf("registrations %d reports %v", len(hub.register), hub.reports)
	}
}

func TestReportMapsAHubRefusalToAStatusError(t *testing.T) {
	t.Parallel()
	hub := &recordingHub{failReports: true}
	srv := httptest.NewServer(hub.handler())
	defer srv.Close()
	s, _ := newSession(t, srv, "0123456789abcdef")
	err := s.Report("d-9", model.DeliveryReport{OK: true})
	var status *client.StatusError
	if !errors.As(err, &status) || status.Status != 404 || !strings.Contains(err.Error(), "no such delivery") {
		t.Fatalf("%v", err)
	}
}

func TestDeliveriesAndReportToAHubThatIsNotRunningSayWhatToDo(t *testing.T) {
	t.Parallel()
	env := Env{ConfigDir: t.TempDir(), Host: "mac", Getenv: func(k string) string {
		return map[string]string{"LANDLINE_URL": "http://127.0.0.1:1", "LANDLINE_SECRET": "0123456789abcdef"}[k]
	}}
	s := NewSession(env, func() Identity { return Identity{Seed: "x", Folder: "x"} }, Identity{})
	if err := s.Report("d-1", model.DeliveryReport{OK: true}); err == nil || !strings.Contains(err.Error(), "landline server up") {
		t.Fatalf("%v", err)
	}
	if err := s.Deliveries(context.Background(), func(model.Delivery) {}); err == nil || !strings.Contains(err.Error(), "landline server up") {
		t.Fatalf("%v", err)
	}
}

func TestTheEnvironmentSuppliesTheProjectModelAndEffort(t *testing.T) {
	t.Parallel()
	got := EnvironmentIdentity(Env{Getenv: func(k string) string {
		return map[string]string{"LANDLINE_PROJECT": "tutor", "LANDLINE_MODEL": "gpt-6.1-sol", "LANDLINE_EFFORT": "medium", "LANDLINE_FOLDER": "f"}[k]
	}})
	if got.Project != "tutor" || got.Model != "gpt-6.1-sol" || got.Effort != "medium" || got.Folder != "f" {
		t.Fatalf("%+v", got)
	}
}

func TestAClaimMadeThroughAttachSurvivesARegistrationAfterTheHubForgetsTheAgent(t *testing.T) {
	t.Parallel()
	hub := &recordingHub{}
	srv := httptest.NewServer(hub.handler())
	defer srv.Close()
	s, _ := newSession(t, srv, "0123456789abcdef")
	if _, err := s.Attach(model.AttachRequest{Model: "claude-opus-5-5", Effort: "high", Project: "tutor"}); err != nil {
		t.Fatal(err)
	}
	hub.mu.Lock()
	hub.rejected = 1
	hub.mu.Unlock()
	if _, err := s.Send(model.SendBody{To: "x", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	last := hub.register[len(hub.register)-1]
	if len(hub.register) != 2 || last.Model != "claude-opus-5-5" || last.Effort != "high" || last.Project != "tutor" {
		t.Fatalf("%d registrations, the last %+v", len(hub.register), last)
	}
}

func TestRenewingATokenRemovesTheSessionFileOfTheHandleItReplaced(t *testing.T) {
	t.Parallel()
	hub := &recordingHub{}
	srv := httptest.NewServer(hub.handler())
	defer srv.Close()
	s, env := newSession(t, srv, "0123456789abcdef")
	if _, err := s.Send(model.SendBody{To: "x", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(env.ConfigDir, "session-threnos-a.json")
	if _, err := os.Stat(old); err != nil {
		t.Fatal(err)
	}
	hub.mu.Lock()
	hub.rejected = 1
	hub.mu.Unlock()
	if _, err := s.Send(model.SendBody{To: "x", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); err == nil {
		t.Fatal("the file of the replaced handle is still there")
	}
	if _, err := os.Stat(filepath.Join(env.ConfigDir, "session-threnos-b.json")); err != nil {
		t.Fatalf("the new session file is missing: %v", err)
	}
	os.Remove(filepath.Join(env.ConfigDir, "session-threnos-b.json"))
	hub.mu.Lock()
	hub.rejected = 1
	hub.mu.Unlock()
	if _, err := s.Send(model.SendBody{To: "x", Body: "b"}); err != nil {
		t.Fatalf("a missing old file broke the renewal: %v", err)
	}
}

func TestTheSessionSaysTheSwitchboardIsNotRunningWhenItCannotConnect(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	s, _ := newSession(t, srv, "0123456789abcdef")
	_, err := s.Send(model.SendBody{To: "x", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "the Landline switchboard is not running at "+url+". Run: landline server up") {
		t.Fatalf("%v", err)
	}
}

func TestTheSessionSaysThereIsNoSwitchboardSecret(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	s, _ := newSession(t, srv, "")
	_, err := s.Send(model.SendBody{To: "x", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "there is no switchboard secret. Run: landline server up, or landline server join <host:port>") {
		t.Fatalf("%v", err)
	}
}

func TestTheDefaultProjectComesFromTheRepositoryOrTheFolder(t *testing.T) {
	t.Parallel()
	gitAnswering := func(common string, err error) GitRunner {
		return func(dir string, args ...string) (string, error) {
			if len(args) == 2 && args[1] == "--git-common-dir" {
				return common, err
			}
			return "", errors.New("not asked")
		}
	}
	for name, c := range map[string]struct {
		dir, common string
		err         error
		want        string
	}{
		"main tree, relative":         {"/w/foo", ".git", nil, "foo"},
		"subfolder, relative":         {"/w/foo/sub", "../.git", nil, "foo"},
		"linked worktree, absolute":   {"/w/wt", "/w/foo/.git", nil, "foo"},
		"bare repository":             {"/w/foo.git", "/w/foo.git", nil, "foo"},
		"separate git folder":         {"/w/elsewhere", "/w/store.git", nil, "store"},
		"not a repository":            {"/w/plain-folder", "", errors.New("fatal"), "plain-folder"},
		"root, not a repository":      {"/", "", errors.New("fatal"), ""},
		"empty folder, no repository": {"", "", errors.New("fatal"), ""},
	} {
		if got := DetectIdentity(Env{Getenv: func(string) string { return "" }}, c.dir, gitAnswering(c.common, c.err)).Project; got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

func TestTheSessionRemembersWhetherTheProjectWasGivenOrDerived(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer((&recordingHub{}).handler())
	defer srv.Close()
	env := Env{ConfigDir: t.TempDir(), Host: "mac", Getenv: func(k string) string {
		return map[string]string{"LANDLINE_URL": srv.URL, "LANDLINE_SECRET": "0123456789abcdef"}[k]
	}}
	s := NewSession(env, func() Identity { return Identity{Seed: "x", Project: "derived"} }, Identity{})
	if _, err := s.Attach(model.AttachRequest{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if project, given := s.Project(); project != "derived" || given {
		t.Fatalf("%q %v", project, given)
	}
	if _, err := s.Attach(model.AttachRequest{Project: "chosen"}); err != nil {
		t.Fatal(err)
	}
	if project, given := s.Project(); project != "chosen" || !given {
		t.Fatalf("%q %v", project, given)
	}
}

func TestARenewalReadsTheSavedSecretAgainAfterItWasRotated(t *testing.T) {
	t.Parallel()
	hub := &recordingHub{}
	var mu sync.Mutex
	var bearers []string
	inner := hub.handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/register" {
			mu.Lock()
			bearers = append(bearers, r.Header.Get("Authorization"))
			mu.Unlock()
		}
		inner.ServeHTTP(w, r)
	}))
	defer srv.Close()
	dir := t.TempDir()
	host, port, _ := splitSwitchboardAddress(srv.URL)
	env := Env{ConfigDir: dir, Host: "mac", Getenv: func(string) string { return "" }}
	if err := SaveConfig(env, Config{Host: host, Port: port, Secret: "old-secret-0123456789"}); err != nil {
		t.Fatal(err)
	}
	s := NewSession(env, func() Identity { return Identity{Seed: "x", Folder: "x"} }, Identity{})
	if _, err := s.Send(model.SendBody{To: "y", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(env, Config{Secret: "new-secret-0123456789"}); err != nil {
		t.Fatal(err)
	}
	hub.mu.Lock()
	hub.rejected = 1
	hub.mu.Unlock()
	if _, err := s.Send(model.SendBody{To: "y", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bearers) != 2 || bearers[0] != "Bearer old-secret-0123456789" || bearers[1] != "Bearer new-secret-0123456789" {
		t.Fatalf("register secrets: %q", bearers)
	}
}
