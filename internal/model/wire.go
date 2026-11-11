package model

import "time"

type RegisterRequest struct {
	Name       string `json:"name"`
	Harness    string `json:"harness"`
	Host       string `json:"host"`
	Session    string `json:"session"`
	WakeKind   string `json:"wake_kind"`
	WakeTarget string `json:"wake_target"`
}

type RegisterResponse struct {
	Name  string `json:"name"`
	Token string `json:"token"`
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
}

type AgentInfo struct {
	Name     string    `json:"name"`
	Harness  string    `json:"harness"`
	Host     string    `json:"host"`
	Session  string    `json:"session"`
	WakeKind string    `json:"wake_kind"`
	Online   bool      `json:"online"`
	LastSeen time.Time `json:"last_seen"`
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
