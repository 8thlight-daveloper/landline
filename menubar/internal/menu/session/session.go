package session

import (
	"errors"
	"sync"

	"github.com/8thlight-daveloper/landline/menubar/internal/agentsview"
	"github.com/8thlight-daveloper/landline/menubar/internal/buildinfo"
	"github.com/8thlight-daveloper/landline/menubar/internal/hubclient"
	"github.com/8thlight-daveloper/landline/menubar/internal/menu/model"
)

type Source interface {
	Agents() ([]hubclient.Agent, error)
	Build() (hubclient.Build, bool, error)
	Address() string
}

type Observed struct {
	Address           string
	Reachable         bool
	KeyRefusal        string
	CredentialProblem string
	NoCredential      bool
	Agents            []agentsview.Row
	Switchboard       model.Switchboard
	LandlineFound     bool
}

func Observe(source Source, landlineFound bool) Observed {
	observed := Observed{LandlineFound: landlineFound, Address: source.Address()}
	agents, err := source.Agents()
	var refusal hubclient.KeyRefusal
	if errors.As(err, &refusal) {
		observed.KeyRefusal = refusal.Text
	}
	var problem hubclient.CredentialProblem
	if errors.As(err, &problem) {
		observed.CredentialProblem = problem.Text
	}
	observed.NoCredential = errors.Is(err, hubclient.ErrNoSecret)
	if err != nil {
		return observed
	}
	observed.Reachable = true
	observed.Agents = agentsview.Build(agents)
	if build, known, err := source.Build(); err == nil && known {
		observed.Switchboard = model.Switchboard{
			Known: true,
			Build: buildinfo.Clean(buildinfo.Build{Version: build.Version, Commit: build.Commit}),
		}
	}
	return observed
}

type Session struct {
	mu       sync.Mutex
	address  string
	app      buildinfo.Build
	observed Observed
	busy     bool
	notice   string
	skip     bool
	local    bool
	isLocal  func(string) bool
}

func New(address string, app buildinfo.Build) *Session {
	return &Session{address: address, app: app, isLocal: thisMachinesRemembered()}
}

func (s *Session) Apply(observed Observed) {
	s.mu.Lock()
	address := s.address
	s.mu.Unlock()
	if observed.Address != "" {
		address = observed.Address
	}
	local := s.isLocal(address)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.address, s.observed, s.local = address, observed, local
	switch {
	case s.skip:
		s.skip = false
	case observed.KeyRefusal != "":
		s.notice = observed.KeyRefusal
	case observed.Reachable:
		s.notice = ""
	}
}

func (s *Session) Notify(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notice, s.skip = text, true
}

func (s *Session) NotifyFailure(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observed.NoCredential {
		text = model.NoCredentialLine
	}
	s.notice, s.skip = text, true
}

func (s *Session) Start() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return false
	}
	s.busy = true
	return true
}

func (s *Session) Finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy = false
}

func (s *Session) Menu() model.Menu {
	s.mu.Lock()
	defer s.mu.Unlock()
	return model.Build(model.Input{
		Address: s.address, Local: s.local, Reachable: s.observed.Reachable, Agents: s.observed.Agents,
		Switchboard: s.observed.Switchboard, App: s.app, LandlineFound: s.observed.LandlineFound,
		Busy: s.busy, Notice: s.notice, CredentialProblem: s.observed.CredentialProblem, NoCredential: s.observed.NoCredential,
	})
}
