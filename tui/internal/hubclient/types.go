package hubclient

import "time"

type Agent struct {
	ID              string    `json:"id"`
	Harness         string    `json:"harness"`
	Host            string    `json:"host"`
	Session         string    `json:"session"`
	Connected       bool      `json:"connected"`
	LastSeen        time.Time `json:"last_seen"`
	Folder          string    `json:"folder,omitempty"`
	Branch          string    `json:"branch,omitempty"`
	Worktree        string    `json:"worktree,omitempty"`
	HarnessVersion  string    `json:"harness_version,omitempty"`
	LandlineVersion string    `json:"landline_version,omitempty"`
	LandlineCommit  string    `json:"landline_commit,omitempty"`
	LandlineDate    string    `json:"landline_date,omitempty"`
	Model           string    `json:"model,omitempty"`
	Effort          string    `json:"effort,omitempty"`
	Project         string    `json:"project,omitempty"`
	Task            string    `json:"task,omitempty"`
	TaskAt          time.Time `json:"task_at,omitzero"`
}
