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
	Name       string
	Harness    string
	Host       string
	Session    string
	WakeKind   string
	WakeTarget string
	LastSeen   time.Time
}

type Envelope struct {
	Seq       int64
	ID        string
	Time      time.Time
	Thread    string
	From      string
	To        string
	Kind      Kind
	Body      string
	InReplyTo string
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
