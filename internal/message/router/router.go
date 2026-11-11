package router

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

var replyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
var threadPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type Router struct{}

func New() Router { return Router{} }

func (Router) Route(known []model.Agent, from string, request model.SendRequest, id string, seq int64, now time.Time) (model.Decision, error) {
	if !has(known, from) {
		return model.Decision{}, model.ErrUnknownSender
	}
	thread := request.Thread
	if thread == "" {
		thread = model.DefaultThread
	}
	if !threadPattern.MatchString(thread) {
		return model.Decision{}, model.ErrBadThread
	}
	kind := request.Kind
	if kind == "" {
		kind = model.KindChat
	}
	if !kind.Valid() {
		return model.Decision{}, model.ErrBadKind
	}
	if request.InReplyTo != "" && !replyPattern.MatchString(request.InReplyTo) {
		return model.Decision{}, model.ErrBadReplyTo
	}
	if strings.TrimSpace(request.Body) == "" {
		return model.Decision{}, model.ErrEmptyBody
	}
	if len(request.Body) > model.MaxBodyBytes {
		return model.Decision{}, model.ErrBodyTooLarge
	}
	recipients, err := recipientsOf(known, from, request.To)
	if err != nil {
		return model.Decision{}, err
	}
	return model.Decision{
		Envelope: model.Envelope{
			Seq: seq, ID: id, Time: now, Thread: thread, From: from, To: request.To,
			Kind: kind, Body: request.Body, InReplyTo: request.InReplyTo,
		},
		Recipients: recipients,
	}, nil
}

func recipientsOf(known []model.Agent, from, to string) ([]string, error) {
	if to == model.Broadcast {
		out := []string{}
		for _, each := range known {
			if each.Name != from {
				out = append(out, each.Name)
			}
		}
		slices.Sort(out)
		return slices.Compact(out), nil
	}
	if to == from {
		return nil, model.ErrSelfSend
	}
	if !has(known, to) {
		return nil, model.ErrUnknownRecipient
	}
	return []string{to}, nil
}

func has(known []model.Agent, name string) bool {
	for _, each := range known {
		if each.Name == name {
			return true
		}
	}
	return false
}
