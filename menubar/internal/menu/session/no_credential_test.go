package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/menubar/internal/hubclient"
)

const noCredentialLine = "No switchboard credential"

const longNoCredentialFailure = "Failed: there is no switchboard credential. Run landline server join <host:port> --code <code>, or set LANDLINE_SECRET"

func stateLine(s *Session) string {
	lines := strings.Split(s.Menu().Text(), "\n")
	if len(lines) < 2 {
		return ""
	}
	return strings.TrimSuffix(lines[1], " (disabled)")
}

func TestWithNoCredentialTheStateLineIsTheShortNoCredentialLine(t *testing.T) {
	s := newSession()
	s.Apply(Observe(source{agentsErr: hubclient.ErrNoSecret}, true))
	if got := stateLine(s); got != noCredentialLine {
		t.Fatalf("state line %q, want %q", got, noCredentialLine)
	}
}

func TestAnActionThatFailsWhileThereIsNoCredentialShowsTheShortLineWhateverTheCommandPrinted(t *testing.T) {
	for _, printed := range []string{
		longNoCredentialFailure,
		"Failed: Switchboard unreachable",
		"Failed: cannot listen on alpha-host:8787",
		"Failed: exit status 1",
	} {
		s := newSession()
		s.Apply(Observe(source{agentsErr: hubclient.ErrNoSecret}, true))
		s.NotifyFailure(printed)
		if got := firstLine(s); got != noCredentialLine {
			t.Errorf("the command printed %q: notice %q, want %q", printed, got, noCredentialLine)
		}
	}
}

func TestAnActionThatSucceedsWhileThereIsNoCredentialShowsItsDoneTextAsGiven(t *testing.T) {
	const done = "Codex set up. Start a new session."
	s := newSession()
	s.Apply(Observe(source{agentsErr: hubclient.ErrNoSecret}, true))
	s.Notify(done)
	if got := firstLine(s); got != done {
		t.Fatalf("notice %q, want the done text %q", got, done)
	}
}

func TestAnActionThatFailsWhileACredentialExistsShowsWhatTheCommandPrintedEvenWhenItNamesNoCredential(t *testing.T) {
	s := newSession()
	s.Apply(Observe(source{agentsErr: errors.New("dial tcp 127.0.0.1:8787: connection refused")}, true))
	s.NotifyFailure(longNoCredentialFailure)
	if got := firstLine(s); got != longNoCredentialFailure {
		t.Fatalf("notice %q, want the command's text %q, because the menu bar holds a credential", got, longNoCredentialFailure)
	}
}
