package command

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const refusalThatMentionsATransportError = "dial tcp 127.0.0.1:1: connect: connection refused; no such host"

func switchboardRefusingWith(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var answered atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answered.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &answered
}

func savedSwitchboard(t *testing.T, dir string, server *httptest.Server, extraLines string) {
	t.Helper()
	host, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "config.toml", fmt.Sprintf("host = %q\nport = %s\n%s", host, port, extraLines))
}

func TestTheSessionGivesTheSignedRefusalAdviceWhenARefusalMessageMentionsATransportError(t *testing.T) {
	t.Parallel()
	server, answered := switchboardRefusingWith(t, `{"error":"`+refusalThatMentionsATransportError+`","reason":"refused"}`)
	dir := t.TempDir()
	savedSwitchboard(t, dir, server, "machine = \"linux-box\"\n")
	env := Env{ConfigDir: dir, Host: "mac", Getenv: func(string) string { return "" }}
	if err := writeMachineKey(env, mustKey(t)); err != nil {
		t.Fatal(err)
	}
	s := NewSession(env, func() Identity { return Identity{Seed: "alpha", Folder: "alpha"} }, Identity{})

	_, err := s.Agents()

	if answered.Load() == 0 {
		t.Fatal("the session never reached the switchboard")
	}
	if err == nil {
		t.Fatal("a refused registration gave no error")
	}
	if strings.Contains(err.Error(), "not running") || strings.Contains(err.Error(), "landline server up") {
		t.Errorf("the switchboard answered %d times with a 401, and the session said it is not running: %v", answered.Load(), err)
	}
	if !strings.Contains(err.Error(), "join again") {
		t.Errorf("the session did not give the advice for a refused machine key: %v", err)
	}
}

func TestTheSessionDoesNotSayTheSwitchboardIsNotRunningWhenItRefusedTheSecretWithAMessageThatMentionsATransportError(t *testing.T) {
	t.Parallel()
	server, answered := switchboardRefusingWith(t, `{"error":"`+refusalThatMentionsATransportError+`"}`)
	dir := t.TempDir()
	savedSwitchboard(t, dir, server, "secret = \""+longSecret+"\"\n")
	env := Env{ConfigDir: dir, Host: "mac", Getenv: func(string) string { return "" }}
	s := NewSession(env, func() Identity { return Identity{Seed: "alpha", Folder: "alpha"} }, Identity{})

	_, err := s.Agents()

	if answered.Load() == 0 {
		t.Fatal("the session never reached the switchboard")
	}
	if err == nil || strings.Contains(err.Error(), "not running") || strings.Contains(err.Error(), "landline server up") {
		t.Errorf("the switchboard answered %d times with a 401, and the session said: %v", answered.Load(), err)
	}
}
