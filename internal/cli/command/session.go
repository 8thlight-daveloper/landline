package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	client "github.com/8thlight-daveloper/landline/internal/client/http"
	"github.com/8thlight-daveloper/landline/internal/model"
)

type Identity struct {
	Seed           string
	Folder         string
	Branch         string
	Worktree       string
	Harness        string
	HarnessVersion string
	Model          string
	Effort         string
	Label          string
}

type GitRunner func(dir string, args ...string) (string, error)

func DetectIdentity(env Env, dir string, git GitRunner) Identity {
	id := Identity{Folder: filepath.Base(dir)}
	id.Seed = id.Folder
	if branch, err := git(dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && branch != "HEAD" {
		id.Branch = branch
	}
	gitDir, errOne := git(dir, "rev-parse", "--git-dir")
	common, errTwo := git(dir, "rev-parse", "--git-common-dir")
	if errOne == nil && errTwo == nil && gitDir != common {
		if top, err := git(dir, "rev-parse", "--show-toplevel"); err == nil {
			id.Worktree = filepath.Base(top)
		}
	}
	id.Effort = env.Getenv("CLAUDE_EFFORT")
	return id
}

func (base Identity) With(over Identity) Identity {
	pick := func(a, b string) string {
		if a != "" {
			return a
		}
		return b
	}
	return Identity{
		Seed: pick(over.Seed, base.Seed), Folder: pick(over.Folder, base.Folder), Branch: pick(over.Branch, base.Branch),
		Worktree: pick(over.Worktree, base.Worktree), Harness: pick(over.Harness, base.Harness),
		HarnessVersion: pick(over.HarnessVersion, base.HarnessVersion), Model: pick(over.Model, base.Model),
		Effort: pick(over.Effort, base.Effort), Label: pick(over.Label, base.Label),
	}
}

func EnvironmentIdentity(env Env) Identity {
	return Identity{
		Folder: env.Getenv("LANDLINE_FOLDER"), Model: env.Getenv("LANDLINE_MODEL"),
		Effort: env.Getenv("LANDLINE_EFFORT"), Label: env.Getenv("LANDLINE_LABEL"),
	}
}

func HarnessFromClient(name, aiAgent string) string {
	switch {
	case name == "claude-code":
		return "claude-code"
	case name == "codex-mcp-client":
		return "codex"
	case name != "":
		return name
	case aiAgent != "":
		return strings.SplitN(aiAgent, "_", 2)[0]
	}
	return "mcp"
}

type Session struct {
	env                       Env
	mu                        sync.Mutex
	identity                  Identity
	overrides                 Identity
	client                    *client.Client
	handle                    string
	clientName, clientVersion string
	detect                    func() Identity
}

func NewSession(env Env, detect func() Identity, overrides Identity) *Session {
	return &Session{env: withConfig(env), detect: detect, overrides: overrides.With(EnvironmentIdentity(withConfig(env)))}
}

func (s *Session) SetClient(name, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientName, s.clientVersion = name, version
}

func (s *Session) Handle() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handle
}

func (s *Session) hubDown(err error) error {
	if err != nil && (strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "dial tcp")) {
		return fmt.Errorf("the Landline hub is not running at %s. Run: landline server up", HubURL(s.env))
	}
	return err
}

func (s *Session) register() error {
	secret := s.env.Getenv("LANDLINE_SECRET")
	if secret == "" {
		return errors.New("there is no hub secret. Run: landline server up, or landline join <hub address>")
	}
	host := s.env.Host
	identity := s.detect().With(Identity{Harness: HarnessFromClient(s.clientName, s.env.Getenv("AI_AGENT")), HarnessVersion: s.clientVersion}).With(s.overrides)
	url := HubURL(s.env)
	out, err := client.New(url, secret).Register(model.RegisterRequest{
		Seed: identity.Seed, Harness: identity.Harness, Host: host, Session: s.env.Getenv("CLAUDE_CODE_SESSION_ID"),
		Folder: identity.Folder, Branch: identity.Branch, Worktree: identity.Worktree, HarnessVersion: identity.HarnessVersion,
		Model: identity.Model, Effort: identity.Effort, Label: identity.Label,
	})
	if err != nil {
		return s.hubDown(err)
	}
	s.identity, s.handle, s.client = identity, out.Name, client.New(url, out.Token)
	raw, _ := json.Marshal(session{URL: url, Token: out.Token})
	if os.MkdirAll(s.env.ConfigDir, 0o700) == nil {
		path := filepath.Join(s.env.ConfigDir, "session-"+out.Name+".json")
		if os.WriteFile(path, raw, 0o600) == nil {
			os.Chmod(path, 0o600)
		}
	}
	return nil
}

func (s *Session) current() (*client.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		if err := s.register(); err != nil {
			return nil, err
		}
	}
	return s.client, nil
}

func (s *Session) renewed(stale *client.Client) (*client.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == stale {
		if err := s.register(); err != nil {
			return nil, err
		}
	}
	return s.client, nil
}

func (s *Session) call(do func(*client.Client) error) error {
	current, err := s.current()
	if err != nil {
		return err
	}
	err = do(current)
	var status *client.StatusError
	if errors.As(err, &status) && status.Status == 401 {
		fresh, registerErr := s.renewed(current)
		if registerErr != nil {
			return registerErr
		}
		err = do(fresh)
	}
	return s.hubDown(err)
}

func (s *Session) Send(body model.SendBody) (out model.SendResponse, err error) {
	err = s.call(func(c *client.Client) (e error) { out, e = c.Send(body); return })
	return
}

func (s *Session) Deliveries(ctx context.Context, handle func(model.Delivery)) error {
	return s.call(func(c *client.Client) error { return c.Stream(ctx, handle) })
}

func (s *Session) Report(id string, report model.DeliveryReport) error {
	return s.call(func(c *client.Client) error { return c.Report(id, report) })
}

func (s *Session) Agents() (out []model.AgentInfo, err error) {
	err = s.call(func(c *client.Client) (e error) { out, e = c.Agents(); return })
	return
}

func (s *Session) Attach(req model.AttachRequest) (out model.AttachResponse, err error) {
	err = s.call(func(c *client.Client) (e error) { out, e = c.Attach(req); return })
	return
}

func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != "" {
		os.Remove(filepath.Join(s.env.ConfigDir, "session-"+s.handle+".json"))
	}
}
