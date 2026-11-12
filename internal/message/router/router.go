package router

import (
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

const maxTokenLength = 64

type Router struct{}

func New() Router { return Router{} }

func (Router) Route(known []model.Agent, from string, request model.SendRequest, id string, seq int64, now time.Time) (model.Decision, error) {
	if !isRegistered(known, from) {
		return model.Decision{}, model.ErrUnknownSender
	}
	thread := request.Thread
	if thread == "" {
		thread = model.DefaultThread
	}
	if !isThread(thread) {
		return model.Decision{}, model.ErrBadThread
	}
	kind := request.Kind
	if kind == "" {
		kind = model.KindChat
	}
	if !kind.Valid() {
		return model.Decision{}, model.ErrBadKind
	}
	if request.InReplyTo != "" && !isReplyToken(request.InReplyTo) {
		return model.Decision{}, model.ErrBadReplyTo
	}
	if strings.TrimSpace(request.Body) == "" {
		return model.Decision{}, model.ErrEmptyBody
	}
	if len(request.Body) > model.MaxBodyBytes {
		return model.Decision{}, model.ErrBodyTooLarge
	}
	if request.To == from {
		return model.Decision{}, model.ErrSelfSend
	}
	if !isRegistered(known, request.To) {
		return model.Decision{}, model.ErrUnknownRecipient
	}
	return model.Decision{
		Envelope: model.Envelope{
			Seq:       seq,
			ID:        id,
			Time:      now,
			Thread:    thread,
			From:      from,
			To:        request.To,
			Kind:      kind,
			Body:      request.Body,
			InReplyTo: request.InReplyTo,
		},
		Recipients: []string{request.To},
	}, nil
}

func isRegistered(known []model.Agent, name string) bool {
	for _, agent := range known {
		if agent.Name == name {
			return true
		}
	}
	return false
}

func isThread(thread string) bool {
	if len(thread) == 0 || len(thread) > maxTokenLength {
		return false
	}
	for index := 0; index < len(thread); index++ {
		character := thread[index]
		isAlphanumeric := isLowercaseLetter(character) || isDigit(character)
		if index == 0 && !isAlphanumeric {
			return false
		}
		if !isAlphanumeric && !isPunctuation(character) {
			return false
		}
	}
	return true
}

func isReplyToken(token string) bool {
	if len(token) > maxTokenLength {
		return false
	}
	for index := 0; index < len(token); index++ {
		character := token[index]
		if !isLowercaseLetter(character) && !isUppercaseLetter(character) && !isDigit(character) && !isPunctuation(character) {
			return false
		}
	}
	return true
}

func isLowercaseLetter(character byte) bool { return character >= 'a' && character <= 'z' }

func isUppercaseLetter(character byte) bool { return character >= 'A' && character <= 'Z' }

func isDigit(character byte) bool { return character >= '0' && character <= '9' }

func isPunctuation(character byte) bool {
	return character == '.' || character == '-' || character == '_'
}
