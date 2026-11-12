package agentsview

import (
	"sort"
	"time"

	"github.com/8thlight-daveloper/landline/tui/internal/clean"
	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
)

const (
	Connected    = "connected"
	Disconnected = "disconnected"
)

type Row struct {
	Handle          string `json:"handle"`
	Label           string `json:"label,omitempty"`
	Harness         string `json:"harness"`
	State           string `json:"state"`
	LandlineVersion string `json:"landline_version,omitempty"`
	HarnessVersion  string `json:"harness_version,omitempty"`
	Folder          string `json:"folder,omitempty"`
	Branch          string `json:"branch,omitempty"`
	Worktree        string `json:"worktree,omitempty"`
	Model           string `json:"model,omitempty"`
	Effort          string `json:"effort,omitempty"`
	Host            string `json:"host"`
}

type View struct {
	Screen    string `json:"screen"`
	FetchedAt string `json:"fetched_at"`
	Hub       string `json:"hub"`
	Agents    []Row  `json:"agents"`
}

func Build(hub string, at time.Time, agents []hubclient.Agent) View {
	rows := make([]Row, 0, len(agents))
	for _, agent := range agents {
		state := Disconnected
		if agent.Connected {
			state = Connected
		}
		rows = append(rows, Row{
			Handle: clean.Text(agent.Name), Label: clean.Text(agent.Label), Harness: clean.Text(agent.Harness), State: state,
			LandlineVersion: clean.Text(agent.LandlineVersion), HarnessVersion: clean.Text(agent.HarnessVersion),
			Folder: clean.Text(agent.Folder), Branch: clean.Text(agent.Branch), Worktree: clean.Text(agent.Worktree),
			Model: clean.Text(agent.Model), Effort: clean.Text(agent.Effort), Host: clean.Text(agent.Host),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.State != b.State {
			return a.State == Connected
		}
		if (a.Label == "") != (b.Label == "") {
			return a.Label != ""
		}
		if a.Label != b.Label {
			return a.Label < b.Label
		}
		return a.Handle < b.Handle
	})
	return View{Screen: "agents", FetchedAt: at.UTC().Format(time.RFC3339), Hub: clean.Text(hub), Agents: rows}
}
