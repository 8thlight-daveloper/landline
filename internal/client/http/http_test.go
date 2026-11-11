package http

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

func serve(t *testing.T, handler nethttp.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return New(s.URL, "tok")
}

func TestSendPostsJSONWithTheBearerToken(t *testing.T) {
	t.Parallel()
	var got model.SendBody
	var auth, path string
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		json.NewDecoder(r.Body).Decode(&got)
		json.NewEncoder(w).Encode(model.SendResponse{ID: "m-1", Seq: 4, Recipients: []string{"bravo"}})
	})
	out, err := c.Send(model.SendBody{To: "bravo", Body: "hi"})
	if err != nil || out.ID != "m-1" || out.Seq != 4 {
		t.Fatalf("%+v %v", out, err)
	}
	if auth != "Bearer tok" || path != "/v1/send" || got.To != "bravo" || got.Body != "hi" {
		t.Fatalf("auth %q path %q body %+v", auth, path, got)
	}
}

func TestAHubErrorSurfacesItsMessageAndStatus(t *testing.T) {
	t.Parallel()
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(model.ErrorResponse{Error: "the recipient is not a registered agent"})
	})
	_, err := c.Send(model.SendBody{To: "ghost", Body: "x"})
	if err == nil || !strings.Contains(err.Error(), "not a registered agent") || !strings.Contains(err.Error(), "404") {
		t.Fatalf("got %v", err)
	}
}

func TestInboxSendsWaitAndThreadParameters(t *testing.T) {
	t.Parallel()
	var query string
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		query = r.URL.RawQuery
		json.NewEncoder(w).Encode(model.InboxResponse{Messages: []model.WireEnvelope{{ID: "m-1"}}})
	})
	out, err := c.Inbox(25, "deploy,main", 3)
	if err != nil || len(out.Messages) != 1 {
		t.Fatalf("%+v %v", out, err)
	}
	for _, want := range []string{"wait=25", "thread=deploy%2Cmain", "after=3"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q lacks %q", query, want)
		}
	}
}

func TestAnUnreachableHubIsAnError(t *testing.T) {
	t.Parallel()
	c := New("http://127.0.0.1:1", "tok")
	if _, err := c.Agents(); err == nil {
		t.Fatal("no error")
	}
}

func TestWakesAsksForTheHostAndWait(t *testing.T) {
	t.Parallel()
	var query, path string
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		query, path = r.URL.RawQuery, r.URL.Path
		json.NewEncoder(w).Encode([]model.WakeJob{{ID: "j1", Agent: "bravo", Kind: "codex", Target: "t", Text: "[landline] x"}})
	})
	out, err := c.Wakes("linux", 30)
	if err != nil || len(out) != 1 || out[0].ID != "j1" || out[0].Target != "t" {
		t.Fatalf("%+v %v", out, err)
	}
	if path != "/v1/wakes" || !strings.Contains(query, "host=linux") || !strings.Contains(query, "wait=30") {
		t.Fatalf("path %q query %q", path, query)
	}
}

func TestAckWakePostsTheAckToTheJobPath(t *testing.T) {
	t.Parallel()
	var got model.WakeAck
	var method, path, auth string
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte("{}"))
	})
	if err := c.AckWake("m-a-bravo", model.WakeAck{OK: false, Error: "no thread"}); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/v1/wakes/m-a-bravo/ack" || auth != "Bearer tok" || got.OK || got.Error != "no thread" {
		t.Fatalf("%s %s %q %+v", method, path, auth, got)
	}
}

func TestAckWakeSurfacesAnUnknownJob(t *testing.T) {
	t.Parallel()
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(model.ErrorResponse{Error: "no such wake"})
	})
	if err := c.AckWake("ghost", model.WakeAck{OK: true}); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("got %v", err)
	}
}

func TestAnInboxPageOfSeveralMegabytesIsReadInFull(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("a", 60000)
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		page := model.InboxResponse{}
		for i := 0; i < 40; i++ {
			page.Messages = append(page.Messages, model.WireEnvelope{Body: body, Text: body})
		}
		json.NewEncoder(w).Encode(page)
	})
	out, err := c.Inbox(0, "", 0)
	if err != nil || len(out.Messages) != 40 {
		t.Fatalf("%d messages, %v", len(out.Messages), err)
	}
}
