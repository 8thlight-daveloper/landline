package model

import (
	"strings"
	"time"
)

type RegisterRequest struct {
	Name           string `json:"name"`
	Seed           string `json:"seed,omitempty"`
	Harness        string `json:"harness"`
	Host           string `json:"host"`
	Session        string `json:"session"`
	WakeKind       string `json:"wake_kind"`
	WakeTarget     string `json:"wake_target"`
	Folder         string `json:"folder,omitempty"`
	Branch         string `json:"branch,omitempty"`
	Worktree       string `json:"worktree,omitempty"`
	HarnessVersion string `json:"harness_version,omitempty"`
	Model          string `json:"model,omitempty"`
	Effort         string `json:"effort,omitempty"`
	Label          string `json:"label,omitempty"`
}

type RegisterResponse struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}

type AttachRequest struct {
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	Label       string `json:"label,omitempty"`
	CodexThread string `json:"codex_thread,omitempty"`
	Code        string `json:"code,omitempty"`
}

type AttachResponse struct {
	Name      string   `json:"name"`
	Updated   []string `json:"updated"`
	Challenge bool     `json:"challenge"`
	Attached  bool     `json:"attached"`
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
	Woken      []string `json:"woken"`
	Queued     []string `json:"queued"`
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

type InboxResponse struct {
	Messages  []WireEnvelope `json:"messages"`
	Next      int64          `json:"next"`
	Remaining int            `json:"remaining"`
	Head      int64          `json:"head"`
}

type AgentInfo struct {
	Name           string    `json:"name"`
	Harness        string    `json:"harness"`
	Host           string    `json:"host"`
	Session        string    `json:"session"`
	WakeKind       string    `json:"wake_kind"`
	Online         bool      `json:"online"`
	LastSeen       time.Time `json:"last_seen"`
	Folder         string    `json:"folder,omitempty"`
	Branch         string    `json:"branch,omitempty"`
	Worktree       string    `json:"worktree,omitempty"`
	HarnessVersion string    `json:"harness_version,omitempty"`
	Model          string    `json:"model,omitempty"`
	Effort         string    `json:"effort,omitempty"`
	Label          string    `json:"label,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type WakeJob struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	Agent  string `json:"agent"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Text   string `json:"text"`
}

type WakeAck struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (a AgentInfo) Attributes() string {
	parts := []string{}
	for _, each := range []struct{ name, value string }{
		{"label", a.Label}, {"folder", a.Folder}, {"branch", a.Branch}, {"worktree", a.Worktree},
		{"model", a.Model}, {"effort", a.Effort}, {"version", a.HarnessVersion},
	} {
		if each.value != "" {
			parts = append(parts, each.name+": "+oneLine(each.value))
		}
	}
	return strings.Join(parts, "  ")
}
