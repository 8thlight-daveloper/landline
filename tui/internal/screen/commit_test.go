package screen

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
	tea "github.com/charmbracelet/bubbletea"
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

func viewOf(t *testing.T, agent hubclient.Agent) string {
	t.Helper()
	m, _ := send(newModel(), tea.WindowSizeMsg{Width: 240, Height: 20})
	m, _ = send(m, snapshotMsg{agents: []hubclient.Agent{agent}, at: clock})
	return m.View()
}

func TestTheScreenShowsTheShortCommitAndNeitherTheFullCommitNorTheDate(t *testing.T) {
	view := viewOf(t, decodedAgent(t, `{"name":"alpha","harness":"claude","host":"mac","connected":true,"landline_version":"0.1.0","landline_commit":"`+fullCommit+`","landline_date":"`+buildDate+`"}`))
	if !strings.Contains(view, "a1b2c3d") {
		t.Fatalf("no short commit in\n%s", view)
	}
	for _, unwanted := range []string{fullCommit, "a1b2c3d4", "2026-11-12", "16:34:00"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("the screen shows %q:\n%s", unwanted, view)
		}
	}
}

func TestTheScreenKeepsTheDirtyMarkAfterTheShortCommit(t *testing.T) {
	view := viewOf(t, decodedAgent(t, `{"name":"alpha","harness":"claude","host":"mac","connected":true,"landline_commit":"`+fullCommit+`-dirty"}`))
	if !strings.Contains(view, "a1b2c3d-dirty") || strings.Contains(view, fullCommit) {
		t.Fatalf("got\n%s", view)
	}
}

func TestTheScreenShowsNoCommitForAnAgentThatSentNone(t *testing.T) {
	view := viewOf(t, decodedAgent(t, `{"name":"alpha","harness":"claude","host":"mac","connected":true,"landline_version":"0.1.0"}`))
	if strings.Contains(view, "dirty") || strings.Contains(view, "a1b2c3d") {
		t.Fatalf("got\n%s", view)
	}
}

func TestTheCommitColumnIsLastAndFirstAppearsAtWidthOneHundredFiftyOne(t *testing.T) {
	agent := decodedAgent(t, `{"name":"agent00","harness":"claude","host":"mac","connected":true,"landline_commit":"`+fullCommit+`-dirty"}`)
	for width, want := range map[int]bool{80: false, 149: false, 150: false, 151: true, 200: true} {
		view := rendered([]hubclient.Agent{agent}, width)
		if got := strings.Contains(strings.Split(view, "\n")[0], "COMMIT"); got != want {
			t.Errorf("width %d: COMMIT shown %v, want %v\n%s", width, got, want, view)
		}
		if want && !strings.Contains(view, "a1b2c3d-dirty") {
			t.Errorf("width %d: the whole short dirty commit is not shown\n%s", width, view)
		}
	}
}

func TestTheShortCommitIsTheFirstSevenCharactersAndNotTheFirstSevenBytes(t *testing.T) {
	for commit, want := range map[string]string{
		"éàüöíóúñç":       "éàüöíóú",
		"éàüöíóúñç-dirty": "éàüöíóú-dirty",
	} {
		view := viewOf(t, decodedAgent(t, `{"name":"alpha","harness":"claude","host":"mac","connected":true,"landline_commit":"`+commit+`"}`))
		if !strings.Contains(view, want) || strings.Contains(view, "éàüöíóúñ") {
			t.Errorf("%q: want %q in\n%s", commit, want, view)
		}
	}
}
