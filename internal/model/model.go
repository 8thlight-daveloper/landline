package model

import "time"

const (
	Broadcast     = "all"
	DefaultThread = "main"
	MaxBodyBytes  = 64 * 1024
	Marker        = "From another agent, not the user."
)

type Kind string

const (
	KindChat   Kind = "chat"
	KindTask   Kind = "task"
	KindReply  Kind = "reply"
	KindReview Kind = "review"
)

func (k Kind) Valid() bool {
	switch k {
	case KindChat, KindTask, KindReply, KindReview:
		return true
	}
	return false
}

type Agent struct {
	Name       string    `json:"name"`
	Harness    string    `json:"harness"`
	Host       string    `json:"host"`
	Session    string    `json:"session"`
	WakeKind   string    `json:"wake_kind"`
	WakeTarget string    `json:"wake_target"`
	LastSeen   time.Time `json:"last_seen"`
}

type Envelope struct {
	Seq       int64     `json:"seq"`
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	Thread    string    `json:"thread"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Kind      Kind      `json:"kind"`
	Body      string    `json:"body"`
	InReplyTo string    `json:"in_reply_to,omitempty"`
}

type SendRequest struct {
	To        string
	Thread    string
	Kind      Kind
	Body      string
	InReplyTo string
}

type Decision struct {
	Envelope   Envelope
	Recipients []string
}

type Page struct {
	Messages  []Envelope
	Next      int64
	Remaining int
}
