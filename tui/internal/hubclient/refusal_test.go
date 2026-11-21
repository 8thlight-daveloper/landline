package hubclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const switchboardClockMessage = "the clock of this machine differs from the switchboard's by 75 seconds. Set the clock"

func refusingHub(t *testing.T, message string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
	}))
	t.Cleanup(server.Close)
	return server
}

func machineCredential(t *testing.T) Credential {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return Credential{Machine: "linux-box", Key: key}
}

func TestAClockRefusalOfASignedRegistrationShowsTheSwitchboardsClockMessageAndDoesNotSayToJoinAgain(t *testing.T) {
	server := refusingHub(t, switchboardClockMessage)
	_, err := NewWith(server.URL, machineCredential(t)).Agents()
	if err == nil {
		t.Fatal("a 401 with the clock message was not an error")
	}
	if !strings.Contains(err.Error(), switchboardClockMessage) {
		t.Errorf("the error leaves out the switchboard's clock message, which names the difference in seconds and says to set the clock: %q", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "join") || strings.Contains(err.Error(), "--code") {
		t.Errorf("a clock refusal says to join again, which a person with a working key must not do: %q", err)
	}
}
