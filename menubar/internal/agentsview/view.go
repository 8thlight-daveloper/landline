package agentsview

import (
	"sort"

	"github.com/8thlight-daveloper/landline/menubar/internal/clean"
	"github.com/8thlight-daveloper/landline/menubar/internal/hubclient"
)

const (
	Active       = "active"
	Disconnected = "disconnected"
)

type Row struct {
	Handle          string
	Label           string
	Harness         string
	Host            string
	Marker          string
	Model           string
	Effort          string
	Folder          string
	Branch          string
	LandlineVersion string
}

func (r Row) Connected() bool { return r.Marker != Disconnected }

func (r Row) Title() string {
	if r.Label != "" {
		return r.Label
	}
	return r.Handle
}

func Build(agents []hubclient.Agent) []Row {
	rows := make([]Row, 0, len(agents))
	for _, agent := range agents {
		marker := Disconnected
		if agent.Connected {
			marker = Active
		}
		rows = append(rows, Row{
			Handle: clean.Text(agent.Name), Label: clean.Text(agent.Label), Harness: clean.Text(agent.Harness),
			Host: clean.Text(agent.Host), Marker: marker, Model: clean.Text(agent.Model), Effort: clean.Text(agent.Effort),
			Folder: clean.Text(agent.Folder), Branch: clean.Text(agent.Branch),
			LandlineVersion: clean.Text(agent.LandlineVersion),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Connected() != b.Connected() {
			return a.Connected()
		}
		if (a.Label == "") != (b.Label == "") {
			return a.Label != ""
		}
		if a.Label != b.Label {
			return a.Label < b.Label
		}
		return a.Handle < b.Handle
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
