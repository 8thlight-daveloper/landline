package hubclient

import "time"

type Agent struct {
	Name            string    `json:"name"`
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
	Model           string    `json:"model,omitempty"`
	Effort          string    `json:"effort,omitempty"`
	Label           string    `json:"label,omitempty"`
}
