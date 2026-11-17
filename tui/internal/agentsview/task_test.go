package agentsview

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
)

func TestTheExportLeavesOutTaskAndTaskAtWhenTheAgentSetNone(t *testing.T) {
	rows := exportedRows(t, hubclient.Agent{ID: "alpha", Harness: "claude", Host: "mac"})
	for _, key := range []string{"task", "task_at"} {
		if _, present := rows[0][key]; present {
			t.Errorf("the key %q is present with no task: %v", key, rows[0])
		}
	}
}

func TestTheExportCarriesTaskAndTaskAtAsAnRFC3339StringWhenSet(t *testing.T) {
	taskAt := time.Date(2026, 11, 16, 9, 15, 30, 0, time.UTC)
	rows := exportedRows(t, hubclient.Agent{ID: "alpha", Harness: "claude", Host: "mac", Project: "landline", Task: "menu bar app for ADR 0140", TaskAt: taskAt})
	if rows[0]["task"] != "menu bar app for ADR 0140" || rows[0]["task_at"] != "2026-11-16T09:15:30Z" || rows[0]["project"] != "landline" || rows[0]["id"] != "alpha" {
		t.Fatalf("got %v", rows[0])
	}
	for _, old := range []string{"handle", "name", "label"} {
		if _, present := rows[0][old]; present {
			t.Errorf("the old key %q is still exported", old)
		}
	}
}

func TestTheHubClientReadsIDProjectTaskAndTaskAtFromTheWireKeys(t *testing.T) {
	agent := decodedAgent(t, `{"id":"a3f9","project":"landline","task":"x","task_at":"2026-11-16T09:15:30Z","harness":"claude","host":"mac"}`)
	if agent.ID != "a3f9" || agent.Project != "landline" || agent.Task != "x" || !agent.TaskAt.Equal(time.Date(2026, 11, 16, 9, 15, 30, 0, time.UTC)) {
		t.Fatalf("got %+v", agent)
	}
}

func TestATaskDoesNotChangeTheSortOrder(t *testing.T) {
	view := Build("h", at, []hubclient.Agent{
		{ID: "bbbb", Project: "p", Task: "a", Connected: true},
		{ID: "aaaa", Project: "p", Task: "z", Connected: true},
	})
	if got := ids(view); got[0] != "aaaa" || got[1] != "bbbb" {
		t.Fatalf("got %v", got)
	}
}

func TestAnExportedRowDecodesBackWithItsTask(t *testing.T) {
	raw, _ := json.Marshal(Build("h", at, []hubclient.Agent{{ID: "a", Task: "write tests", TaskAt: at}}))
	var decoded View
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Agents[0].Task != "write tests" || decoded.Agents[0].TaskAt != "2026-11-12T10:30:00Z" {
		t.Fatalf("%v %s", err, raw)
	}
}
