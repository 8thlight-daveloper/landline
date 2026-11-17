package agentsview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
)

const (
	fullCommit = "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"
	buildDate  = "2026-11-12T16:34:00Z"
)

func decodedAgent(t *testing.T, body string) hubclient.Agent {
	t.Helper()
	var agent hubclient.Agent
	if err := json.Unmarshal([]byte(body), &agent); err != nil {
		t.Fatal(err)
	}
	return agent
}

func exportedRows(t *testing.T, agents ...hubclient.Agent) []map[string]any {
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

func TestTheExportCarriesTheFullCommitAndTheDateUnderTheHubNames(t *testing.T) {
	agent := decodedAgent(t, `{"id":"alpha","harness":"claude","host":"mac","connected":true,"landline_version":"0.1.0","landline_commit":"`+fullCommit+`","landline_date":"`+buildDate+`"}`)
	rows := exportedRows(t, agent)
	if len(rows) != 1 || rows[0]["landline_commit"] != fullCommit || rows[0]["landline_date"] != buildDate || rows[0]["landline_version"] != "0.1.0" {
		t.Fatalf("got %+v", rows)
	}
}

func TestTheExportCarriesTheFullDirtyCommitAndNoDate(t *testing.T) {
	agent := decodedAgent(t, `{"id":"alpha","harness":"claude","host":"mac","landline_commit":"`+fullCommit+`-dirty"}`)
	rows := exportedRows(t, agent)
	if rows[0]["landline_commit"] != fullCommit+"-dirty" {
		t.Fatalf("got %+v", rows[0])
	}
	if _, present := rows[0]["landline_date"]; present {
		t.Fatalf("a date appeared: %+v", rows[0])
	}
}

func TestTheExportLeavesOutAnAbsentCommitAndDate(t *testing.T) {
	rows := exportedRows(t, decodedAgent(t, `{"id":"alpha","harness":"claude","host":"mac"}`))
	for _, key := range []string{"landline_commit", "landline_date"} {
		if _, present := rows[0][key]; present {
			t.Errorf("%s is present: %+v", key, rows[0])
		}
	}
}

func TestTheExportStripsControlCharactersFromTheCommitAndTheDate(t *testing.T) {
	agent := decodedAgent(t, `{"id":"alpha","harness":"claude","host":"mac","landline_commit":"ab\u001b[31mcd","landline_date":"x\u202ey"}`)
	raw, _ := json.Marshal(Build("http://hub", at, []hubclient.Agent{agent}))
	if strings.Contains(string(raw), `\u001b`) || strings.Contains(string(raw), "\xe2\x80\xae") || strings.Contains(string(raw), `\u202e`) {
		t.Fatalf("got %s", raw)
	}
	var decoded struct{ Agents []map[string]any }
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"landline_commit", "landline_date"} {
		text, _ := decoded.Agents[0][key].(string)
		if strings.ContainsAny(text, "\x1b\u202e") {
			t.Errorf("%s holds a forbidden character: %q", key, text)
		}
	}
}
