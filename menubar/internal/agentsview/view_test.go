package agentsview

import (
	"testing"

	"github.com/8thlight-daveloper/landline/menubar/internal/hubclient"
)

func agent(name, label string, connected bool) hubclient.Agent {
	return hubclient.Agent{Name: name, Label: label, Connected: connected, Harness: "claude-code", Host: "mac"}
}

func handles(rows []Row) []string {
	var out []string
	for _, row := range rows {
		out = append(out, row.Handle)
	}
	return out
}

func TestRowsComeConnectedFirstThenLabelledThenByLabelThenHandle(t *testing.T) {
	rows := Build([]hubclient.Agent{
		agent("tango", "", true), agent("bravo", "zed", true), agent("alpha", "amy", true),
		agent("echo", "yankee", false), agent("delta", "", false), agent("charlie", "", true),
	})
	want := []string{"alpha", "bravo", "charlie", "tango", "echo", "delta"}
	got := handles(rows)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestMarkerIsActiveForAConnectedAgentAndDisconnectedOtherwise(t *testing.T) {
	rows := Build([]hubclient.Agent{agent("a", "", true), agent("b", "", false)})
	if rows[0].Marker != Active || rows[1].Marker != Disconnected {
		t.Fatalf("%+v", rows)
	}
	if Count(rows) != 1 {
		t.Fatalf("count %d", Count(rows))
	}
}

func TestTitleIsTheLabelOrTheHandle(t *testing.T) {
	if (Row{Handle: "h", Label: "l"}).Title() != "l" || (Row{Handle: "h"}).Title() != "h" {
		t.Fatal("title")
	}
}

func TestEveryTextFieldIsCleaned(t *testing.T) {
	rows := Build([]hubclient.Agent{{
		Name: "a\x1bb", Label: "l\nm", Harness: "h\t", Host: "x ", Model: "m\x00", Effort: "e\r",
		Folder: "f‮", Branch: "b\a", LandlineVersion: "v​", Connected: true,
	}})
	row := rows[0]
	for _, value := range []string{row.Handle, row.Label, row.Harness, row.Host, row.Model, row.Effort, row.Folder, row.Branch, row.LandlineVersion} {
		for _, r := range value {
			if r < ' ' || r == 0x2028 || r == 0x202e || r == 0x200b {
				t.Errorf("%q keeps %U", value, r)
			}
		}
	}
}

func TestCountOfNoRowsIsZero(t *testing.T) {
	if Count(nil) != 0 {
		t.Fatal("count")
	}
}
