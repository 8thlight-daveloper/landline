package session

import (
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/menubar/internal/hubclient"
)

type movingSource struct{ address string }

func (s *movingSource) Agents() ([]hubclient.Agent, error) {
	s.address = adoptedAddress
	return nil, nil
}

func (s *movingSource) Build() (hubclient.Build, bool, error) { return hubclient.Build{}, false, nil }
func (s *movingSource) Address() string                       { return s.address }

func TestThePollInWhichTheClientSwitchesAddressShowsTheAddressInUse(t *testing.T) {
	s := newSession()
	s.isLocal = func(address string) bool { return address == startupAddress }
	s.Apply(Observe(&movingSource{address: startupAddress}, true))
	text := s.Menu().Text()
	if got := firstLine(s); got != "Remote switchboard at "+adoptedAddress {
		t.Errorf("the client moved to %s during this poll's registration, and the menu says %q", adoptedAddress, got)
	}
	for _, item := range []string{"Stop", "Restart", "Rotate secret…"} {
		if !strings.Contains(text, "  "+item+" (disabled)\n") {
			t.Errorf("%s is enabled for the remote switchboard at %s:\n%s", item, adoptedAddress, text)
		}
	}
}
