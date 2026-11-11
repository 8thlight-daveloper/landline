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
