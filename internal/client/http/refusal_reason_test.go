package http

import (
	"crypto/ed25519"
	"crypto/rand"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

const (
	oldGenericRefusal = "missing or wrong switchboard secret or machine signature"
	oldClockMessage   = "the clock of this machine differs from the switchboard's by 75 seconds. Set the clock"
)

type refusalCase struct {
	name     string
	body     string
	requests int
	message  string
}

var refusalCases = []refusalCase{
	{"refused with a reworded message", `{"error":"the switchboard turned this request away","reason":"refused"}`, 2, ""},
	{"refused with the old clock text", `{"error":"` + oldClockMessage + `","reason":"refused"}`, 2, ""},
	{"clock with the old generic text", `{"error":"` + oldGenericRefusal + `","reason":"clock","clock_difference":437}`, 1, ""},
	{"clock behind with the old generic text", `{"error":"` + oldGenericRefusal + `","reason":"clock","clock_difference":-437}`, 1, ""},
	{"an unknown reason", `{"error":"` + oldClockMessage + `","reason":"weather","clock_difference":75}`, 2, oldClockMessage},
	{"no reason with the clock text", `{"error":"` + oldClockMessage + `"}`, 2, oldClockMessage},
	{"no reason with the generic text", `{"error":"` + oldGenericRefusal + `"}`, 2, oldGenericRefusal},
	{"clock with no difference", `{"error":"` + oldClockMessage + `","reason":"clock"}`, 2, oldClockMessage},
	{"clock with a difference in quotes", `{"error":"` + oldClockMessage + `","reason":"clock","clock_difference":"75"}`, 2, oldClockMessage},
	{"clock with a fractional difference", `{"error":"` + oldClockMessage + `","reason":"clock","clock_difference":75.5}`, 2, oldClockMessage},
	{"clock with a null difference", `{"error":"` + oldClockMessage + `","reason":"clock","clock_difference":null}`, 2, oldClockMessage},
	{"a reason that is not text", `{"error":"` + oldClockMessage + `","reason":7,"clock_difference":75}`, 2, oldClockMessage},
}

type countingRefuser struct {
	mu       sync.Mutex
	requests map[string]int
	signed   int
}

func refusingSwitchboard(t *testing.T, body string) (*httptest.Server, *countingRefuser) {
	t.Helper()
	counter := &countingRefuser{requests: map[string]int{}}
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		counter.mu.Lock()
		counter.requests[r.Method+" "+r.URL.Path]++
		if strings.HasPrefix(r.Header.Get("Authorization"), "Landline-Machine ") {
			counter.signed++
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

func testSigner(t *testing.T) Signer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return Signer{Name: "linux-box", Key: key}
}

func TestASignedRequestRetriesOnceOrNotAtAllByTheReasonAndNeverByTheMessageText(t *testing.T) {
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
	for route, call := range calls {
		for _, each := range refusalCases {
			t.Run(route+" "+each.name, func(t *testing.T) {
				t.Parallel()
				server, counter := refusingSwitchboard(t, each.body)
				err := call(NewSigned(server.URL, testSigner(t)))
				if err == nil {
					t.Fatal("a 401 was not an error")
				}
				requests, signed := counter.count(route)
				if requests != each.requests || signed != each.requests {
					t.Errorf("%d requests, %d of them signed, to %s, want %d signed: %s", requests, signed, route, each.requests, each.body)
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
