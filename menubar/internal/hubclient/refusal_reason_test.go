package hubclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	formerGenericRefusal = "missing or wrong switchboard secret or machine signature"
	formerClockMessage   = "the clock of this machine differs from the switchboard's by 75 seconds. Set the clock"
)

type advice int

const (
	genericAdvice advice = iota
	clockAdvice
)

type reasonCase struct {
	name          string
	body          string
	registrations int32
	advice        advice
	shownAsGiven  string
}

var reasonCases = []reasonCase{
	{"refused with a reworded message", `{"error":"the switchboard turned this request away","reason":"refused"}`, 2, genericAdvice, ""},
	{"refused with the former clock text", `{"error":"` + formerClockMessage + `","reason":"refused"}`, 2, genericAdvice, ""},
	{"clock ahead with the former generic text", `{"error":"` + formerGenericRefusal + `","reason":"clock","clock_difference":437}`, 1, clockAdvice, ""},
	{"clock with the switchboard's own clock message", `{"error":"the clock of this machine differs from the switchboard's by 437 seconds. Set the clock","reason":"clock","clock_difference":437}`, 1, clockAdvice, ""},
	{"clock behind with the former generic text", `{"error":"` + formerGenericRefusal + `","reason":"clock","clock_difference":-437}`, 1, clockAdvice, ""},
	{"an unknown reason", `{"error":"` + formerClockMessage + `","reason":"weather","clock_difference":75}`, 2, genericAdvice, formerClockMessage},
	{"no reason with the former clock text", `{"error":"` + formerClockMessage + `"}`, 2, genericAdvice, formerClockMessage},
	{"no reason with the former generic text", `{"error":"` + formerGenericRefusal + `"}`, 2, genericAdvice, formerGenericRefusal},
	{"clock with no difference", `{"error":"` + formerClockMessage + `","reason":"clock"}`, 2, genericAdvice, formerClockMessage},
	{"clock with a difference in quotes", `{"error":"` + formerClockMessage + `","reason":"clock","clock_difference":"75"}`, 2, genericAdvice, formerClockMessage},
	{"clock with a fractional difference", `{"error":"` + formerClockMessage + `","reason":"clock","clock_difference":75.5}`, 2, genericAdvice, formerClockMessage},
	{"clock with a null difference", `{"error":"` + formerClockMessage + `","reason":"clock","clock_difference":null}`, 2, genericAdvice, formerClockMessage},
	{"a reason that is not text", `{"error":"` + formerClockMessage + `","reason":7,"clock_difference":75}`, 2, genericAdvice, formerClockMessage},
}

func reasonRefusingHub(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var registrations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/observers" && strings.HasPrefix(r.Header.Get("Authorization"), "Landline-Machine ") {
			registrations.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &registrations
}

var reasonKey = sync.OnceValue(func() ed25519.PrivateKey {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	return key
})

func checkAdvice(t *testing.T, each reasonCase, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("a 401 to a signed registration was not an error")
	}
	text := strings.ToLower(err.Error())
	switch each.advice {
	case clockAdvice:
		if !strings.Contains(text, "clock") || !strings.Contains(text, "437") {
			t.Errorf("the clock advice does not name the clock and the difference of 437 seconds from clock_difference: %q", err)
		}
		for _, wrong := range []string{"join", "--code", "revoke", "rotate"} {
			if strings.Contains(text, wrong) {
				t.Errorf("the clock advice says %q, which belongs to the generic advice: %q", wrong, err)
			}
		}
	case genericAdvice:
		for _, cause := range []string{"rotate", "revoke", "clock", "join"} {
			if !strings.Contains(text, cause) {
				t.Errorf("the generic advice leaves out %q; it names a rotated secret, a revoked machine, and a clock that runs behind, and says to join again only if the refusal persists: %q", cause, err)
			}
		}
	}
	if each.shownAsGiven != "" && !strings.Contains(err.Error(), each.shownAsGiven) {
		t.Errorf("the error does not show the switchboard's message as given, %q: %q", each.shownAsGiven, err)
	}
}

func TestARefusedSignedRegistrationRetriesAndAdvisesByTheReasonAndNeverByTheMessageText(t *testing.T) {
	for _, each := range reasonCases {
		t.Run(each.name, func(t *testing.T) {
			t.Parallel()
			server, registrations := reasonRefusingHub(t, each.body)
			_, err := NewWith(server.URL, Credential{Machine: "linux-box", Key: reasonKey()}).Agents()
			if got := registrations.Load(); got != each.registrations {
				t.Errorf("%d signed registrations, want %d: %s", got, each.registrations, each.body)
			}
			checkAdvice(t, each, err)
		})
	}
}
