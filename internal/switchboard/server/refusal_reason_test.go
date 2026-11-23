package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

const insideWindowRefusal = "missing or wrong switchboard secret or machine signature"

type signedRoute struct {
	method, path string
	body         []byte
}

var routesAMachineKeyOpens = map[string]signedRoute{
	"register":  {"POST", "/v1/register", []byte(`{"id":"alpha","harness":"claude-code","host":"mac"}`)},
	"observers": {"POST", "/v1/observers", []byte(`{"kind":"tui"}`)},
	"log":       {"GET", "/v1/log?after=0", nil},
	"build":     {"GET", "/v1/build", nil},
}

func refusalFields(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("the 401 body is not a JSON object: %q", raw)
	}
	return fields
}

func textField(fields map[string]json.RawMessage, name string) string {
	var text string
	json.Unmarshal(fields[name], &text)
	return text
}

func fieldNames(fields map[string]json.RawMessage) []string {
	var names []string
	for name := range fields {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func startedPastTheStartSecond(t *testing.T) *harness {
	t.Helper()
	h := start(t)
	h.advance(5 * time.Second)
	return h
}

func TestASignedRequestOutsideTheWindowGetsReasonClockAndTheSignedDifferenceOnEveryRouteAMachineKeyOpens(t *testing.T) {
	for routeName, route := range routesAMachineKeyOpens {
		for _, offset := range []int64{75, -75, 61, -61, 3600, -3600} {
			for _, machine := range []string{"linux-box", "ghost-box"} {
				t.Run(routeName+" "+strconv.FormatInt(offset, 10)+" "+machine, func(t *testing.T) {
					h := startedPastTheStartSecond(t)
					key := h.joinMachine("linux-box")
					authorization := machineAuthorization(key, machine, h.now.Unix()+offset, route.method, route.path, route.body)
					status, raw := h.sendWithAuthorization(route.method, route.path, authorization, route.body)
					if status != http.StatusUnauthorized {
						t.Fatalf("status %d %s, want 401", status, raw)
					}
					fields := refusalFields(t, raw)
					if got := fieldNames(fields); !slices.Equal(got, []string{"clock_difference", "error", "reason"}) {
						t.Errorf("fields %v, want exactly clock_difference, error, and reason: %s", got, raw)
					}
					if reason := textField(fields, "reason"); reason != "clock" {
						t.Errorf("reason %q, want clock: %s", reason, raw)
					}
					if got, want := string(fields["clock_difference"]), strconv.FormatInt(offset, 10); got != want {
						t.Errorf("clock_difference %s, want the integer %s, the request time minus the switchboard time: %s", got, want, raw)
					}
					message := textField(fields, "error")
					if !strings.Contains(strings.ToLower(message), "clock") || !strings.Contains(message, strconv.FormatInt(abs(offset), 10)) {
						t.Errorf("the message %q does not name the clock and the difference", message)
					}
				})
			}
		}
	}
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

type insideWindowCause struct {
	name          string
	authorization func(h *harness, keys map[string]ed25519.PrivateKey, route signedRoute) string
}

var insideWindowCauses = []insideWindowCause{
	{"an unknown name", func(h *harness, keys map[string]ed25519.PrivateKey, route signedRoute) string {
		return machineAuthorization(keys["stranger"], "ghost-box", h.now.Unix(), route.method, route.path, route.body)
	}},
	{"a pending name", func(h *harness, keys map[string]ed25519.PrivateKey, route signedRoute) string {
		return machineAuthorization(keys["stranger"], "new-box", h.now.Unix(), route.method, route.path, route.body)
	}},
	{"a revoked machine", func(h *harness, keys map[string]ed25519.PrivateKey, route signedRoute) string {
		return machineAuthorization(keys["old-box"], "old-box", h.now.Unix(), route.method, route.path, route.body)
	}},
	{"a time in the start second", func(h *harness, keys map[string]ed25519.PrivateKey, route signedRoute) string {
		return machineAuthorization(keys["linux-box"], "linux-box", h.now.Unix()-5, route.method, route.path, route.body)
	}},
	{"a wrong key", func(h *harness, keys map[string]ed25519.PrivateKey, route signedRoute) string {
		return machineAuthorization(keys["stranger"], "linux-box", h.now.Unix(), route.method, route.path, route.body)
	}},
	{"a signature over another path", func(h *harness, keys map[string]ed25519.PrivateKey, route signedRoute) string {
		return machineAuthorization(keys["linux-box"], "linux-box", h.now.Unix(), route.method, "/v1/other", route.body)
	}},
	{"a replay", func(h *harness, keys map[string]ed25519.PrivateKey, route signedRoute) string {
		authorization := machineAuthorization(keys["linux-box"], "linux-box", h.now.Unix(), route.method, route.path, route.body)
		if status, raw := h.sendWithAuthorization(route.method, route.path, authorization, route.body); status != http.StatusOK {
			h.t.Fatalf("the first use of the signature was refused: %d %s", status, raw)
		}
		return authorization
	}},
	{"a header with no fields", func(*harness, map[string]ed25519.PrivateKey, signedRoute) string {
		return "Landline-Machine "
	}},
	{"a header with words in place of fields", func(*harness, map[string]ed25519.PrivateKey, signedRoute) string {
		return "Landline-Machine let me in"
	}},
	{"a header with a time that is not a number", func(*harness, map[string]ed25519.PrivateKey, signedRoute) string {
		return "Landline-Machine name=linux-box, time=soon, nonce=AAAAAAAAAAAAAAAAAAAAAA==, signature=AAAA"
	}},
	{"a header with a short nonce", func(h *harness, keys map[string]ed25519.PrivateKey, route signedRoute) string {
		return strings.Replace(machineAuthorization(keys["linux-box"], "linux-box", h.now.Unix(), route.method, route.path, route.body), "nonce=", "nonce=AAAA", 1)
	}},
	{"a header with a signature that is not base64", func(h *harness, _ map[string]ed25519.PrivateKey, _ signedRoute) string {
		return "Landline-Machine name=linux-box, time=" + strconv.FormatInt(h.now.Unix(), 10) + ", nonce=" + randomNonce() + ", signature=%%%"
	}},
}

func machinesForRefusals(t *testing.T, h *harness) map[string]ed25519.PrivateKey {
	t.Helper()
	keys := map[string]ed25519.PrivateKey{"linux-box": h.joinMachine("linux-box"), "old-box": h.joinMachine("old-box")}
	if status, raw := h.call("DELETE", "/v1/machines/old-box", secret, nil); status != http.StatusOK {
		t.Fatalf("revoke old-box: %d %s", status, raw)
	}
	if status, raw := h.call("POST", "/v1/machines", secret, map[string]string{"name": "new-box"}); status != http.StatusOK {
		t.Fatalf("invite new-box: %d %s", status, raw)
	}
	_, stranger, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys["stranger"] = stranger
	return keys
}

func TestEveryRefusalInsideTheWindowGetsReasonRefusedAndOneBodyOnEveryRouteAMachineKeyOpens(t *testing.T) {
	for routeName, route := range routesAMachineKeyOpens {
		t.Run(routeName, func(t *testing.T) {
			h := startedPastTheStartSecond(t)
			keys := machinesForRefusals(t, h)
			bodies := map[string]string{}
			for _, cause := range insideWindowCauses {
				authorization := cause.authorization(h, keys, route)
				status, raw := h.sendWithAuthorization(route.method, route.path, authorization, route.body)
				if status != http.StatusUnauthorized {
					t.Errorf("%s: status %d %s, want 401", cause.name, status, raw)
					continue
				}
				fields := refusalFields(t, raw)
				if got := fieldNames(fields); !slices.Equal(got, []string{"error", "reason"}) {
					t.Errorf("%s: fields %v, want exactly error and reason: %s", cause.name, got, raw)
				}
				if reason := textField(fields, "reason"); reason != "refused" {
					t.Errorf("%s: reason %q, want refused: %s", cause.name, reason, raw)
				}
				if message := textField(fields, "error"); message != insideWindowRefusal {
					t.Errorf("%s: message %q, want %q", cause.name, message, insideWindowRefusal)
				}
				bodies[cause.name] = strings.TrimSpace(string(raw))
			}
			for name, body := range bodies {
				if body != bodies[insideWindowCauses[0].name] {
					t.Errorf("%s answers %s and %s answers %s, so the body tells the causes apart", name, body, insideWindowCauses[0].name, bodies[insideWindowCauses[0].name])
				}
				for _, machine := range []string{"linux-box", "old-box", "new-box", "ghost-box"} {
					if strings.Contains(body, machine) {
						t.Errorf("%s: the body names %s: %s", name, machine, body)
					}
				}
			}
		})
	}
}

func TestASignedRequestJustInsideTheWindowIsAcceptedAndCarriesNoRefusal(t *testing.T) {
	for routeName, route := range routesAMachineKeyOpens {
		for _, offset := range []int64{60, -4} {
			t.Run(routeName+" "+strconv.FormatInt(offset, 10), func(t *testing.T) {
				h := startedPastTheStartSecond(t)
				key := h.joinMachine("linux-box")
				authorization := machineAuthorization(key, "linux-box", h.now.Unix()+offset, route.method, route.path, route.body)
				if status, raw := h.sendWithAuthorization(route.method, route.path, authorization, route.body); status != http.StatusOK {
					t.Errorf("a time %d seconds from the switchboard's: %d %s", offset, status, raw)
				}
			})
		}
	}
}
