package agentsview

import (
	"sort"

	"github.com/8thlight-daveloper/landline/menubar/internal/clean"
	"github.com/8thlight-daveloper/landline/menubar/internal/hubclient"
)

const (
	Connected    = "connected"
	Disconnected = "disconnected"
)

type Row struct {
	ID              string
	Project         string
	Task            string
	Harness         string
	Host            string
	State           string
	Model           string
	Effort          string
	Folder          string
	Branch          string
	LandlineVersion string
}

func (r Row) Connected() bool { return r.State == Connected }

func (r Row) Title() string {
	if r.Project != "" {
		return r.Project + " " + r.ID
	}
	return r.ID
}

func Build(agents []hubclient.Agent) []Row {
	rows := make([]Row, 0, len(agents))
	for _, agent := range agents {
		state := Disconnected
		if agent.Connected {
			state = Connected
		}
		rows = append(rows, Row{
			ID: clean.Text(agent.ID), Project: clean.Text(agent.Project), Task: clean.Text(agent.Task), Harness: clean.Text(agent.Harness),
			Host: clean.Text(agent.Host), State: state, Model: clean.Text(agent.Model), Effort: clean.Text(agent.Effort),
			Folder: clean.Text(agent.Folder), Branch: clean.Text(agent.Branch),
			LandlineVersion: clean.Text(agent.LandlineVersion),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Connected() != b.Connected() {
			return a.Connected()
		}
		if (a.Project == "") != (b.Project == "") {
			return a.Project != ""
		}
		if a.Project != b.Project {
			return a.Project < b.Project
		}
		return a.ID < b.ID
	})
	return rows
}

func Count(rows []Row) int {
	n := 0
	for _, row := range rows {
		if row.Connected() {
			n++
		}
	}
	return n
}
