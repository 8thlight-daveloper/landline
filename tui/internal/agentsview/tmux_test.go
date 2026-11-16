package agentsview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
)

func tmuxExportedRows(t *testing.T, agents []hubclient.Agent) []map[string]any {
	t.Helper()
	raw, err := json.Marshal(Build("http://hub", at, agents))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct{ Agents []map[string]any }
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded.Agents
}

func TestTheRowCarriesTheTmuxSocketAndPaneAndTheJSONExportNamesThem(t *testing.T) {
	view := Build("http://hub", at, []hubclient.Agent{{Name: "alpha", Harness: "claude", Host: "mac", Connected: true, TmuxSocket: "default", TmuxPane: "%3"}})
	if view.Agents[0].TmuxSocket != "default" || view.Agents[0].TmuxPane != "%3" {
		t.Fatalf("row %+v", view.Agents[0])
	}
	rows := tmuxExportedRows(t, []hubclient.Agent{{Name: "alpha", Harness: "claude", Host: "mac", Connected: true, TmuxSocket: "default", TmuxPane: "%3"}})
	if rows[0]["tmux_socket"] != "default" || rows[0]["tmux_pane"] != "%3" {
		t.Fatalf("row %v", rows[0])
	}
}

func TestTheJSONExportLeavesOutTheTmuxKeysForAnAgentWithNone(t *testing.T) {
	rows := tmuxExportedRows(t, []hubclient.Agent{{Name: "alpha", Harness: "claude", Host: "mac"}})
	for _, key := range []string{"tmux_socket", "tmux_pane"} {
		if _, present := rows[0][key]; present {
			t.Errorf("%s is present: %v", key, rows[0])
		}
	}
	raw, _ := json.Marshal(Build("h", at, []hubclient.Agent{{Name: "bravo", Harness: "codex", Host: "mac"}}))
	if strings.Contains(string(raw), "tmux") {
		t.Fatalf("got %s", raw)
	}
}

func TestTheTmuxValuesOfTheRowCarryNoControlRune(t *testing.T) {
	hostile := "a\x1b]0;pwned\x07\n‮b"
	rows := tmuxExportedRows(t, []hubclient.Agent{{Name: "alpha", Harness: "claude", Host: "mac", TmuxSocket: hostile, TmuxPane: hostile}})
	for _, key := range []string{"tmux_socket", "tmux_pane"} {
		text, _ := rows[0][key].(string)
		for _, r := range text {
			if r < ' ' || r == 0x7f || r == 0x202e || r == 0x85 {
				t.Errorf("%s holds %U: %q", key, r, text)
			}
		}
	}
}
