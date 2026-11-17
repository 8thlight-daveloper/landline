package agentsview

import (
	"encoding/json"
	"testing"

	"github.com/8thlight-daveloper/landline/menubar/internal/hubclient"
)

func agent(name, project string, connected bool) hubclient.Agent {
	return hubclient.Agent{ID: name, Project: project, Connected: connected, Harness: "claude-code", Host: "mac"}
}

func ids(rows []Row) []string {
	var out []string
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out
}

func TestRowsComeConnectedFirstThenWithAProjectThenByProjectThenID(t *testing.T) {
	rows := Build([]hubclient.Agent{
		agent("tango", "", true), agent("bravo", "zed", true), agent("alpha", "amy", true),
		agent("echo", "yankee", false), agent("delta", "", false), agent("charlie", "", true),
	})
	want := []string{"alpha", "bravo", "charlie", "tango", "echo", "delta"}
	got := ids(rows)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestStateIsConnectedForAConnectedAgentAndDisconnectedOtherwise(t *testing.T) {
	rows := Build([]hubclient.Agent{agent("a", "", true), agent("b", "", false)})
	if rows[0].State != Connected || rows[1].State != Disconnected {
		t.Fatalf("%+v", rows)
	}
	if Count(rows) != 1 {
		t.Fatalf("count %d", Count(rows))
	}
}

func TestTitleIsTheProjectThenTheIDOrTheIDAlone(t *testing.T) {
	if (Row{ID: "h", Project: "l"}).Title() != "l h" || (Row{ID: "h"}).Title() != "h" {
		t.Fatal("title")
	}
}

func TestEveryTextFieldIsCleanedAndEachSeparatorOrControlBecomesExactlyOneSpace(t *testing.T) {
	for _, bad := range []string{"\x1b", "\n", "\t", "\x00", "\u0085", "\u009b", "\u2028", "\u2029", "\u00ad", "\u200b", "\u202e"} {
		value := "a" + bad + "b"
		rows := Build([]hubclient.Agent{{
			ID: value, Project: value, Task: value, Harness: value, Host: value, Model: value, Effort: value,
			Folder: value, Branch: value, LandlineVersion: value, Connected: true,
		}})
		row := rows[0]
		for name, got := range map[string]string{"id": row.ID, "project": row.Project, "task": row.Task, "harness": row.Harness,
			"host": row.Host, "model": row.Model, "effort": row.Effort, "folder": row.Folder, "branch": row.Branch, "version": row.LandlineVersion} {
			if got != "a b" {
				t.Errorf("%s: %q becomes %q, want %q", name, value, got, "a b")
			}
		}
	}
}

func TestARowBuiltFromTheOldKeysNameAndLabelHasNoIDAndNoProject(t *testing.T) {
	var agent hubclient.Agent
	if err := json.Unmarshal([]byte(`{"name":"x","label":"y","connected":true}`), &agent); err != nil {
		t.Fatal(err)
	}
	row := Build([]hubclient.Agent{agent})[0]
	if row.ID != "" || row.Project != "" {
		t.Fatalf("an old key reached the row: %+v", row)
	}
}

func TestCountOfNoRowsIsZero(t *testing.T) {
	if Count(nil) != 0 {
		t.Fatal("count")
	}
}

func TestTheTaskIsCarriedIntoTheRow(t *testing.T) {
	rows := Build([]hubclient.Agent{{ID: "a3f9", Project: "landline", Task: "write tests", Connected: true}})
	if rows[0].Task != "write tests" || rows[0].Project != "landline" || rows[0].ID != "a3f9" {
		t.Fatalf("%+v", rows[0])
	}
}

func TestATaskDoesNotChangeTheOrderOfRows(t *testing.T) {
	rows := Build([]hubclient.Agent{
		{ID: "bbbb", Project: "p", Task: "a", Connected: true},
		{ID: "aaaa", Project: "p", Task: "z", Connected: true},
	})
	if got := ids(rows); got[0] != "aaaa" || got[1] != "bbbb" {
		t.Fatalf("%v", got)
	}
}
