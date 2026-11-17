package agentsview

import (
	"sort"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/tui/internal/clean"
	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
)

const (
	Connected    = "connected"
	Disconnected = "disconnected"
)

type Row struct {
	ID              string `json:"id"`
	Project         string `json:"project,omitempty"`
	Task            string `json:"task,omitempty"`
	TaskAt          string `json:"task_at,omitempty"`
	Harness         string `json:"harness"`
	State           string `json:"state"`
	LandlineVersion string `json:"landline_version,omitempty"`
	LandlineCommit  string `json:"landline_commit,omitempty"`
	LandlineDate    string `json:"landline_date,omitempty"`
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
		taskAt := ""
		if !agent.TaskAt.IsZero() {
			taskAt = agent.TaskAt.UTC().Format(time.RFC3339)
		}
		rows = append(rows, Row{
			ID: clean.Text(agent.ID), Project: clean.Text(agent.Project), Task: clean.Text(agent.Task), TaskAt: taskAt, Harness: clean.Text(agent.Harness), State: state,
			LandlineVersion: clean.Text(agent.LandlineVersion), HarnessVersion: clean.Text(agent.HarnessVersion),
			LandlineCommit: clean.Text(agent.LandlineCommit), LandlineDate: clean.Text(agent.LandlineDate),
			Folder: clean.Text(agent.Folder), Branch: clean.Text(agent.Branch), Worktree: clean.Text(agent.Worktree),
			Model: clean.Text(agent.Model), Effort: clean.Text(agent.Effort), Host: clean.Text(agent.Host),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.State != b.State {
			return a.State == Connected
		}
		if (a.Project == "") != (b.Project == "") {
			return a.Project != ""
		}
		if a.Project != b.Project {
			return a.Project < b.Project
		}
		return a.ID < b.ID
	})
	return View{Screen: "agents", FetchedAt: at.UTC().Format(time.RFC3339), Hub: clean.Text(hub), Agents: rows}
}

const (
	shortCommitLength = 7
	dirtySuffix       = "-dirty"
)

func (r Row) ShortCommit() string {
	runes := []rune(r.LandlineCommit)
	if len(runes) <= shortCommitLength {
		return r.LandlineCommit
	}
	short := string(runes[:shortCommitLength])
	if strings.HasSuffix(r.LandlineCommit, dirtySuffix) {
		short += dirtySuffix
	}
	return short
}
