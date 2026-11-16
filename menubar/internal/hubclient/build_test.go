package hubclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

func buildServer(t *testing.T, status int, body string, stale *bool) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	issued := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v1/observers":
			issued++
			json.NewEncoder(w).Encode(map[string]string{"token": "tok-" + string(rune('0'+issued))})
		case "/v1/build":
			if stale != nil && *stale && r.Header.Get("Authorization") == "Bearer tok-1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(status)
			w.Write([]byte(body))
		}
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

func TestBuildReadsTheSwitchboardBuildWithTheObserverToken(t *testing.T) {
	server, seen := buildServer(t, 200, `{"version":"0.2.0","commit":"abc","date":"2026-11-16T10:00:00Z"}`, nil)
	build, known, err := New(server.URL, "s3cret-s3cret-s3cret").Build()
	if err != nil || !known || build != (Build{Version: "0.2.0", Commit: "abc", Date: "2026-11-16T10:00:00Z"}) {
		t.Fatalf("%+v %v %v", build, known, err)
	}
	for _, line := range *seen {
		if strings.Contains(line, "s3cret") && !strings.HasPrefix(line, "POST /v1/observers") {
			t.Errorf("the secret travelled on %q", line)
		}
	}
}

func TestBuildTreatsA404AndAnUnreadableAnswerAsUnknown(t *testing.T) {
	for name, c := range map[string]struct {
		status int
		body   string
	}{"404": {404, `{"error":"not found"}`}, "not json": {200, `<html>`}, "403": {403, `{}`}} {
		server, _ := buildServer(t, c.status, c.body, nil)
		_, known, err := New(server.URL, "s3cret-s3cret-s3cret").Build()
		if known || err != nil {
			t.Errorf("%s: known %v, err %v", name, known, err)
		}
	}
}

func TestBuildRegistersAgainOnA401(t *testing.T) {
	stale := true
	server, seen := buildServer(t, 200, `{"version":"0.2.0"}`, &stale)
	build, known, err := New(server.URL, "s3cret-s3cret-s3cret").Build()
	if err != nil || !known || build.Version != "0.2.0" {
		t.Fatalf("%+v %v %v", build, known, err)
	}
	registrations := 0
	for _, line := range *seen {
		if strings.HasPrefix(line, "POST /v1/observers") {
			registrations++
		}
	}
	if registrations != 2 {
		t.Errorf("registrations %d: %v", registrations, *seen)
	}
}

func TestBuildNeedsASecretAndReportsAnUnreachableSwitchboard(t *testing.T) {
	if _, _, err := New("http://127.0.0.1:1", "").Build(); err != ErrNoSecret {
		t.Errorf("no secret: %v", err)
	}
	if _, _, err := New("http://127.0.0.1:1", "s3cret-s3cret-s3cret").Build(); err == nil {
		t.Error("an unreachable switchboard is an error")
	}
}

func jsonNames(t reflect.Type) []string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		names = append(names, strings.Split(t.Field(i).Tag.Get("json"), ",")[0])
	}
	sort.Strings(names)
	return names
}

func TestAgentHasTheFieldsOfTheRootListEntry(t *testing.T) {
	if got, want := jsonNames(reflect.TypeOf(Agent{})), jsonNames(reflect.TypeOf(model.AgentInfo{})); !reflect.DeepEqual(got, want) {
		t.Fatalf("the client has %v, the switchboard sends %v", got, want)
	}
}

func TestTheAgentFieldsTheDeliveryProtocolNamesAreInTheClient(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/reference/delivery-protocol.md")
	if err != nil {
		t.Fatal(err)
	}
	have := jsonNames(reflect.TypeOf(Agent{}))
	for _, name := range []string{"connected", "landline_version", "landline_commit", "landline_date"} {
		if !strings.Contains(string(raw), "`"+name+"`") {
			t.Errorf("the protocol document no longer names %q", name)
		}
		if !slices.Contains(have, name) {
			t.Errorf("the client lacks %q", name)
		}
	}
}
