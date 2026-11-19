package model

import (
	"errors"
	"fmt"
)

var (
	ErrBadName          = errors.New("agent id must be 1 to 32 lowercase letters, digits, - or _, and not \"all\"")
	ErrNoHarness        = errors.New("an agent must name its harness")
	ErrUnknownSender    = errors.New("the sender is not a registered agent")
	ErrUnknownRecipient = errors.New("the recipient is not a registered agent")
	ErrSelfSend         = errors.New("an agent cannot send a message to itself")
	ErrBadThread        = errors.New("thread must be 1 to 64 lowercase letters, digits, . - or _")
	ErrBadKind          = errors.New("kind must be chat, task, reply, or review")
	ErrEmptyBody        = errors.New("a message needs a body")
	ErrBodyTooLarge     = errors.New("the body is larger than 64 KiB")
	ErrBadReplyTo       = errors.New("in_reply_to must be 1 to 64 letters, digits, . - or _")
	ErrBadField         = errors.New("agent text fields must be at most 64 printable characters on one line")

	ErrBadMachineName  = errors.New("a machine name is 1 to 32 lowercase letters, digits, or hyphens, and starts with a letter")
	ErrMachineHeld     = errors.New("a machine or a pending invite already holds that name. Revoke it first")
	ErrCodeInUse       = errors.New("another pending invite already has that code")
	ErrNotEnoughRandom = errors.New("an invite code needs at least 8 random bytes")
	ErrInvalidCode     = errors.New("invalid or expired invite code")
	ErrBadPublicKey    = errors.New("a machine public key is 32 bytes")
	ErrUnknownMachine  = errors.New("no machine has that name")
	ErrBadSignature    = errors.New("the machine signature does not verify")
	ErrReplayed        = errors.New("the machine signature was already used")
	ErrBeforeStart     = errors.New("the signature is timed before the switchboard started")
)

type ClockSkewError struct{ Seconds int64 }

func (e *ClockSkewError) Error() string {
	return fmt.Sprintf("the clock of this machine differs from the switchboard's by %d seconds. Set the clock", e.Seconds)
}
