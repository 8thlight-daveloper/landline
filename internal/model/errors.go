package model

import "errors"

var (
	ErrBadName          = errors.New("agent name must be 1 to 32 lowercase letters, digits, - or _, and not \"all\"")
	ErrNoHarness        = errors.New("an agent must name its harness")
	ErrUnknownSender    = errors.New("the sender is not a registered agent")
	ErrUnknownRecipient = errors.New("the recipient is not a registered agent")
	ErrSelfSend         = errors.New("an agent cannot send a message to itself")
	ErrBadThread        = errors.New("thread must be 1 to 64 lowercase letters, digits, . - or _")
	ErrBadKind          = errors.New("kind must be chat, task, reply, or review")
	ErrEmptyBody        = errors.New("a message needs a body")
	ErrBodyTooLarge     = errors.New("the body is larger than 64 KiB")
	ErrBadReplyTo       = errors.New("in_reply_to must be 1 to 64 letters, digits, . - or _")
	ErrBadField         = errors.New("harness, host, session, and wake fields must be 1 to 64 printable characters on one line")
)
