package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/menubar/internal/buildinfo"
	"github.com/8thlight-daveloper/landline/menubar/internal/hubclient"
)

type source struct {
	agents    []hubclient.Agent
	agentsErr error
	build     hubclient.Build
	known     bool
	buildErr  error
}

func (s source) Agents() ([]hubclient.Agent, error)    { return s.agents, s.agentsErr }
func (s source) Build() (hubclient.Build, bool, error) { return s.build, s.known, s.buildErr }

func TestAnyFailureToListAgentsIsTheUnreachableState(t *testing.T) {
	got := Observe(source{agentsErr: errors.New("boom"), known: true, build: hubclient.Build{Version: "1"}}, true)
	if got.Reachable || len(got.Agents) != 0 || got.Switchboard.Known || !got.LandlineFound {
		t.Fatalf("%+v", got)
	}
}

func TestABuildFailureAloneKeepsTheSwitchboardReachableWithAnUnknownBuild(t *testing.T) {
	for name, s := range map[string]source{
		"error":   {buildErr: errors.New("boom")},
		"unknown": {known: false},
	} {
		s.agents = []hubclient.Agent{{Name: "alpha", Connected: true}}
		got := Observe(s, false)
		if !got.Reachable || len(got.Agents) != 1 || got.Switchboard.Known {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestTheSwitchboardBuildIsCleanedBeforeItIsKept(t *testing.T) {
	got := Observe(source{known: true, build: hubclient.Build{Version: "1\x1b[2J\nX", Commit: "ab\tcdefghij"}}, true)
	if got.Switchboard.Build.Version != "1 [2J X" || got.Switchboard.Build.Commit != "ab cdefghij" {
		t.Fatalf("%+v", got.Switchboard)
	}
}

func newSession() *Session {
	return New("http://127.0.0.1:8787", buildinfo.Build{Version: "dev"})
}

func firstLine(s *Session) string {
	for _, line := range strings.Split(s.Menu().Text(), "\n") {
		if strings.HasPrefix(line, "  ") && strings.HasSuffix(line, " (disabled)") {
			return strings.TrimSuffix(strings.TrimSpace(line), " (disabled)")
		}
	}
	return ""
}

func TestANoticeSurvivesThePollAfterTheCommandAndGoesOnTheNextSuccessfulPoll(t *testing.T) {
	s := newSession()
	up := Observed{Reachable: true, LandlineFound: true}
	s.Apply(up)
	s.Notify("Failed: boom")
	s.Apply(up)
	if got := firstLine(s); got != "Failed: boom" {
		t.Fatalf("after the immediate poll: %q", got)
	}
	s.Apply(Observed{LandlineFound: true})
	if got := firstLine(s); got != "Failed: boom" {
		t.Fatalf("an unreachable poll cleared the notice: %q", got)
	}
	s.Apply(up)
	if got := firstLine(s); got != "Running at http://127.0.0.1:8787" {
		t.Fatalf("after a successful poll: %q", got)
	}
}

func TestOnlyOneCommandRunsAtATimeAndBusyDisablesTheItems(t *testing.T) {
	s := newSession()
	s.Apply(Observed{Reachable: true, LandlineFound: true})
	if !s.Start() || s.Start() {
		t.Fatal("the second start must be refused")
	}
	if text := s.Menu().Text(); !strings.Contains(text, "  Stop (disabled)\n") || !strings.Contains(text, "  Set up Codex (disabled)\n") {
		t.Fatalf("busy menu:\n%s", text)
	}
	s.Finish()
	if !s.Start() {
		t.Fatal("finish did not release")
	}
}
