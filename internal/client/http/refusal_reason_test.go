package http

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

const (
	oldGenericRefusal = "missing or wrong switchboard secret or machine signature"
	oldClockMessage   = "the clock of this machine differs from the switchboard's by 75 seconds. Set the clock"
)

type refusalCase struct {
	name    string
	body    string
	retried bool
	message string
}

var refusalCases = []refusalCase{
	{"refused with a reworded message", `{"error":"the switchboard turned this request away","reason":"refused"}`, true, ""},
	{"refused with the old clock text", `{"error":"` + oldClockMessage + `","reason":"refused"}`, true, ""},
	{"clock with the old generic text", `{"error":"` + oldGenericRefusal + `","reason":"clock","clock_difference":437}`, false, ""},
	{"clock behind with the old generic text", `{"error":"` + oldGenericRefusal + `","reason":"clock","clock_difference":-437}`, false, ""},
	{"an unknown reason", `{"error":"` + oldClockMessage + `","reason":"weather","clock_difference":75}`, true, oldClockMessage},
	{"no reason with the clock text", `{"error":"` + oldClockMessage + `"}`, true, oldClockMessage},
	{"no reason with the generic text", `{"error":"` + oldGenericRefusal + `"}`, true, oldGenericRefusal},
	{"clock with no difference", `{"error":"` + oldClockMessage + `","reason":"clock"}`, true, oldClockMessage},
	{"clock with a difference in quotes", `{"error":"` + oldClockMessage + `","reason":"clock","clock_difference":"75"}`, true, oldClockMessage},
	{"clock with a fractional difference", `{"error":"` + oldClockMessage + `","reason":"clock","clock_difference":75.5}`, true, oldClockMessage},
	{"clock with a null difference", `{"error":"` + oldClockMessage + `","reason":"clock","clock_difference":null}`, true, oldClockMessage},
	{"a reason that is not text", `{"error":"` + oldClockMessage + `","reason":7,"clock_difference":75}`, true, oldClockMessage},
}

type signedArrival struct {
	at    time.Time
	nonce string
	unix  int64
}

type countingRefuser struct {
	mu       sync.Mutex
	requests map[string]int
	signed   int
	arrivals []signedArrival
}

var signedFields = regexp.MustCompile(`time=(-?[0-9]+), nonce=([^,]+),`)

func refusingSwitchboard(t *testing.T, body string) (*httptest.Server, *countingRefuser) {
	t.Helper()
	counter := &countingRefuser{requests: map[string]int{}}
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		counter.mu.Lock()
		counter.requests[r.Method+" "+r.URL.Path]++
		if authorization := r.Header.Get("Authorization"); strings.HasPrefix(authorization, "Landline-Machine ") {
			counter.signed++
			arrival := signedArrival{at: time.Now()}
			if found := signedFields.FindStringSubmatch(authorization); found != nil {
				arrival.unix, _ = strconv.ParseInt(found[1], 10, 64)
				arrival.nonce = found[2]
			}
			counter.arrivals = append(counter.arrivals, arrival)
		}
		counter.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(nethttp.StatusUnauthorized)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, counter
}

func (c *countingRefuser) count(route string) (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests[route], c.signed
}

func (c *countingRefuser) signedArrivals() []signedArrival {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.arrivals)
}

const (
	fewestRequestsOverTheRetries = 6
	mostRequestsOverTheRetries   = 8
)

func testSigner(t *testing.T) Signer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return Signer{Name: "linux-box", Key: key}
}

func TestASignedRequestRetriesForSixSecondsOrNotAtAllByTheReasonAndNeverByTheMessageText(t *testing.T) {
	t.Parallel()
	calls := map[string]func(*Client) error{
		"POST /v1/register": func(c *Client) error {
			_, err := c.Register(model.RegisterRequest{ID: "alpha", Harness: "claude-code", Host: "mac"})
			return err
		},
		"GET /v1/log": func(c *Client) error {
			_, err := c.Log(0)
			return err
		},
	}
	casesOf := map[string][]refusalCase{"POST /v1/register": refusalCases, "GET /v1/log": {refusalCases[0], refusalCases[2]}}
	for route, call := range calls {
		for _, each := range casesOf[route] {
			t.Run(route+" "+each.name, func(t *testing.T) {
				t.Parallel()
				server, counter := refusingSwitchboard(t, each.body)
				began := time.Now()
				err := call(NewSigned(server.URL, testSigner(t)))
				took := time.Since(began)
				if err == nil {
					t.Fatal("a 401 was not an error")
				}
				requests, signed := counter.count(route)
				switch {
				case signed != requests:
					t.Errorf("%d requests to %s, and only %d of them signed: %s", requests, route, signed, each.body)
				case each.retried && (requests < fewestRequestsOverTheRetries || requests > mostRequestsOverTheRetries):
					t.Errorf("%d signed requests to %s in %s, want the first and a retry about once a second until 6 seconds after it, %d to %d in all: %s", requests, route, took.Round(time.Millisecond), fewestRequestsOverTheRetries, mostRequestsOverTheRetries, each.body)
				case !each.retried && (requests != 1 || took > time.Second):
					t.Errorf("%d signed requests to %s in %s, want 1 and the clock advice at once: %s", requests, route, took.Round(time.Millisecond), each.body)
				}
				if each.message != "" && !strings.Contains(err.Error(), each.message) {
					t.Errorf("the error %q does not show the switchboard's message as given: %q", err, each.message)
				}
			})
		}
	}
}

func TestARequestWithTheSharedSecretNeverRetriesWhateverTheReason(t *testing.T) {
	for _, each := range refusalCases {
		t.Run(each.name, func(t *testing.T) {
			t.Parallel()
			server, counter := refusingSwitchboard(t, each.body)
			if _, err := New(server.URL, "0123456789abcdef-secret").Register(model.RegisterRequest{ID: "alpha", Harness: "claude-code", Host: "mac"}); err == nil {
				t.Fatal("a 401 was not an error")
			}
			if requests, _ := counter.count("POST /v1/register"); requests != 1 {
				t.Errorf("%d requests with the shared secret, want 1: %s", requests, each.body)
			}
		})
	}
}

func TestARefusedSignedRequestIsSignedAnewAboutOnceASecondUntilSixSecondsAfterTheFirstRefusalAndThenGivesTheGenericAdvice(t *testing.T) {
	t.Parallel()
	server, counter := refusingSwitchboard(t, `{"error":"the switchboard turned this request away","reason":"refused"}`)
	_, err := NewSigned(server.URL, testSigner(t)).Register(model.RegisterRequest{ID: "alpha", Harness: "claude-code", Host: "mac"})
	var refused *StatusError
	if !errors.As(err, &refused) || refused.Status != nethttp.StatusUnauthorized {
		t.Fatalf("the last refusal is not a 401 status error: %v", err)
	}
	for _, cause := range []string{"rotate", "revoke", "clock", "join"} {
		if !strings.Contains(strings.ToLower(err.Error()), cause) {
			t.Errorf("the advice after the last retry leaves out %q: %q", cause, err)
		}
	}
	arrivals := counter.signedArrivals()
	if len(arrivals) < fewestRequestsOverTheRetries {
		t.Fatalf("%d signed requests, want the first and a retry about once a second until 6 seconds after it", len(arrivals))
	}
	nonces := map[string]bool{}
	for i, arrival := range arrivals {
		if arrival.nonce == "" || nonces[arrival.nonce] {
			t.Errorf("request %d carries the nonce %q, which is empty or was sent before, want each retry signed anew", i+1, arrival.nonce)
		}
		nonces[arrival.nonce] = true
		if i == 0 {
			continue
		}
		gap := arrival.at.Sub(arrivals[i-1].at)
		if gap < 500*time.Millisecond || gap > 2*time.Second {
			t.Errorf("request %d came %s after the one before, want about a second", i+1, gap.Round(time.Millisecond))
		}
		if arrival.unix < arrivals[i-1].unix {
			t.Errorf("request %d is signed with time %d, earlier than %d before it, want each retry signed at the moment it is sent", i+1, arrival.unix, arrivals[i-1].unix)
		}
	}
	if last := arrivals[len(arrivals)-1].at.Sub(arrivals[0].at); last < 5500*time.Millisecond || last > 8*time.Second {
		t.Errorf("the last retry came %s after the first refusal, want it near 6 seconds after", last.Round(time.Millisecond))
	}
}

func TestASignedRequestRefusedInTheStartPeriodOfASwitchboardThatJustStartedSucceedsWithoutAnError(t *testing.T) {
	t.Parallel()
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
	startSecond := time.Now().Unix()
	var mu sync.Mutex
	var refusals, accepted int
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		found := signedFields.FindStringSubmatch(r.Header.Get("Authorization"))
		unix := int64(0)
		if found != nil {
			unix, _ = strconv.ParseInt(found[1], 10, 64)
		}
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if unix <= startSecond+5 {
			refusals++
			w.WriteHeader(nethttp.StatusUnauthorized)
			w.Write([]byte(`{"error":"missing or wrong switchboard secret or machine signature","reason":"refused"}`))
			return
		}
		accepted++
		w.Write([]byte(`{"id":"alpha","token":"agent-token"}`))
	}))
	t.Cleanup(server.Close)
	out, err := NewSigned(server.URL, testSigner(t)).Register(model.RegisterRequest{ID: "alpha", Harness: "claude-code", Host: "mac"})
	mu.Lock()
	defer mu.Unlock()
	if err != nil || out.Token != "agent-token" {
		t.Fatalf("a registration refused in the 5 second start period of a switchboard that started at a whole second: %+v %v after %d refusals, want it to succeed on a retry", out, err, refusals)
	}
	if refusals == 0 || accepted != 1 {
		t.Errorf("%d refusals and %d accepted requests, want the start period to refuse the first and one retry to pass", refusals, accepted)
	}
}
