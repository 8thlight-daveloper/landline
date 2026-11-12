package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const secret = "s3cret-s3cret-s3cret"

func hub(t *testing.T) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/observers" {
			w.Write([]byte(`{"token":"observer-token-xyz"}`))
			return
		}
		w.Write([]byte(`[{"name":"alpha","harness":"claude","host":"mac","connected":true},{"name":"bravo","harness":"codex","host":"mac"}]`))
	}))
	t.Cleanup(server.Close)
	return server
}

func invoke(t *testing.T, address string, args ...string) (int, string, string) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LANDLINE_URL", address)
	t.Setenv("LANDLINE_SECRET", secret)
	var out, errs bytes.Buffer
	code := run(args, &out, &errs)
	return code, out.String(), errs.String()
}

func TestAgentsJSONPrintsOneSnapshot(t *testing.T) {
	server := hub(t)
	code, out, errs := invoke(t, server.URL, "agents", "--json")
	if code != 0 || errs != "" {
		t.Fatalf("code %d stderr %q", code, errs)
	}
	var view struct {
		Screen string
		Hub    string
		Agents []struct{ Handle, State string }
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	if view.Screen != "agents" || view.Hub != server.URL || len(view.Agents) != 2 || view.Agents[0].Handle != "alpha" || view.Agents[1].State != "disconnected" {
		t.Fatalf("got %s", out)
	}
	for _, forbidden := range []string{secret, "observer-token"} {
		if strings.Contains(out+errs, forbidden) {
			t.Fatalf("%q leaked", forbidden)
		}
	}
}

func TestAgentsJSONFailsWithAMessageWhenTheHubIsDown(t *testing.T) {
	server := hub(t)
	address := server.URL
	server.Close()
	code, out, errs := invoke(t, address, "agents", "--json")
	if code == 0 || out != "" || !strings.Contains(errs, "cannot reach the hub") || strings.Contains(errs, secret) {
		t.Fatalf("code %d out %q err %q", code, out, errs)
	}
}

func TestUsageErrors(t *testing.T) {
	server := hub(t)
	for _, args := range [][]string{{"agents"}, {"agents", "--json", "extra"}, {"agents", "--bogus"}, {"--bogus"}, {"extra"}} {
		if code, _, errs := invoke(t, server.URL, args...); code != 2 || !strings.Contains(errs, "usage:") {
			t.Fatalf("%v: code %d err %q", args, code, errs)
		}
	}
}

func TestIntervalBelowTheFloorIsAnError(t *testing.T) {
	server := hub(t)
	code, _, errs := invoke(t, server.URL, "--interval", "100ms")
	if code != 2 || !strings.Contains(errs, "at least 500ms") {
		t.Fatalf("code %d err %q", code, errs)
	}
}

func TestNoIntervalFlagMeansTwoSeconds(t *testing.T) {
	opts, problem := parseArgs(nil)
	if problem != "" || opts.interval != 2*time.Second {
		t.Fatalf("got %s %q", opts.interval, problem)
	}
}

func TestIntervalFlagOverridesTheDefault(t *testing.T) {
	opts, problem := parseArgs([]string{"--interval", "750ms"})
	if problem != "" || opts.interval != 750*time.Millisecond {
		t.Fatalf("got %s %q", opts.interval, problem)
	}
}

func TestAHostileDialErrorReachesStandardErrorOnlyCleaned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://evil.example/pwned\u202e\u0085")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	code, out, errs := invoke(t, server.URL, "agents", "--json")
	if code != 1 || out != "" || !strings.Contains(errs, "the hub answered with a redirect") {
		t.Fatalf("code %d out %q err %q", code, out, errs)
	}
	if strings.Contains(errs, "evil") || strings.Contains(errs, "pwned") {
		t.Fatalf("Location echoed: %q", errs)
	}
}

func TestBadHubAddressesFailOnStandardErrorWithoutEchoingTheUserinfo(t *testing.T) {
	for _, bad := range []string{
		"http://user:hunter2@127.0.0.1:1", "http://127.0.0.1:1?token=hunter2", "http://127.0.0.1:1#hunter2",
		"ftp://127.0.0.1:1", "http://127.0.0.1:1\x1b]0;pwned\x07",
	} {
		for _, args := range [][]string{{"agents", "--json"}, nil} {
			code, out, errs := invoke(t, bad, args...)
			if code != 1 || out != "" || !strings.Contains(errs, "the hub address must look like") {
				t.Fatalf("%q: code %d out %q err %q", bad, code, out, errs)
			}
			for _, forbidden := range []string{"hunter2", "pwned", "\x1b", "user"} {
				if strings.Contains(errs, forbidden) {
					t.Fatalf("%q: %q echoed in %q", bad, forbidden, errs)
				}
			}
		}
	}
}

func TestTheJSONHubFieldNeverHasUserinfo(t *testing.T) {
	server := hub(t)
	_, out, _ := invoke(t, server.URL, "agents", "--json")
	var view struct{ Hub string }
	if err := json.Unmarshal([]byte(out), &view); err != nil || strings.Contains(view.Hub, "@") || strings.Contains(view.Hub, "?") {
		t.Fatalf("%q %v", out, err)
	}
}

func TestFailWritesNoControlRuneButKeepsLineBreaks(t *testing.T) {
	var errs bytes.Buffer
	fail(&errs, "a\x1b[2J\u0085b\nc‮", 3)
	if errs.String() != "a [2J b\nc \n" {
		t.Fatalf("%q", errs.String())
	}
}

type countingWriter struct {
	writes int
	buf    bytes.Buffer
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.writes++
	return c.buf.Write(p)
}

func TestTheJSONSnapshotIsOneWrite(t *testing.T) {
	server := hub(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LANDLINE_URL", server.URL)
	t.Setenv("LANDLINE_SECRET", secret)
	out := &countingWriter{}
	var errs bytes.Buffer
	if code := run([]string{"agents", "--json"}, out, &errs); code != 0 {
		t.Fatalf("code %d err %q", code, errs.String())
	}
	if out.writes != 1 || !json.Valid(out.buf.Bytes()) {
		t.Fatalf("%d writes, valid %v", out.writes, json.Valid(out.buf.Bytes()))
	}
}

func TestBadArgumentsExitTwoWithUsageEvenWhenTheAddressIsBad(t *testing.T) {
	for _, args := range [][]string{{"--bogus"}, {"extra"}, {"agents"}, {"agents", "--bogus"}, {"agents", "--json", "extra"}, {"--interval", "100ms"}} {
		code, out, errs := invoke(t, "ftp://not-a-hub", args...)
		if code != 2 || out != "" || strings.Contains(errs, "the hub address must look like") {
			t.Fatalf("%v: code %d out %q err %q", args, code, out, errs)
		}
	}
	code, _, errs := invoke(t, "ftp://not-a-hub", "--bogus")
	if code != 2 || !strings.Contains(errs, "usage:") {
		t.Fatalf("code %d err %q", code, errs)
	}
}

func TestHelpPrintsUsageToStandardOutAndExitsZero(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"agents", "--help"}} {
		code, out, errs := invoke(t, "ftp://not-a-hub", args...)
		if code != 0 || !strings.Contains(out, "usage: landline-tui") || errs != "" {
			t.Fatalf("%v: code %d out %q err %q", args, code, out, errs)
		}
	}
}

func TestABadIntervalNamesTheReasonAndExitsTwo(t *testing.T) {
	for _, args := range [][]string{{"--interval", "abc"}, {"--interval"}, {"--interval=abc"}, {"agents", "--json", "--interval"}} {
		code, out, errs := invoke(t, "ftp://not-a-hub", args...)
		if code != 2 || out != "" || !strings.Contains(errs, "the interval must be a duration such as 2s") || !strings.Contains(errs, "usage:") {
			t.Fatalf("%v: code %d out %q err %q", args, code, out, errs)
		}
	}
}

func TestFlagsMayComeBeforeTheScreenName(t *testing.T) {
	server := hub(t)
	for _, args := range [][]string{{"--interval", "1s", "agents", "--json"}, {"--json", "agents"}, {"agents", "--interval=1s", "--json"}} {
		code, out, errs := invoke(t, server.URL, args...)
		if code != 0 || errs != "" || !json.Valid([]byte(out)) {
			t.Fatalf("%v: code %d out %q err %q", args, code, out, errs)
		}
	}
}
