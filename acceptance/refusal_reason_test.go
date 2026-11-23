package acceptance

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

const (
	formerMachineRefusal = "missing or wrong switchboard secret or machine signature"
	formerClockRefusal   = "the clock of this machine differs from the switchboard's by 75 seconds. Set the clock"
)

type refusalAnswer struct {
	name         string
	body         string
	signed       int
	clock        bool
	shownAsGiven string
}

var refusalAnswers = []refusalAnswer{
	{"refused with a reworded message", `{"error":"the switchboard turned this request away","reason":"refused"}`, 2, false, ""},
	{"clock with the former generic text", `{"error":"` + formerMachineRefusal + `","reason":"clock","clock_difference":437}`, 1, true, ""},
	{"no reason with the former clock text", `{"error":"` + formerClockRefusal + `"}`, 2, false, formerClockRefusal},
	{"an unknown reason", `{"error":"` + formerClockRefusal + `","reason":"weather","clock_difference":75}`, 2, false, formerClockRefusal},
	{"clock with a difference in quotes", `{"error":"` + formerClockRefusal + `","reason":"clock","clock_difference":"75"}`, 2, false, formerClockRefusal},
}

type refusingSwitchboard struct {
	mu     sync.Mutex
	signed map[string]int
	host   string
	port   string
}

func startRefusingSwitchboard(t *testing.T, body string) *refusingSwitchboard {
	t.Helper()
	fake := &refusingSwitchboard{signed: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Landline-Machine ") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fake.mu.Lock()
		fake.signed[r.Method+" "+r.URL.Path]++
		fake.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	fake.host, fake.port, _ = net.SplitHostPort(server.Listener.Addr().String())
	return fake
}

func (f *refusingSwitchboard) signedRequests(route string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.signed[route]
}

func machineHomeFor(t *testing.T, fake *refusingSwitchboard) (string, []string) {
	t.Helper()
	home := t.TempDir()
	seed := make([]byte, ed25519.SeedSize)
	rand.Read(seed)
	writeConfigFile(t, home, "machine.key", base64.StdEncoding.EncodeToString(seed)+"\n")
	writeConfigFile(t, home, "config.toml", "host = \""+fake.host+"\"\nport = "+fake.port+"\nmachine = \"linux-box\"\n")
	return home, isolatedEnv(home)
}

func checkRefusalAdvice(t *testing.T, answer refusalAnswer, shown string) {
	t.Helper()
	text := strings.ToLower(shown)
	if answer.clock {
		if !strings.Contains(text, "clock") || !strings.Contains(text, "437") {
			t.Errorf("the clock advice does not name the clock and the difference of 437 seconds from clock_difference: %q", shown)
		}
		for _, wrong := range []string{"join", "--code", "revoke", "rotate"} {
			if strings.Contains(text, wrong) {
				t.Errorf("the clock advice says %q, which belongs to the generic advice: %q", wrong, shown)
			}
		}
	} else {
		for _, cause := range []string{"rotate", "revoke", "clock", "join"} {
			if !strings.Contains(text, cause) {
				t.Errorf("the generic advice leaves out %q: %q", cause, shown)
			}
		}
	}
	if answer.shownAsGiven != "" && !strings.Contains(shown, answer.shownAsGiven) {
		t.Errorf("the switchboard's message is not shown as given, %q: %q", answer.shownAsGiven, shown)
	}
}

func TestLandlineRegisterOnAMachineWithAKeyRetriesAndAdvisesByTheReasonOfA401AndNeverByItsText(t *testing.T) {
	t.Parallel()
	bin := build(t)
	for _, answer := range refusalAnswers {
		t.Run(answer.name, func(t *testing.T) {
			t.Parallel()
			fake := startRefusingSwitchboard(t, answer.body)
			home, env := machineHomeFor(t, fake)
			out, err := runIn(t, bin, env, home, "register", "--id", "alpha", "--harness", "claude-code", "--host", "mac")
			if err == nil {
				t.Fatalf("register succeeded against a switchboard that refuses everything: %s", out)
			}
			if got := fake.signedRequests("POST /v1/register"); got != answer.signed {
				t.Errorf("%d signed registrations, want %d: %s", got, answer.signed, answer.body)
			}
			checkRefusalAdvice(t, answer, out)
		})
	}
}

func TestLandlineMCPOnAMachineWithAKeyRetriesAndAdvisesByTheReasonOfA401AndNeverByItsText(t *testing.T) {
	t.Parallel()
	bin := build(t)
	for _, answer := range refusalAnswers {
		t.Run(answer.name, func(t *testing.T) {
			t.Parallel()
			fake := startRefusingSwitchboard(t, answer.body)
			home, env := machineHomeFor(t, fake)
			cmd := exec.Command(mcpOf(bin))
			cmd.Dir, cmd.Env = home, env
			p := newMCP(t, cmd)
			p.handshake("claude-code", "2.1.296")
			reply, _ := json.Marshal(p.request("tools/call", `{"name":"landline_register","arguments":{"project":"remote-proj"}}`))
			var decoded struct {
				Result struct {
					Content []struct{ Text string } `json:"content"`
				} `json:"result"`
				Error struct{ Message string } `json:"error"`
			}
			json.Unmarshal(reply, &decoded)
			shown := decoded.Error.Message
			for _, part := range decoded.Result.Content {
				shown += part.Text
			}
			if got := fake.signedRequests("POST /v1/register"); got != answer.signed {
				t.Errorf("%d signed registrations, want %d: %s", got, answer.signed, answer.body)
			}
			checkRefusalAdvice(t, answer, shown)
		})
	}
}
