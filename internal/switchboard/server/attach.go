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
	name, ok := s.agentFor(w, r)
	if !ok {
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
	}{{"model", body.Model, &agent.Model}, {"effort", body.Effort, &agent.Effort}, {"project", body.Project, &agent.Project}} {
		if claim.value != "" {
			*claim.field = claim.value
			updated = append(updated, claim.name)
		}
	}
	response := model.AttachResponse{ID: name, Updated: updated, ProjectPeers: s.projectPeersLocked(agent)}
	var challengeText, challengeThread string
	switch {
	case body.CodexThread != "" && body.Code == "":
		if !s.takeChallengeSlot(name) {
			s.mu.Unlock()
			fail(w, http.StatusTooManyRequests, "too many proof challenges, try again later")
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
		response.Attached = true
	}
	if len(updated) > 0 {
		revised, err := s.reg.Register(s.agents, agent, s.cfg.Now())
		if err != nil {
			s.mu.Unlock()
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		s.agents = revised
	}
	s.mu.Unlock()
	if challengeText != "" {
		status, problem := s.deliver(r.Context(), name, model.Delivery{Kind: model.DeliveryChallenge, Text: challengeText, Target: challengeThread})
		if status != http.StatusOK {
			s.mu.Lock()
			delete(s.attach, name)
			s.mu.Unlock()
			s.cfg.Notify(fmt.Sprintf("attach code for %s could not be delivered: %s", name, problem))
			if status == http.StatusConflict || status == http.StatusGatewayTimeout {
				fail(w, status, problem)
			} else {
				fail(w, http.StatusBadGateway, "could not reach that session: "+problem)
			}
			return
		}
		s.cfg.Notify(fmt.Sprintf("sent an attach code for %s to a Codex session", name))
	}
	write(w, http.StatusOK, response)
}

func (s *Server) takeChallengeSlot(name string) bool {
	now := s.cfg.Now()
	recent := []time.Time{}
	for _, at := range s.sent[name] {
		if now.Sub(at) < s.cfg.ChallengeWindow {
			recent = append(recent, at)
		}
	}
	if len(recent) >= s.cfg.ChallengeLimit {
		s.sent[name] = recent
		return false
	}
	s.sent[name] = append(recent, now)
	return true
}

func (s *Server) projectPeersLocked(caller model.Agent) int {
	if caller.Project == "" {
		return 0
	}
	peers := 0
	for _, agent := range s.connectedLocked() {
		if agent.ID != caller.ID && agent.Project == caller.Project {
			peers++
		}
	}
	return peers
}

func (s *Server) setStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	var body struct {
		Task *string `json:"task"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Task == nil {
		fail(w, http.StatusBadRequest, "task must be present and a string")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	agent, found := s.reg.Find(s.agents, id)
	if !found {
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
		return
	}
	if agent.Task != *body.Task {
		agent.Task = *body.Task
		agent.TaskAt = time.Time{}
		if agent.Task != "" {
			agent.TaskAt = s.cfg.Now()
		}
	}
	updated, err := s.reg.Register(s.agents, agent, s.cfg.Now())
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.agents = updated
	write(w, http.StatusOK, model.StatusResponse{Task: agent.Task, TaskAt: agent.TaskAt})
}
