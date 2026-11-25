package hubclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"github.com/8thlight-daveloper/landline/testsupport/clock/sleeper"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
	name         string
	body         string
	retried      bool
	advice       advice
	shownAsGiven string
}

var reasonCases = []reasonCase{
	{"refused with a reworded message", `{"error":"the switchboard turned this request away","reason":"refused"}`, true, genericAdvice, ""},
	{"refused with the former clock text", `{"error":"` + formerClockMessage + `","reason":"refused"}`, true, genericAdvice, ""},
	{"clock ahead with the former generic text", `{"error":"` + formerGenericRefusal + `","reason":"clock","clock_difference":437}`, false, clockAdvice, ""},
	{"clock with the switchboard's own clock message", `{"error":"the clock of this machine differs from the switchboard's by 437 seconds. Set the clock","reason":"clock","clock_difference":437}`, false, clockAdvice, ""},
	{"clock behind with the former generic text", `{"error":"` + formerGenericRefusal + `","reason":"clock","clock_difference":-437}`, false, clockAdvice, ""},
	{"an unknown reason", `{"error":"` + formerClockMessage + `","reason":"weather","clock_difference":75}`, true, genericAdvice, formerClockMessage},
	{"no reason with the former clock text", `{"error":"` + formerClockMessage + `"}`, true, genericAdvice, formerClockMessage},
	{"no reason with the former generic text", `{"error":"` + formerGenericRefusal + `"}`, true, genericAdvice, formerGenericRefusal},
	{"clock with no difference", `{"error":"` + formerClockMessage + `","reason":"clock"}`, true, genericAdvice, formerClockMessage},
	{"clock with a difference in quotes", `{"error":"` + formerClockMessage + `","reason":"clock","clock_difference":"75"}`, true, genericAdvice, formerClockMessage},
	{"clock with a fractional difference", `{"error":"` + formerClockMessage + `","reason":"clock","clock_difference":75.5}`, true, genericAdvice, formerClockMessage},
	{"clock with a null difference", `{"error":"` + formerClockMessage + `","reason":"clock","clock_difference":null}`, true, genericAdvice, formerClockMessage},
	{"a reason that is not text", `{"error":"` + formerClockMessage + `","reason":7,"clock_difference":75}`, true, genericAdvice, formerClockMessage},
}

type signedRegistration struct {
	at    time.Time
	nonce string
	unix  int64
}

type registrationLog struct {
	mu   sync.Mutex
	seen []signedRegistration
}

func (l *registrationLog) Load() int32 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return int32(len(l.seen))
}

func (l *registrationLog) all() []signedRegistration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.seen)
}

var signedFields = regexp.MustCompile(`time=(-?[0-9]+), nonce=([^,]+),`)

func signedTimeOf(r *http.Request) (int64, string) {
	found := signedFields.FindStringSubmatch(r.Header.Get("Authorization"))
	if found == nil {
		return 0, ""
	}
	unix, _ := strconv.ParseInt(found[1], 10, 64)
	return unix, found[2]
}

const (
	fewestRegistrationsOverTheRetries = 6
	mostRegistrationsOverTheRetries   = 8
)

func reasonRefusingHub(t *testing.T, body string) (*httptest.Server, *registrationLog) {
	t.Helper()
	registrations := &registrationLog{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/observers" && strings.HasPrefix(r.Header.Get("Authorization"), "Landline-Machine ") {
			unix, nonce := signedTimeOf(r)
			registrations.mu.Lock()
			registrations.seen = append(registrations.seen, signedRegistration{at: time.Now(), nonce: nonce, unix: unix})
			registrations.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, registrations
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

func TestARefusedSignedRegistrationRetriesForSixSecondsOrNotAtAllAndAdvisesByTheReasonAndNeverByTheMessageText(t *testing.T) {
	t.Parallel()
	for _, each := range reasonCases {
		t.Run(each.name, func(t *testing.T) {
			t.Parallel()
			server, registrations := reasonRefusingHub(t, each.body)
			began := time.Now()
			_, err := NewWith(server.URL, Credential{Machine: "linux-box", Key: reasonKey()}).Agents()
			took := time.Since(began)
			got := registrations.Load()
			switch {
			case each.retried && (got < fewestRegistrationsOverTheRetries || got > mostRegistrationsOverTheRetries):
				t.Errorf("%d signed registrations in %s, want the first and a retry about once a second until 6 seconds after it, %d to %d in all: %s", got, took.Round(time.Millisecond), fewestRegistrationsOverTheRetries, mostRegistrationsOverTheRetries, each.body)
			case !each.retried && (got != 1 || took > time.Second):
				t.Errorf("%d signed registrations in %s, want 1 and the clock advice at once: %s", got, took.Round(time.Millisecond), each.body)
			}
			checkAdvice(t, each, err)
		})
	}
}

func TestARefusedSignedRegistrationIsSignedAnewAboutOnceASecondUntilSixSecondsAfterTheFirstRefusal(t *testing.T) {
	t.Parallel()
	server, registrations := reasonRefusingHub(t, `{"error":"the switchboard turned this request away","reason":"refused"}`)
	_, err := NewWith(server.URL, Credential{Machine: "linux-box", Key: reasonKey()}).Agents()
	checkAdvice(t, reasonCases[0], err)
	seen := registrations.all()
	if len(seen) < fewestRegistrationsOverTheRetries {
		t.Fatalf("%d signed registrations, want the first and a retry about once a second until 6 seconds after it", len(seen))
	}
	nonces := map[string]bool{}
	for i, each := range seen {
		if each.nonce == "" || nonces[each.nonce] {
			t.Errorf("registration %d carries the nonce %q, which is empty or was sent before, want each retry signed anew", i+1, each.nonce)
		}
		nonces[each.nonce] = true
		if i == 0 {
			continue
		}
		if gap := each.at.Sub(seen[i-1].at); gap < 500*time.Millisecond || gap > 2*time.Second {
			t.Errorf("registration %d came %s after the one before, want about a second", i+1, gap.Round(time.Millisecond))
		}
		if each.unix < seen[i-1].unix {
			t.Errorf("registration %d is signed with time %d, earlier than %d before it", i+1, each.unix, seen[i-1].unix)
		}
	}
	if last := seen[len(seen)-1].at.Sub(seen[0].at); last < 5500*time.Millisecond || last > 8*time.Second {
		t.Errorf("the last retry came %s after the first refusal, want it near 6 seconds after", last.Round(time.Millisecond))
	}
}

func TestARegistrationRefusedInTheStartPeriodOfASwitchboardThatJustStartedEndsInTheAgentList(t *testing.T) {
	t.Parallel()
	sleeper.Until(time.Now().Truncate(time.Second).Add(time.Second))
	startSecond := time.Now().Unix()
	var refusals atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/observers":
			if unix, _ := signedTimeOf(r); unix <= startSecond+5 {
				refusals.Add(1)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"missing or wrong switchboard secret or machine signature","reason":"refused"}`))
				return
			}
			w.Write([]byte(`{"token":"observer-token"}`))
		case r.URL.Path == "/v1/agents" && r.Header.Get("Authorization") == "Bearer observer-token":
			w.Write([]byte(`[{"id":"alpha","harness":"claude-code","host":"mac","connected":true}]`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unknown token"}`))
		}
	}))
	t.Cleanup(server.Close)
	agents, err := NewWith(server.URL, Credential{Machine: "linux-box", Key: reasonKey()}).Agents()
	if err != nil || len(agents) != 1 {
		t.Fatalf("a registration refused in the 5 second start period of a switchboard that started at a whole second: %+v %v after %d refusals, want the agent list after a retry", agents, err, refusals.Load())
	}
	if refusals.Load() == 0 {
		t.Errorf("the fake switchboard refused nothing, so the test did not reach the start period")
	}
}
