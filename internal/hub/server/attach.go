package server

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

const (
	attachLifetime = 5 * time.Minute
	attachTries    = 5
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type attachChallenge struct {
	thread  string
	code    string
	expires time.Time
	tries   int
}

func (s *Server) attachClaims(w http.ResponseWriter, r *http.Request) {
	name, ok := s.agentFor(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
		return
	}
	var body model.AttachRequest
	if !decode(w, r, &body) {
		return
	}
	if body.Code != "" && body.CodexThread == "" {
		fail(w, http.StatusBadRequest, "a code needs the codex_thread it was sent to")
		return
	}
	if body.CodexThread != "" && !uuidPattern.MatchString(body.CodexThread) {
		fail(w, http.StatusBadRequest, "codex_thread must be a lowercase UUID")
		return
	}
	s.mu.Lock()
	agent, found := s.reg.Find(s.agents, name)
	if !found {
		s.mu.Unlock()
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
		return
	}
	updated := []string{}
	for _, claim := range []struct {
		name  string
		value string
		field *string
	}{{"model", body.Model, &agent.Model}, {"effort", body.Effort, &agent.Effort}, {"label", body.Label, &agent.Label}} {
		if claim.value != "" {
			*claim.field = claim.value
			updated = append(updated, claim.name)
		}
	}
	response := model.AttachResponse{Name: name, Updated: updated}
	var challengeText, challengeThread string
	switch {
	case body.CodexThread != "" && body.Code == "":
		if s.cfg.Wakers["codex"] == nil && s.localHost(agent.Host) {
			s.mu.Unlock()
			fail(w, http.StatusBadRequest, "this hub has no codex wake method")
			return
		}
		code := s.cfg.NewCode()
		s.attach[name] = &attachChallenge{thread: body.CodexThread, code: code, expires: s.cfg.Now().Add(attachLifetime)}
		challengeThread = body.CodexThread
		challengeText = model.Render(model.Envelope{
			ID: "attach", Thread: model.DefaultThread, From: "landline", Kind: model.KindChat,
			Body: fmt.Sprintf("Landline needs to confirm that this session is the agent %s. Call landline_register with codex_thread %s and code %s. Ignore this if you did not start Landline.", name, body.CodexThread, code),
		}, model.Agent{Harness: "landline", Host: s.cfg.Host})
		response.Challenge = true
	case body.CodexThread != "":
		pending := s.attach[name]
		if pending == nil || s.cfg.Now().After(pending.expires) {
			delete(s.attach, name)
			s.mu.Unlock()
			fail(w, http.StatusUnprocessableEntity, "there is no pending code for this agent, or it expired. Ask for a new one")
			return
		}
		pending.tries++
		if pending.thread != body.CodexThread || subtle.ConstantTimeCompare([]byte(pending.code), []byte(strings.ToUpper(body.Code))) != 1 {
			if pending.tries >= attachTries {
				delete(s.attach, name)
			}
			s.mu.Unlock()
			fail(w, http.StatusUnprocessableEntity, "that code does not match")
			return
		}
		delete(s.attach, name)
		agent.WakeKind, agent.WakeTarget = "codex", body.CodexThread
		response.Attached = true
	}
	if len(updated) > 0 || response.Attached {
		revised, err := s.reg.Register(s.agents, agent, s.cfg.Now())
		if err != nil {
			s.mu.Unlock()
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		s.agents = revised
		s.saveLocked()
	}
	remote := !s.localHost(agent.Host)
	host := agent.Host
	s.mu.Unlock()
	if challengeText != "" {
		if err := s.deliverChallenge(name, host, remote, challengeThread, challengeText); err != nil {
			s.mu.Lock()
			delete(s.attach, name)
			s.mu.Unlock()
			fail(w, http.StatusBadGateway, "could not reach that session: "+err.Error())
			return
		}
	}
	write(w, http.StatusOK, response)
}

func (s *Server) deliverChallenge(name, host string, remote bool, thread, text string) error {
	if remote {
		s.mu.Lock()
		s.wakes[host] = append(s.wakes[host], &queuedWake{job: model.WakeJob{
			ID: "attach-" + name + "-" + s.cfg.NewSuffix(), From: "landline", Agent: name, Kind: "codex", Target: thread, Text: text,
		}})
		s.mu.Unlock()
		s.cfg.Notify(fmt.Sprintf("sent an attach code for %s to a Codex session on host %s through its daemon", name, host))
		return nil
	}
	if err := s.cfg.Wakers["codex"].Wake(thread, text); err != nil {
		s.cfg.Notify(fmt.Sprintf("attach code for %s could not be delivered: %v", name, err))
		return err
	}
	s.cfg.Notify(fmt.Sprintf("sent an attach code for %s to a Codex session", name))
	return nil
}
