package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/8thlight-daveloper/landline/internal/agent/registry"
	"github.com/8thlight-daveloper/landline/internal/message/inbox"
	"github.com/8thlight-daveloper/landline/internal/message/router"
	"github.com/8thlight-daveloper/landline/internal/model"
)

type Store interface {
	Append(model.Envelope) error
	All() []model.Envelope
	LastSeq() int64
}

type Waker interface {
	Wake(target, text string) error
}

type Config struct {
	Secret     string
	Host       string
	Window     time.Duration
	IdleWindow time.Duration
	Now        func() time.Time
	NewID      func() string
	NewToken   func() string
	Wakers     map[string]Waker
	Notify     func(string)
	StatePath  string
}

type Server struct {
	cfg     Config
	store   Store
	reg     registry.Registry
	rt      router.Router
	in      inbox.Inbox
	mu      sync.Mutex
	agents  []model.Agent
	tokens  map[string]string
	cursors map[string]int64
	waiting map[string]int
	seq     int64
	changed chan struct{}
	wakes   map[string][]*queuedWake
}

type queuedWake struct {
	job        model.WakeJob
	leaseUntil time.Time
}

const wakeLease = 60 * time.Second

var kindPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

const maxQueuedWakes = 200

const maxPageBytes = 2 << 20

const maxRequestBytes = 6*model.MaxBodyBytes + 4096

func New(cfg Config, store Store) (*Server, error) {
	if len(cfg.Secret) < 16 {
		return nil, errors.New("the hub secret must be at least 16 characters")
	}
	if cfg.Window == 0 {
		cfg.Window = 90 * time.Second
	}
	if cfg.IdleWindow == 0 {
		cfg.IdleWindow = 24 * time.Hour
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.NewID == nil {
		cfg.NewID = func() string { return "m-" + randomHex(8) }
	}
	if cfg.NewToken == nil {
		cfg.NewToken = func() string { return randomHex(24) }
	}
	if cfg.Notify == nil {
		cfg.Notify = func(string) {}
	}
	s := &Server{
		cfg: cfg, store: store,
		tokens: map[string]string{}, cursors: map[string]int64{}, waiting: map[string]int{}, wakes: map[string][]*queuedWake{},
		seq: store.LastSeq(), changed: make(chan struct{}),
	}
	s.loadState()
	return s, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/register", s.register)
	mux.HandleFunc("POST /v1/send", s.send)
	mux.HandleFunc("GET /v1/inbox", s.inbox)
	mux.HandleFunc("GET /v1/agents", s.list)
	mux.HandleFunc("GET /v1/log", s.logRead)
	mux.HandleFunc("GET /v1/wakes", s.wakePoll)
	mux.HandleFunc("POST /v1/wakes/{id}/ack", s.wakeAck)
	return mux
}

func bearer(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if len(value) > 7 && strings.EqualFold(value[:7], "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}

func (s *Server) isSecret(r *http.Request) bool {
	return subtle.ConstantTimeCompare([]byte(bearer(r)), []byte(s.cfg.Secret)) == 1
}

func (s *Server) agentFor(r *http.Request) (string, bool) {
	token := bearer(r)
	if token == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	name, ok := s.tokens[token]
	if !ok {
		return "", false
	}
	if agent, found := s.reg.Find(s.agents, name); found {
		if updated, err := s.reg.Register(s.agents, agent, s.cfg.Now()); err == nil {
			s.agents = updated
		}
	}
	return name, true
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, status int, message string) {
	write(w, status, model.ErrorResponse{Error: message})
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		fail(w, http.StatusBadRequest, "bad request: "+err.Error())
		return false
	}
	return true
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !s.isSecret(r) {
		fail(w, http.StatusUnauthorized, "missing or wrong hub secret")
		return
	}
	var body model.RegisterRequest
	if !decode(w, r, &body) {
		return
	}
	if body.WakeKind != "" && !kindPattern.MatchString(body.WakeKind) {
		fail(w, http.StatusBadRequest, "a wake method is 1 to 32 lowercase letters, digits, or -")
		return
	}
	if body.WakeKind != "" && s.localHost(body.Host) && s.cfg.Wakers[body.WakeKind] == nil {
		fail(w, http.StatusBadRequest, "this hub has no wake method "+body.WakeKind)
		return
	}
	if body.WakeKind != "" && strings.TrimSpace(body.WakeTarget) == "" {
		fail(w, http.StatusBadRequest, "a wake method needs a wake_target")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := model.Agent{Name: body.Name, Harness: body.Harness, Host: body.Host, Session: body.Session, WakeKind: body.WakeKind, WakeTarget: body.WakeTarget}
	updated, err := s.reg.Register(s.agents, entry, s.cfg.Now())
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.agents = updated
	for token, name := range s.tokens {
		if name == body.Name {
			delete(s.tokens, token)
		}
	}
	token := s.cfg.NewToken()
	s.tokens[token] = body.Name
	if _, seen := s.cursors[body.Name]; !seen {
		s.cursors[body.Name] = s.store.LastSeq()
	}
	s.saveLocked()
	write(w, http.StatusOK, model.RegisterResponse{Name: body.Name, Token: token})
}

func (s *Server) send(w http.ResponseWriter, r *http.Request) {
	from, ok := s.agentFor(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
		return
	}
	var body model.SendBody
	if !decode(w, r, &body) {
		return
	}
	s.mu.Lock()
	decision, err := s.rt.Route(s.agents, from, model.SendRequest(body), s.cfg.NewID(), s.seq+1, s.cfg.Now())
	if err != nil {
		s.mu.Unlock()
		fail(w, statusOf(err), err.Error())
		return
	}
	if err := s.store.Append(decision.Envelope); err != nil {
		s.mu.Unlock()
		fail(w, http.StatusInternalServerError, "could not record the message")
		return
	}
	s.seq = decision.Envelope.Seq
	sender, _ := s.reg.Find(s.agents, from)
	targets, queued := s.wakeTargets(decision.Envelope, sender, decision.Recipients)
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()

	woken := s.wake(decision.Envelope, sender, targets)
	write(w, http.StatusOK, model.SendResponse{ID: decision.Envelope.ID, Seq: decision.Envelope.Seq, Recipients: decision.Recipients, Woken: woken, Queued: queued})
}

func statusOf(err error) int {
	switch {
	case errors.Is(err, model.ErrUnknownRecipient):
		return http.StatusNotFound
	case errors.Is(err, model.ErrBodyTooLarge):
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusUnprocessableEntity
	}
}

func (s *Server) wakeTargets(e model.Envelope, sender model.Agent, recipients []string) ([]model.Agent, []string) {
	out := []model.Agent{}
	queued := []string{}
	for _, name := range recipients {
		agent, found := s.reg.Find(s.agents, name)
		if !found || agent.WakeKind == "" || s.waiting[name] != 0 {
			continue
		}
		if s.localTo(agent) {
			out = append(out, agent)
			continue
		}
		s.wakes[agent.Host] = append(s.wakes[agent.Host], &queuedWake{job: model.WakeJob{
			ID: e.ID + "-" + agent.Name, From: e.From, Agent: agent.Name, Kind: agent.WakeKind, Target: agent.WakeTarget, Text: model.Render(e, sender),
		}})
		queued = append(queued, agent.Name)
		if len(s.wakes[agent.Host]) > maxQueuedWakes {
			dropped := s.wakes[agent.Host][0].job
			s.wakes[agent.Host] = s.wakes[agent.Host][1:]
			s.cfg.Notify(fmt.Sprintf("the wake queue for host %s is full, so the oldest wake, for %s, was dropped. The message is still in its inbox", agent.Host, dropped.Agent))
		}
	}
	return out, queued
}

func (s *Server) localHost(host string) bool {
	return s.cfg.Host == "" || host == s.cfg.Host
}

func (s *Server) localTo(agent model.Agent) bool {
	return s.localHost(agent.Host)
}

func (s *Server) wake(e model.Envelope, sender model.Agent, targets []model.Agent) []string {
	text := model.Render(e, sender)
	results := make([]bool, len(targets))
	var group sync.WaitGroup
	for i, target := range targets {
		group.Add(1)
		go func() {
			defer group.Done()
			waker := s.cfg.Wakers[target.WakeKind]
			if waker == nil {
				s.cfg.Notify(fmt.Sprintf("wake of %s failed: this hub has no wake method %s", target.Name, target.WakeKind))
				return
			}
			if err := waker.Wake(target.WakeTarget, text); err != nil {
				s.cfg.Notify(fmt.Sprintf("wake of %s failed: %v", target.Name, err))
				return
			}
			s.cfg.Notify(fmt.Sprintf("woke %s (%s) for a message from %s, id %s", target.Name, target.WakeKind, e.From, e.ID))
			results[i] = true
		}()
	}
	group.Wait()
	woken := []string{}
	for i, target := range targets {
		if results[i] {
			woken = append(woken, target.Name)
		}
	}
	return woken
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	name, ok := s.agentFor(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
		return
	}
	query := r.URL.Query()
	if query.Get("peek") != "" {
		s.mu.Lock()
		cursor := s.cursors[name]
		unread := s.in.Select(s.store.All(), name, cursor, nil, 0)
		s.mu.Unlock()
		write(w, http.StatusOK, model.InboxResponse{Messages: []model.WireEnvelope{}, Next: cursor, Remaining: len(unread.Messages), Head: s.store.LastSeq()})
		return
	}
	wait := clamp(intOf(query.Get("wait")), 0, 60)
	limit := clamp(intOf(query.Get("limit")), 0, 200)
	if limit == 0 {
		limit = 50
	}
	var threads []string
	if raw := query.Get("thread"); raw != "" {
		threads = strings.Split(raw, ",")
	}
	deadline := time.NewTimer(time.Duration(wait) * time.Second)
	defer deadline.Stop()
	startWaiting := func() {
		if len(threads) == 0 {
			s.mu.Lock()
			s.waiting[name]++
			s.mu.Unlock()
		}
	}
	stopWaiting := func() {
		if len(threads) == 0 {
			s.mu.Lock()
			s.waiting[name]--
			s.mu.Unlock()
		}
	}
	for {
		s.mu.Lock()
		cursor := s.cursors[name]
		if len(threads) > 0 {
			cursor = int64(intOf(query.Get("after")))
		}
		page := capPage(s.in.Select(s.store.All(), name, cursor, threads, limit))
		changed := s.changed
		if len(page.Messages) > 0 && len(threads) == 0 {
			s.cursors[name] = page.Next
			s.saveLocked()
		}
		senders := map[string]model.Agent{}
		for _, each := range page.Messages {
			if a, found := s.reg.Find(s.agents, each.From); found {
				senders[each.From] = a
			}
		}
		s.mu.Unlock()
		if len(page.Messages) > 0 || wait == 0 {
			write(w, http.StatusOK, s.withHead(toWire(page, senders)))
			return
		}
		startWaiting()
		select {
		case <-changed:
			stopWaiting()
		case <-deadline.C:
			stopWaiting()
			write(w, http.StatusOK, s.withHead(toWire(page, senders)))
			return
		case <-r.Context().Done():
			stopWaiting()
			return
		}
	}
}

func (s *Server) withHead(out model.InboxResponse) model.InboxResponse {
	out.Head = s.store.LastSeq()
	return out
}

func capPage(page model.Page) model.Page {
	total := 0
	for i, each := range page.Messages {
		total += 2*len(each.Body) + 512
		if total > maxPageBytes && i > 0 {
			page.Remaining += len(page.Messages) - i
			page.Messages = page.Messages[:i]
			page.Next = page.Messages[i-1].Seq
			return page
		}
	}
	return page
}

func toWire(page model.Page, senders map[string]model.Agent) model.InboxResponse {
	out := model.InboxResponse{Messages: []model.WireEnvelope{}, Next: page.Next, Remaining: page.Remaining}
	for _, e := range page.Messages {
		sender := senders[e.From]
		out.Messages = append(out.Messages, model.WireEnvelope{
			Seq: e.Seq, ID: e.ID, Time: e.Time, Thread: e.Thread, From: e.From,
			FromInfo: fmt.Sprintf("%s on %s", sender.Harness, sender.Host),
			To:       e.To, Kind: e.Kind, Body: e.Body, InReplyTo: e.InReplyTo, Text: model.Render(e, sender),
		})
	}
	return out
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.agentFor(r); !ok {
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
		return
	}
	s.mu.Lock()
	online := map[string]bool{}
	for _, a := range s.reg.Online(s.agents, s.cfg.Now(), s.cfg.Window) {
		online[a.Name] = true
	}
	listed := append([]model.Agent(nil), s.agents...)
	now := s.cfg.Now()
	s.mu.Unlock()
	out := []model.AgentInfo{}
	for _, a := range listed {
		if online[a.Name] || a.WakeKind != "" || now.Sub(a.LastSeen) <= s.cfg.IdleWindow {
			out = append(out, model.AgentInfo{Name: a.Name, Harness: a.Harness, Host: a.Host, Session: a.Session, WakeKind: a.WakeKind, Online: online[a.Name], LastSeen: a.LastSeen})
		}
	}
	write(w, http.StatusOK, out)
}

func (s *Server) logRead(w http.ResponseWriter, r *http.Request) {
	if !s.isSecret(r) {
		fail(w, http.StatusUnauthorized, "missing or wrong hub secret")
		return
	}
	after := int64(intOf(r.URL.Query().Get("after")))
	out := []model.Envelope{}
	for _, e := range s.store.All() {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	write(w, http.StatusOK, out)
}

func intOf(raw string) int {
	n, _ := strconv.Atoi(raw)
	return n
}

func clamp(n, lo, hi int) int {
	return max(lo, min(n, hi))
}

func (s *Server) wakePoll(w http.ResponseWriter, r *http.Request) {
	if !s.isSecret(r) {
		fail(w, http.StatusUnauthorized, "missing or wrong hub secret")
		return
	}
	query := r.URL.Query()
	host := query.Get("host")
	if host == "" {
		fail(w, http.StatusBadRequest, "a wake poll needs a host")
		return
	}
	wait := clamp(intOf(query.Get("wait")), 0, 60)
	deadline := time.NewTimer(time.Duration(wait) * time.Second)
	defer deadline.Stop()
	for {
		s.mu.Lock()
		jobs := s.leaseWakes(host)
		changed := s.changed
		s.mu.Unlock()
		if len(jobs) > 0 || wait == 0 {
			write(w, http.StatusOK, jobs)
			return
		}
		select {
		case <-changed:
		case <-deadline.C:
			write(w, http.StatusOK, jobs)
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) leaseWakes(host string) []model.WakeJob {
	now := s.cfg.Now()
	jobs := []model.WakeJob{}
	for _, queued := range s.wakes[host] {
		if now.Before(queued.leaseUntil) {
			continue
		}
		queued.leaseUntil = now.Add(wakeLease)
		jobs = append(jobs, queued.job)
	}
	return jobs
}

func (s *Server) wakeAck(w http.ResponseWriter, r *http.Request) {
	if !s.isSecret(r) {
		fail(w, http.StatusUnauthorized, "missing or wrong hub secret")
		return
	}
	var ack model.WakeAck
	if !decode(w, r, &ack) {
		return
	}
	job, found := s.removeWake(r.PathValue("id"))
	if !found {
		fail(w, http.StatusNotFound, "no such wake")
		return
	}
	if ack.OK {
		s.cfg.Notify(fmt.Sprintf("host daemon woke %s (%s) for a message from %s, wake %s", job.Agent, job.Kind, job.From, job.ID))
	} else {
		s.cfg.Notify(fmt.Sprintf("wake of %s failed on its host: %s", job.Agent, ack.Error))
	}
	write(w, http.StatusOK, struct{}{})
}

func (s *Server) removeWake(id string) (model.WakeJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for host, queue := range s.wakes {
		for i, queued := range queue {
			if queued.job.ID == id {
				s.wakes[host] = append(queue[:i:i], queue[i+1:]...)
				return queued.job, true
			}
		}
	}
	return model.WakeJob{}, false
}
