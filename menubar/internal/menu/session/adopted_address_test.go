package session

import (
	"strings"
	"testing"
)

const (
	startupAddress = "http://127.0.0.1:8787"
	adoptedAddress = "http://192.0.2.1:8787"
)

func TestAfterTheClientAdoptsAnotherAddressTheMenuJudgesLocalityByTheAddressInUse(t *testing.T) {
	s := newSession()
	s.isLocal = func(address string) bool { return address == startupAddress }
	s.Apply(Observe(source{address: startupAddress}, true))
	if got := firstLine(s); got != "Running at "+startupAddress {
		t.Fatalf("before the change: %q", got)
	}

	s.Apply(Observe(source{address: adoptedAddress}, true))
	text := s.Menu().Text()
	if got := firstLine(s); got != "Remote switchboard at "+adoptedAddress {
		t.Errorf("the client now talks to %s, and the menu says %q", adoptedAddress, got)
	}
	for _, item := range []string{"Start", "Stop", "Restart", "Rotate secret…"} {
		if !strings.Contains(text, "  "+item+" (disabled)\n") {
			t.Errorf("%s is enabled for the remote switchboard at %s:\n%s", item, adoptedAddress, text)
		}
	}
	if strings.Contains(text, startupAddress) {
		t.Errorf("the menu still names the startup address %s:\n%s", startupAddress, text)
	}
}
