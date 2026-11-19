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
	Seed            string
	Folder          string
	Branch          string
	Worktree        string
	Harness         string
	HarnessVersion  string
	LandlineVersion string
	LandlineCommit  string
	LandlineDate    string
	Model           string
	Effort          string
	Project         string
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
	id.Project = repositoryProject(dir, git)
	id.Effort = env.Getenv("CLAUDE_EFFORT")
	return id
}

const gitDirectoryName = ".git"

func repositoryProject(dir string, git GitRunner) string {
	common, err := git(dir, "rev-parse", "--git-common-dir")
	if err != nil || common == "" {
		return projectName(filepath.Base(dir))
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	common = filepath.Clean(common)
	if filepath.Base(common) == gitDirectoryName {
		return projectName(filepath.Base(filepath.Dir(common)))
	}
	return projectName(strings.TrimSuffix(filepath.Base(common), gitDirectoryName))
}

func projectName(base string) string {
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
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
		HarnessVersion:  pick(over.HarnessVersion, base.HarnessVersion),
		LandlineVersion: pick(over.LandlineVersion, base.LandlineVersion),
		LandlineCommit:  pick(over.LandlineCommit, base.LandlineCommit), LandlineDate: pick(over.LandlineDate, base.LandlineDate), Model: pick(over.Model, base.Model),
		Effort: pick(over.Effort, base.Effort), Project: pick(over.Project, base.Project),
	}
}

func EnvironmentIdentity(env Env) Identity {
	return Identity{
		Folder: env.Getenv("LANDLINE_FOLDER"), Model: env.Getenv("LANDLINE_MODEL"),
		Effort: env.Getenv("LANDLINE_EFFORT"), Project: env.Getenv("LANDLINE_PROJECT"),
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
	base                      Env
	mu                        sync.Mutex
	identity                  Identity
	overrides                 Identity
	client                    *client.Client
	id                        string
	clientName, clientVersion string
	detect                    func() Identity
}

func NewSession(env Env, detect func() Identity, overrides Identity) *Session {
	return &Session{env: withConfig(env), base: env, detect: detect, overrides: overrides.With(EnvironmentIdentity(withConfig(env)))}
}

func (s *Session) SetClient(name, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientName, s.clientVersion = name, version
}

func (s *Session) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id
}

func (s *Session) hubDown(err error) error {
	if err != nil && (strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "dial tcp")) {
		return fmt.Errorf("the Landline switchboard is not running at %s. Run: landline server up", HubURL(s.env))
	}
	return err
}

func (s *Session) register() error {
	saved := withConfig(s.base)
	if saved.configProblem != nil {
		return saved.configProblem
	}
	secret := saved.Getenv("LANDLINE_SECRET")
	if secret == "" {
		return errors.New("there is no switchboard secret. Run: landline server up, or landline server join <host:port>")
	}
	host := s.env.Host
	identity := s.detect().With(Identity{Harness: HarnessFromClient(s.clientName, s.env.Getenv("AI_AGENT")), HarnessVersion: s.clientVersion}).With(s.overrides)
	url := saved.Getenv("LANDLINE_URL")
	out, err := client.New(url, secret).Register(model.RegisterRequest{
		Seed: identity.Seed, Harness: identity.Harness, Host: host, Session: s.env.Getenv("CLAUDE_CODE_SESSION_ID"),
		Folder: identity.Folder, Branch: identity.Branch, Worktree: identity.Worktree, HarnessVersion: identity.HarnessVersion, LandlineVersion: identity.LandlineVersion,
		LandlineCommit: identity.LandlineCommit, LandlineDate: identity.LandlineDate,
		Model: identity.Model, Effort: identity.Effort, Project: identity.Project,
	})
	if err != nil {
		return s.hubDown(err)
	}
	replaced := s.id
	s.identity, s.id, s.client = identity, out.ID, client.New(url, out.Token)
	raw, _ := json.Marshal(session{URL: url, Token: out.Token})
	if os.MkdirAll(s.env.ConfigDir, 0o700) == nil {
		path := filepath.Join(s.env.ConfigDir, "session-"+out.ID+".json")
		if os.WriteFile(path, raw, 0o600) == nil {
			os.Chmod(path, 0o600)
		}
	}
	if replaced != "" && replaced != out.ID {
		os.Remove(filepath.Join(s.env.ConfigDir, "session-"+replaced+".json"))
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
	if err == nil {
		s.mu.Lock()
		s.overrides = s.overrides.With(Identity{Model: req.Model, Effort: req.Effort, Project: req.Project})
		s.mu.Unlock()
	}
	return
}

func (s *Session) Project() (project string, given bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.overrides.Project != "" {
		return s.overrides.Project, true
	}
	return s.identity.Project, false
}

func (s *Session) Status(req model.StatusRequest) (out model.StatusResponse, err error) {
	err = s.call(func(c *client.Client) (e error) { out, e = c.Status(req); return })
	return
}

func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.id != "" {
		os.Remove(filepath.Join(s.env.ConfigDir, "session-"+s.id+".json"))
	}
}
