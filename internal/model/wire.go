package model

import (
	"strings"
	"time"
)

type RegisterRequest struct {
	Name            string `json:"name"`
	Seed            string `json:"seed,omitempty"`
	Harness         string `json:"harness"`
	Host            string `json:"host"`
	Session         string `json:"session"`
	Folder          string `json:"folder,omitempty"`
	Branch          string `json:"branch,omitempty"`
	Worktree        string `json:"worktree,omitempty"`
	HarnessVersion  string `json:"harness_version,omitempty"`
	LandlineVersion string `json:"landline_version,omitempty"`
	Model           string `json:"model,omitempty"`
	Effort          string `json:"effort,omitempty"`
	Label           string `json:"label,omitempty"`
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
		{"label", a.Label}, {"folder", a.Folder}, {"branch", a.Branch}, {"worktree", a.Worktree},
		{"model", a.Model}, {"effort", a.Effort}, {"version", a.HarnessVersion}, {"landline", a.LandlineVersion},
	} {
		if each.value != "" {
			parts = append(parts, each.name+": "+oneLine(each.value))
		}
	}
	return strings.Join(parts, "  ")
}
