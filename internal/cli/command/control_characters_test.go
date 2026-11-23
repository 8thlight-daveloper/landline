package command

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

const hostileText = "bad\x1b]0;PWNED\x07\x1b[2J code"

func checkCleaned(t *testing.T, where, text string) {
	t.Helper()
	if strings.ContainsAny(text, "\x1b\x07") {
		t.Errorf("%s holds an escape or bell character from the switchboard: %q", where, text)
	}
	if !strings.Contains(text, "bad") || !strings.Contains(text, "code") {
		t.Errorf("%s lost the readable words of the switchboard's text: %q", where, text)
	}
}

func answering(t *testing.T, status int, body any) *httptest.Server {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(raw)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestAJoinRefusalIsPrintedWithoutTheControlCharactersOfTheSwitchboardsMessage(t *testing.T) {
	t.Parallel()
	server := answering(t, http.StatusUnauthorized, map[string]string{"error": hostileText})
	var out, errOut strings.Builder
	env := joinEnv(t.TempDir(), &out, &errOut)

	if code := Run([]string{"join", strings.TrimPrefix(server.URL, "http://"), "--code", "ABCD-EFGH"}, env); code == 0 {
		t.Fatalf("a refused join exited 0: %s", out.String())
	}

	checkCleaned(t, "the join refusal", errOut.String()+out.String())
}

func TestASignedRefusalIsPrintedWithoutTheControlCharactersOfTheSwitchboardsMessage(t *testing.T) {
	t.Parallel()
	server := answering(t, http.StatusUnauthorized, map[string]string{"error": hostileText, "reason": "refused"})
	dir := t.TempDir()
	savedSwitchboard(t, dir, server, "machine = \"linux-box\"\n")
	var out, errOut strings.Builder
	env := joinEnv(dir, &out, &errOut)
	if err := writeMachineKey(env, mustKey(t)); err != nil {
		t.Fatal(err)
	}

	if code := Run([]string{"log"}, env); code == 0 {
		t.Fatalf("a refused log read exited 0: %s", out.String())
	}

	checkCleaned(t, "the signed refusal", errOut.String()+out.String())
}

func TestLogPrintsAMessageBodyWithoutItsControlCharacters(t *testing.T) {
	t.Parallel()
	server := answering(t, http.StatusOK, []model.Envelope{{
		Seq: 1, ID: "m-01", Time: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
		Thread: "main", From: "alpha", To: "bravo", Kind: model.Kind("chat"), Body: hostileText,
	}})
	dir := t.TempDir()
	savedSwitchboard(t, dir, server, "secret = \""+longSecret+"\"\n")
	var out, errOut strings.Builder
	env := joinEnv(dir, &out, &errOut)

	if code := Run([]string{"log"}, env); code != 0 {
		t.Fatalf("log exited %d: %s", code, errOut.String())
	}

	checkCleaned(t, "the logged body", out.String())
}
