package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestAnUnreachableHubIsAnError(t *testing.T) {
	t.Parallel()
	c := New("http://127.0.0.1:1", "tok")
	if _, err := c.Agents(); err == nil {
		t.Fatal("no error")
	}
}

func writeEvent(w nethttp.ResponseWriter, delivery model.Delivery) {
	raw, _ := json.Marshal(delivery)
	fmt.Fprintf(w, "event: delivery\ndata: %s\n\n", raw)
	w.(nethttp.Flusher).Flush()
}

func collect(t *testing.T, c *Client) ([]model.Delivery, error) {
	t.Helper()
	var got []model.Delivery
	err := c.Stream(context.Background(), func(d model.Delivery) { got = append(got, d) })
	return got, err
}

func TestAStreamDeliversEventsInOrderWithTheirExactFieldsAndEndsCleanly(t *testing.T) {
	t.Parallel()
	first := model.Delivery{ID: "d-1", Kind: model.DeliveryMessage, Text: "[landline] hi", Message: &model.WireEnvelope{ID: "m-1", Body: "hi", Text: "[landline] hi"}}
	second := model.Delivery{ID: "d-2", Kind: model.DeliveryChallenge, Text: "code 42", Target: "thread-alpha"}
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": keepalive\n\n")
		writeEvent(w, first)
		fmt.Fprint(w, "\n\n: keepalive\n\n")
		writeEvent(w, second)
		fmt.Fprint(w, ": keepalive\n\n")
	})
	got, err := collect(t, c)
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if got[0].ID != "d-1" || got[0].Kind != model.DeliveryMessage || got[0].Text != "[landline] hi" || got[0].Message == nil || got[0].Message.ID != "m-1" || got[0].Message.Body != "hi" {
		t.Fatalf("first %+v", got[0])
	}
	if got[1].ID != "d-2" || got[1].Kind != model.DeliveryChallenge || got[1].Target != "thread-alpha" || got[1].Message != nil {
		t.Fatalf("second %+v", got[1])
	}
}

func TestAStreamRequestIsAGetToTheStreamPathWithTheBearerToken(t *testing.T) {
	t.Parallel()
	var method, path, auth string
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
	})
	if _, err := collect(t, c); err != nil {
		t.Fatal(err)
	}
	if method != "GET" || path != "/v1/stream" || auth != "Bearer tok" {
		t.Fatalf("%s %s %q", method, path, auth)
	}
}

func TestAStreamRefusalBecomesAStatusErrorWithItsMessage(t *testing.T) {
	t.Parallel()
	for status, message := range map[int]string{401: "bad token", 409: "not connected"} {
		c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(model.ErrorResponse{Error: message})
		})
		_, err := collect(t, c)
		var statusErr *StatusError
		if !errors.As(err, &statusErr) || statusErr.Status != status || statusErr.Message != message {
			t.Fatalf("status %d got %v", status, err)
		}
	}
}

func TestAStreamRefusalWithoutAJSONBodyStillCarriesTheStatus(t *testing.T) {
	t.Parallel()
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) { w.WriteHeader(500) })
	_, err := collect(t, c)
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.Status != 500 {
		t.Fatalf("got %v", err)
	}
}

func TestCancellingTheContextReturnsPromptlyWithCanceledAndClosesTheConnection(t *testing.T) {
	t.Parallel()
	opened := make(chan struct{})
	closed := make(chan struct{})
	s := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": keepalive\n\n")
		w.(nethttp.Flusher).Flush()
		close(opened)
		<-r.Context().Done()
		close(closed)
	}))
	t.Cleanup(s.Close)
	c := New(s.URL, "tok")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- c.Stream(ctx, func(model.Delivery) {}) }()
	<-opened
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream did not return after cancel")
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the server never saw the connection close")
	}
}

func TestACancelledContextBeforeConnectingReturnsCanceled(t *testing.T) {
	t.Parallel()
	c := New("http://127.0.0.1:1", "tok")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Stream(ctx, func(model.Delivery) {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestBadDeliveryJSONIsAnErrorAndStopsTheStream(t *testing.T) {
	t.Parallel()
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		fmt.Fprint(w, "event: delivery\ndata: {not json\n\n")
	})
	got, err := collect(t, c)
	if err == nil || len(got) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestSeveralDataLinesOfOneEventAreJoinedWithNewlinesBeforeDecoding(t *testing.T) {
	t.Parallel()
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		fmt.Fprint(w, "event: delivery\ndata: {\"id\":\"d-1\",\ndata: \"kind\":\"message\",\"text\":\"hi\"}\n\n")
	})
	got, err := collect(t, c)
	if err != nil || len(got) != 1 || got[0].ID != "d-1" || got[0].Text != "hi" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestAnEventThatIsNotADeliveryIsIgnored(t *testing.T) {
	t.Parallel()
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		fmt.Fprint(w, "event: other\ndata: {\"id\":\"x\"}\n\n")
	})
	got, err := collect(t, c)
	if err != nil || len(got) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestAStreamLineOfOneMebibyteIsRead(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 1<<20)
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		writeEvent(w, model.Delivery{ID: "d-1", Kind: model.DeliveryMessage, Text: text})
	})
	got, err := collect(t, c)
	if err != nil || len(got) != 1 || got[0].Text != text {
		t.Fatalf("%d deliveries, %v", len(got), err)
	}
}

func TestTheStreamClientHasNoOverallTimeoutWhileOtherCallsKeepNinetySeconds(t *testing.T) {
	t.Parallel()
	c := New("http://x", "t")
	if c.streaming.Timeout != 0 {
		t.Fatalf("stream timeout %v", c.streaming.Timeout)
	}
	if c.http.Timeout != 90*time.Second {
		t.Fatalf("call timeout %v", c.http.Timeout)
	}
}

func TestReportPostsTheReportToTheDeliveryPath(t *testing.T) {
	t.Parallel()
	var got model.DeliveryReport
	var method, path, auth string
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		method, path, auth = r.Method, r.URL.EscapedPath(), r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte("{}"))
	})
	if err := c.Report("d/1 x", model.DeliveryReport{OK: false, Error: "no thread"}); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/v1/deliveries/d%2F1%20x/report" || auth != "Bearer tok" || got.OK || got.Error != "no thread" {
		t.Fatalf("%s %s %q %+v", method, path, auth, got)
	}
}

func TestReportSurfacesAnUnknownDelivery(t *testing.T) {
	t.Parallel()
	c := serve(t, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(model.ErrorResponse{Error: "no such delivery"})
	})
	err := c.Report("ghost", model.DeliveryReport{OK: true})
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.Status != 404 || statusErr.Message != "no such delivery" {
		t.Fatalf("got %v", err)
	}
}
