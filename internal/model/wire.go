package model

import (
	"strings"
	"time"
)

type RegisterRequest struct {
	ID              string `json:"id"`
	Seed            string `json:"seed,omitempty"`
	Harness         string `json:"harness"`
	Host            string `json:"host"`
	Session         string `json:"session"`
	Folder          string `json:"folder,omitempty"`
	Branch          string `json:"branch,omitempty"`
	Worktree        string `json:"worktree,omitempty"`
	HarnessVersion  string `json:"harness_version,omitempty"`
	LandlineVersion string `json:"landline_version,omitempty"`
	LandlineCommit  string `json:"landline_commit,omitempty"`
	LandlineDate    string `json:"landline_date,omitempty"`
	Model           string `json:"model,omitempty"`
	Effort          string `json:"effort,omitempty"`
	Project         string `json:"project,omitempty"`
}

type RegisterResponse struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

type AttachRequest struct {
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	Project     string `json:"project,omitempty"`
	CodexThread string `json:"codex_thread,omitempty"`
	Code        string `json:"code,omitempty"`
}

type AttachResponse struct {
	ID           string   `json:"id"`
	Updated      []string `json:"updated"`
	Challenge    bool     `json:"challenge"`
	Attached     bool     `json:"attached"`
	ProjectPeers int      `json:"project_peers"`
}

type StatusRequest struct {
	Task string `json:"task"`
}

type StatusResponse struct {
	Task   string    `json:"task"`
	TaskAt time.Time `json:"task_at,omitzero"`
}

type SendBody struct {
	To        string `json:"to"`
	Thread    string `json:"thread"`
	Kind      Kind   `json:"kind"`
	Body      string `json:"body"`
	InReplyTo string `json:"in_reply_to"`
}

type SendResponse struct {
	ID         string   `json:"id"`
	Seq        int64    `json:"seq"`
	Recipients []string `json:"recipients"`
}

type WireEnvelope struct {
	Seq       int64     `json:"seq"`
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	Thread    string    `json:"thread"`
	From      string    `json:"from"`
	FromInfo  string    `json:"from_info"`
	To        string    `json:"to"`
	Kind      Kind      `json:"kind"`
	Body      string    `json:"body"`
	InReplyTo string    `json:"in_reply_to,omitempty"`
	Text      string    `json:"text"`
}

type AgentInfo struct {
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

type ErrorResponse struct {
	Error string `json:"error"`
}

type DeliveryKind string

const (
	DeliveryMessage   DeliveryKind = "message"
	DeliveryChallenge DeliveryKind = "challenge"
)

type Delivery struct {
	ID      string        `json:"id"`
	Kind    DeliveryKind  `json:"kind"`
	Text    string        `json:"text"`
	Target  string        `json:"target,omitempty"`
	Message *WireEnvelope `json:"message,omitempty"`
}

type DeliveryReport struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (a AgentInfo) Attributes() string {
	parts := []string{}
	for _, each := range []struct{ name, value string }{
		{"project", a.Project}, {"task", a.Task}, {"folder", a.Folder}, {"branch", a.Branch}, {"worktree", a.Worktree},
		{"model", a.Model}, {"effort", a.Effort}, {"version", a.HarnessVersion}, {"landline", a.LandlineVersion},
		{"commit", shortCommit(a.LandlineCommit)}, {"built", a.LandlineDate},
	} {
		if each.value != "" {
			parts = append(parts, each.name+": "+oneLine(each.value))
		}
	}
	return strings.Join(parts, "  ")
}

const (
	shortCommitLength = 7
	dirtySuffix       = "-dirty"
)

func shortCommit(commit string) string {
	runes := []rune(commit)
	if len(runes) <= shortCommitLength {
		return commit
	}
	short := string(runes[:shortCommitLength])
	if strings.HasSuffix(commit, dirtySuffix) {
		short += dirtySuffix
	}
	return short
}
