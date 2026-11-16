package command

import (
	"net/http/httptest"
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

func registeredWith(t *testing.T, variables map[string]string) model.RegisterRequest {
	t.Helper()
	hub := &recordingHub{}
	srv := httptest.NewServer(hub.handler())
	defer srv.Close()
	base := map[string]string{"LANDLINE_URL": srv.URL, "LANDLINE_SECRET": "0123456789abcdef", "CLAUDE_CODE_SESSION_ID": "cc-1"}
	for key, value := range variables {
		base[key] = value
	}
	env := Env{ConfigDir: t.TempDir(), Host: "mac", Getenv: func(k string) string { return base[k] }}
	detect := func() Identity { return Identity{Seed: "threnos", Folder: "threnos"} }
	s := NewSession(env, detect, Identity{Label: "lead", LandlineVersion: "0.2.0"})
	defer s.Close()
	s.SetClient("claude-code", "2.1.296")
	if _, err := s.Send(model.SendBody{To: "x", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if len(hub.register) != 1 {
		t.Fatalf("registrations %d", len(hub.register))
	}
	return hub.register[0]
}

func TestTheSessionRegistersTheTmuxSocketAndPaneItReadFromTMUXAndTMUXPANE(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct{ tmux, socket string }{
		"tmp":         {"/tmp/tmux-501/default,1234,0", "default"},
		"private tmp": {"/private/tmp/tmux-501/work-2,99,3", "work-2"},
		"root":        {"/tmp/tmux-0/default,1,0", "default"},
	} {
		got := registeredWith(t, map[string]string{"TMUX": c.tmux, "TMUX_PANE": "%3"})
		if got.TmuxSocket != c.socket || got.TmuxPane != "%3" {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestTheSessionRegistersNoTmuxAttributesOutsideTmuxOrForASocketOutsideTheDefaultDirectories(t *testing.T) {
	t.Parallel()
	for name, variables := range map[string]map[string]string{
		"no variables":     {},
		"no TMUX":          {"TMUX_PANE": "%3"},
		"TMUX_TMPDIR":      {"TMUX": "/tmp/tmux-501/default,1,0", "TMUX_PANE": "%3", "TMUX_TMPDIR": "/tmp"},
		"another folder":   {"TMUX": "/home/dev/sockets/default,1,0", "TMUX_PANE": "%3"},
		"a name not tmux":  {"TMUX": "/tmp/other-501/default,1,0", "TMUX_PANE": "%3"},
		"letters for uid":  {"TMUX": "/tmp/tmux-abc/default,1,0", "TMUX_PANE": "%3"},
		"nested directory": {"TMUX": "/tmp/tmux-501/sub/default,1,0", "TMUX_PANE": "%3"},
	} {
		got := registeredWith(t, variables)
		if got.TmuxSocket != "" || got.TmuxPane != "" {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestNoLandlineVariableAndNoOverrideSetsTheTmuxAttributes(t *testing.T) {
	t.Parallel()
	got := registeredWith(t, map[string]string{"LANDLINE_TMUX_SOCKET": "forged", "LANDLINE_TMUX_PANE": "%9", "LANDLINE_TMUX": "/tmp/tmux-501/forged,1,0"})
	if got.TmuxSocket != "" || got.TmuxPane != "" {
		t.Fatalf("%+v", got)
	}
}
