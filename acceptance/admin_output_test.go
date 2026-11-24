package acceptance

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const hostileField = "alpha\x1b[2J\x1b]0;owned\x07\tbravo\nlandline server revoke --all\rend"

func startHostileAdminSwitchboard(t *testing.T) (string, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/machines":
			json.NewEncoder(w).Encode([]map[string]string{{"name": hostileField, "fingerprint": hostileField, "created": hostileField}})
		case r.Method == "POST" && r.URL.Path == "/v1/machines":
			json.NewEncoder(w).Encode(map[string]string{"name": "bravo", "code": hostileField})
		default:
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": hostileField})
		}
	}))
	t.Cleanup(server.Close)
	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	return host, port
}

func startMalformedHeaderSwitchboard(t *testing.T) (string, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				reader := bufio.NewReader(conn)
				for {
					line, err := reader.ReadString('\n')
					if err != nil || line == "\r\n" {
						break
					}
				}
				conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n" + strings.ReplaceAll(hostileField, "\n", " ") + "\r\n\r\n"))
			}(conn)
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	return host, port
}

func uncleanOutsideLineEnds(text string) bool {
	return strings.ContainsFunc(strings.ReplaceAll(text, "\n", ""), unclean)
}

func TestAdministrationCommandsPrintEveryValueFromTheSwitchboardCleanedOnOneLine(t *testing.T) {
	t.Parallel()
	bin := build(t)
	cases := map[string]struct {
		start func(t *testing.T) (string, string)
		args  []string
		lines int
	}{
		"server machines rows":          {startHostileAdminSwitchboard, []string{"server", "machines"}, 1},
		"server invite code":            {startHostileAdminSwitchboard, []string{"server", "invite", "--name", "bravo"}, 1},
		"a refusal message":             {startHostileAdminSwitchboard, []string{"server", "revoke", "bravo"}, 1},
		"an error with no message":      {startMalformedHeaderSwitchboard, []string{"server", "revoke", "bravo"}, 1},
		"an error with no message, all": {startMalformedHeaderSwitchboard, []string{"server", "machines"}, 1},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			host, port := c.start(t)
			home, env := secretHomeFor(t, host, port)
			out, _ := runIn(t, bin, env, home, c.args...)
			if !strings.Contains(out, "alpha") || !strings.Contains(out, "bravo") {
				t.Errorf("%v: the switchboard's value is not shown: %q", c.args, out)
			}
			if got := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1; got != c.lines {
				t.Errorf("%v: %d lines, want %d: %q", c.args, got, c.lines, out)
			}
			body := out
			if c.args[1] == "machines" && strings.Count(out, "\t") == 2 {
				body = strings.ReplaceAll(out, "\t", " ")
			}
			if uncleanOutsideLineEnds(body) {
				t.Errorf("%v: a control character from the switchboard reached the terminal: %q", c.args, out)
			}
		})
	}
}
