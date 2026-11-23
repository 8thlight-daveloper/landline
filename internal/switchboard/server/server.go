package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/8thlight-daveloper/landline/internal/agent/registry"
	machineregistry "github.com/8thlight-daveloper/landline/internal/machine/registry"
	"github.com/8thlight-daveloper/landline/internal/message/router"
	"github.com/8thlight-daveloper/landline/internal/model"
)

type Store interface {
	Append(model.Envelope) error
	All() []model.Envelope
	LastSeq() int64
	Result(seq int64, result string) error
}

type Config struct {
	Secret            string
	Host              string
	KeepaliveInterval time.Duration
	DeliveryTimeout   time.Duration
	Grace             time.Duration
	ChallengeLimit    int
	ChallengeWindow   time.Duration
	Now               func() time.Time
	NewID             func() string
	NewToken          func() string
	NewAgentID        func() string
	NewCode           func() string
	Notify            func(string)
	Build             BuildResponse
	InviteLifetime    time.Duration
	SignatureWindow   time.Duration
	MachinesPath      string
	NewRandom         func([]byte)
}

type Server struct {
	cfg           Config
	store         Store
	reg           registry.Registry
	rt            router.Router
	machines      machineregistry.Registry
	mu            sync.Mutex
	state         model.Machines
	started       time.Time
	machineTokens map[string]string
	agents        []model.Agent
	tokens        map[string]string
	observers     map[string]time.Time
	seq           int64
	attach        map[string]*attachChallenge
	streams       map[string]*stream
	pending       map[string]*pendingDelivery
	sent          map[string][]time.Time
}

type stream struct {
	out     chan model.Delivery
	done    chan struct{}
	machine string
}

type pendingDelivery struct {
	agent  string
	stream *stream
	report chan model.DeliveryReport
	lost   chan struct{}
}

const minSecretLength = 16

const maxRequestBytes = 6*model.MaxBodyBytes + 4096

func New(cfg Config, store Store) (*Server, error) {
	if len(cfg.Secret) < minSecretLength {
		return nil, errors.New("the switchboard secret must be at least 16 characters")
	}
	if cfg.KeepaliveInterval == 0 {
		cfg.KeepaliveInterval = 15 * time.Second
	}
	if cfg.DeliveryTimeout == 0 {
		cfg.DeliveryTimeout = 20 * time.Second
	}
	if cfg.Grace == 0 {
		cfg.Grace = 2 * time.Minute
	}
	if cfg.ChallengeLimit == 0 {
		cfg.ChallengeLimit = 5
	}
	if cfg.ChallengeWindow == 0 {
		cfg.ChallengeWindow = 5 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.NewID == nil {
		cfg.NewID = func() string { return "m-" + randomHex(8) }
	}
	if cfg.NewCode == nil {
		cfg.NewCode = func() string { return strings.ToUpper(randomHex(3))[:6] }
	}
	if cfg.NewAgentID == nil {
		cfg.NewAgentID = func() string { return randomHex(2) }
	}
	if cfg.NewToken == nil {
		cfg.NewToken = func() string { return randomHex(24) }
	}
	if cfg.Notify == nil {
		cfg.Notify = func(string) {}
	}
	newMachineSettings(&cfg)
	loaded, err := readMachines(cfg.MachinesPath)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg: cfg, store: store, machines: newMachineRegistry(), started: cfg.Now(), machineTokens: map[string]string{},
		state:  model.Machines{Active: loaded},
		tokens: map[string]string{}, observers: map[string]time.Time{}, attach: map[string]*attachChallenge{},
		streams: map[string]*stream{}, pending: map[string]*pendingDelivery{},
		sent: map[string][]time.Time{},
		seq:  store.LastSeq(),
	}
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
	mux.HandleFunc("POST /v1/observers", s.registerObserver)
	mux.HandleFunc("POST /v1/send", s.send)
	mux.HandleFunc("GET /v1/stream", s.openStream)
	mux.HandleFunc("POST /v1/deliveries/{id}/report", s.report)
	mux.HandleFunc("POST /v1/attach", s.attachClaims)
	mux.HandleFunc("POST /v1/status", s.setStatus)
	mux.HandleFunc("GET /v1/agents", s.list)
	mux.HandleFunc("GET /v1/log", s.logRead)
	mux.HandleFunc("GET /v1/build", s.build)
	mux.HandleFunc("POST /v1/machines", s.invite)
	mux.HandleFunc("PATCH /v1/machines", s.redeem)
	mux.HandleFunc("GET /v1/machines", s.listMachines)
	mux.HandleFunc("DELETE /v1/machines/{name}", s.revokeMachine)
	mux.HandleFunc("DELETE /v1/machines", s.revokeAllMachines)
	return s.expiring(mux)
}

func (s *Server) expiring(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := s.cfg.Now()
		s.mu.Lock()
		s.expireLocked(now)
		s.mu.Unlock()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestTimeKey{}, now)))
	})
}

type requestTimeKey struct{}

func requestTime(r *http.Request) time.Time {
	return r.Context().Value(requestTimeKey{}).(time.Time)
}

func (s *Server) expireLocked(now time.Time) {
	kept := make([]model.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		if s.streams[agent.ID] != nil || now.Sub(agent.LastSeen) <= s.cfg.Grace {
			kept = append(kept, agent)
			continue
		}
		s.forgetLocked(agent.ID)
	}
	s.agents = kept
	for token, seen := range s.observers {
		if now.Sub(seen) > s.cfg.Grace {
			delete(s.observers, token)
			delete(s.machineTokens, token)
		}
	}
}

func (s *Server) forgetLocked(name string) {
	for token, owner := range s.tokens {
		if owner == name {
			delete(s.tokens, token)
			delete(s.machineTokens, token)
		}
	}
	for id, waiting := range s.pending {
		if waiting.agent == name {
			delete(s.pending, id)
		}
	}
	delete(s.attach, name)
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

type caller int

const (
	unknownCaller caller = iota
	agentCaller
	observerCaller
)

func (s *Server) callerFor(r *http.Request) (string, caller) {
	token := bearer(r)
	if token == "" {
		return "", unknownCaller
	}
	now := requestTime(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	if _, ok := s.observers[token]; ok {
		s.observers[token] = now
		return "", observerCaller
	}
	name, ok := s.tokens[token]
	if !ok {
		return "", unknownCaller
	}
	if agent, found := s.reg.Find(s.agents, name); found {
		if updated, err := s.reg.Register(s.agents, agent, now); err == nil {
			s.agents = updated
		}
	}
	return name, agentCaller
}

func (s *Server) secretOnly(w http.ResponseWriter, r *http.Request) bool {
	if s.isSecret(r) {
		return true
	}
	if s.observerRefreshed(r) {
		fail(w, http.StatusForbidden, "an observer token is read-only and may only list agents")
	} else {
		fail(w, http.StatusUnauthorized, "missing or wrong switchboard secret")
	}
	return false
}

func (s *Server) observerRefreshed(r *http.Request) bool {
	token := bearer(r)
	if token == "" {
		return false
	}
	now := requestTime(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	if _, ok := s.observers[token]; !ok {
		return false
	}
	s.observers[token] = now
	return true
}

func (s *Server) agentFor(w http.ResponseWriter, r *http.Request) (string, bool) {
	name, who := s.callerFor(r)
	switch who {
	case agentCaller:
		return name, true
	case observerCaller:
		fail(w, http.StatusForbidden, "an observer token is read-only and may only list agents")
	default:
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
	}
	return "", false
}

func (s *Server) registerObserver(w http.ResponseWriter, r *http.Request) {
	machine, ok := s.secretOrMachine(w, r)
	if !ok {
		return
	}
	var body ObserverRequest
	if !decode(w, r, &body) {
		return
	}
	if !plainKind(body.Kind) {
		fail(w, http.StatusBadRequest, "kind must be 1 to 64 bytes of plain text")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if machine != "" && !s.machineActiveLocked(machine) {
		refuseMachine(w)
		return
	}
	now := requestTime(r)
	s.expireLocked(now)
	token := s.cfg.NewToken()
	s.observers[token] = now
	if machine != "" {
		s.machineTokens[token] = machine
	}
	write(w, http.StatusOK, ObserverResponse{Token: token})
}

func plainKind(kind string) bool {
	if len(kind) > 64 || strings.TrimSpace(kind) == "" {
		return false
	}
	for _, r := range kind {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf) {
			return false
		}
	}
	return true
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
	if decoder.More() {
		fail(w, http.StatusBadRequest, "bad request: more than one JSON value")
		return false
	}
	return true
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	machine, ok := s.secretOrMachine(w, r)
	if !ok {
		return
	}
	var body model.RegisterRequest
	if !decode(w, r, &body) {
		return
	}
	if (body.ID == "") == (body.Seed == "") {
		fail(w, http.StatusBadRequest, "give either an id or a seed, and not both")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if machine != "" && !s.machineActiveLocked(machine) {
		refuseMachine(w)
		return
	}
	now := requestTime(r)
	s.expireLocked(now)
	id := body.ID
	if body.Seed != "" {
		id = s.newID()
	}
	entry := model.Agent{
		ID: id, Harness: body.Harness, Host: body.Host, Session: body.Session,
		Folder: body.Folder, Branch: body.Branch, Worktree: body.Worktree, HarnessVersion: body.HarnessVersion, LandlineVersion: body.LandlineVersion,
		LandlineCommit: body.LandlineCommit, LandlineDate: body.LandlineDate,
		Model: body.Model, Effort: body.Effort, Project: body.Project,
	}
	updated, err := s.reg.Register(s.agents, entry, now)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.agents = updated
	for token, owner := range s.tokens {
		if owner == id {
			delete(s.tokens, token)
			delete(s.machineTokens, token)
		}
	}
	token := s.cfg.NewToken()
	s.tokens[token] = id
	if machine != "" {
		s.machineTokens[token] = machine
	}
	write(w, http.StatusOK, model.RegisterResponse{ID: id, Token: token})
}

func (s *Server) newID() string {
	for {
		id := s.cfg.NewAgentID()
		if _, taken := s.reg.Find(s.agents, id); !taken {
			return id
		}
	}
}

func (s *Server) connectedLocked() []model.Agent {
	out := []model.Agent{}
	for _, a := range s.agents {
		if s.streams[a.ID] != nil {
			out = append(out, a)
		}
	}
	return out
}

const hintLimit = 10

func (s *Server) projectHintLocked(value, sender string) string {
	holders := []string{}
	for _, agent := range s.connectedLocked() {
		if agent.Project == value && agent.ID != sender {
			holders = append(holders, agent.ID)
		}
	}
	if len(holders) == 0 {
		return ""
	}
	slices.Sort(holders)
	listed := strings.Join(holders[:min(len(holders), hintLimit)], ", ")
	if len(holders) > hintLimit {
		listed += ", and more"
	}
	return value + " is a project, not an agent. Send to one of: " + listed
}

func (s *Server) send(w http.ResponseWriter, r *http.Request) {
	from, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	var body model.SendBody
	if !decode(w, r, &body) {
		return
	}
	now := requestTime(r)
	s.mu.Lock()
	s.expireLocked(now)
	decision, err := s.rt.Route(s.agents, from, model.SendRequest(body), s.cfg.NewID(), s.seq+1, now)
	if err != nil {
		problem := err.Error()
		if errors.Is(err, model.ErrUnknownRecipient) && body.To != model.Broadcast {
			if hint := s.projectHintLocked(body.To, from); hint != "" {
				problem += ". " + hint
			}
		}
		s.mu.Unlock()
		fail(w, statusOf(err), problem)
		return
	}
	if err := s.store.Append(decision.Envelope); err != nil {
		s.mu.Unlock()
		fail(w, http.StatusInternalServerError, "could not record the message")
		return
	}
	s.seq = decision.Envelope.Seq
	sender, _ := s.reg.Find(s.agents, from)
	s.mu.Unlock()

	envelope := decision.Envelope
	text := model.Render(envelope, sender)
	wire := model.WireEnvelope{
		Seq: envelope.Seq, ID: envelope.ID, Time: envelope.Time, Thread: envelope.Thread, From: envelope.From,
		FromInfo: fmt.Sprintf("%s on %s", sender.Harness, sender.Host),
		To:       envelope.To, Kind: envelope.Kind, Body: envelope.Body, InReplyTo: envelope.InReplyTo, Text: text,
	}
	recipient := decision.Recipients[0]
	status, problem := s.deliver(r.Context(), recipient, model.Delivery{Kind: model.DeliveryMessage, Text: text, Message: &wire})
	result := "delivered"
	if status != http.StatusOK {
		result = problem
	}
	if err := s.store.Result(envelope.Seq, result); err != nil {
		s.cfg.Notify(fmt.Sprintf("could not record the result of message %s: %v", envelope.ID, err))
	}
	if status != http.StatusOK {
		s.cfg.Notify(fmt.Sprintf("delivery to %s of a message from %s, id %s, failed: %s", recipient, envelope.From, envelope.ID, problem))
		fail(w, status, problem)
		return
	}
	write(w, http.StatusOK, model.SendResponse{ID: envelope.ID, Seq: envelope.Seq, Recipients: decision.Recipients})
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

const maxReportedProblemBytes = 256

func plainProblem(problem string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp) {
			return ' '
		}
		return r
	}, problem)
	if len(cleaned) <= maxReportedProblemBytes {
		return cleaned
	}
	cut := maxReportedProblemBytes
	for cut > 0 && !utf8.RuneStart(cleaned[cut]) {
		cut--
	}
	return cleaned[:cut]
}

const notConnected = "not connected"

const unversionedHint = ": no landline version is on record, so it is a hand-registered id, which can send and cannot receive, or an older landline. Restart its session if it is a landline session"

func (s *Server) notConnectedLocked(name string) string {
	if agent, found := s.reg.Find(s.agents, name); found && agent.LandlineVersion == "" {
		return notConnected + unversionedHint
	}
	return notConnected
}

func (s *Server) deliver(ctx context.Context, name string, delivery model.Delivery) (int, string) {
	s.mu.Lock()
	open := s.streams[name]
	if open == nil {
		problem := s.notConnectedLocked(name)
		s.mu.Unlock()
		return http.StatusConflict, problem
	}
	delivery.ID = "d-" + randomHex(8)
	waiting := &pendingDelivery{agent: name, stream: open, report: make(chan model.DeliveryReport, 1), lost: make(chan struct{})}
	s.pending[delivery.ID] = waiting
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, delivery.ID)
		s.mu.Unlock()
	}()
	timer := time.NewTimer(s.cfg.DeliveryTimeout)
	defer timer.Stop()
	retried := false
	for handed := false; !handed; {
		select {
		case open.out <- delivery:
			handed = true
		case <-open.done:
			s.mu.Lock()
			current := s.streams[name]
			if current == nil || current == open || retried {
				problem := s.notConnectedLocked(name)
				s.mu.Unlock()
				return http.StatusConflict, problem
			}
			waiting.stream = current
			s.mu.Unlock()
			open, retried = current, true
		case <-timer.C:
			return http.StatusGatewayTimeout, "no delivery report"
		case <-ctx.Done():
			return http.StatusGatewayTimeout, "no delivery report"
		}
	}
	select {
	case report := <-waiting.report:
		if report.OK {
			return http.StatusOK, ""
		}
		return http.StatusBadGateway, plainProblem(report.Error)
	case <-waiting.lost:
		s.mu.Lock()
		problem := s.notConnectedLocked(name)
		s.mu.Unlock()
		return http.StatusConflict, problem
	case <-timer.C:
		return http.StatusGatewayTimeout, "no delivery report"
	case <-ctx.Done():
		return http.StatusGatewayTimeout, "no delivery report"
	}
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	name, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	var report model.DeliveryReport
	if !decode(w, r, &report) {
		return
	}
	s.mu.Lock()
	waiting, found := s.pending[r.PathValue("id")]
	if !found {
		s.mu.Unlock()
		fail(w, http.StatusNotFound, "no such delivery")
		return
	}
	if waiting.agent != name {
		s.mu.Unlock()
		fail(w, http.StatusForbidden, "that delivery went to another agent")
		return
	}
	delete(s.pending, r.PathValue("id"))
	s.mu.Unlock()
	waiting.report <- report
	write(w, http.StatusOK, struct{}{})
}

func (s *Server) openStream(w http.ResponseWriter, r *http.Request) {
	name, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	token := bearer(r)
	s.mu.Lock()
	if s.tokens[token] != name {
		s.mu.Unlock()
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
		return
	}
	mine := &stream{out: make(chan model.Delivery), done: make(chan struct{}), machine: s.machineTokens[token]}
	if previous := s.streams[name]; previous != nil {
		close(previous.done)
	}
	s.streams[name] = mine
	s.mu.Unlock()
	defer s.closeStream(name, mine)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(w)
	emit := func(text string) bool {
		if err := controller.SetWriteDeadline(time.Now().Add(2 * s.cfg.KeepaliveInterval)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return false
		}
		if _, err := io.WriteString(w, text); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	if !emit("") {
		return
	}
	ticker := time.NewTicker(s.cfg.KeepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case delivery := <-mine.out:
			raw, err := json.Marshal(delivery)
			if err != nil || !emit("event: delivery\ndata: "+string(raw)+"\n\n") {
				return
			}
			ticker.Reset(s.cfg.KeepaliveInterval)
		case <-ticker.C:
			if !emit(": keepalive\n\n") {
				return
			}
		case <-mine.done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) closeStream(name string, closing *stream) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streams[name] != closing {
		return
	}
	s.endStreamLocked(name, closing)
	if agent, found := s.reg.Find(s.agents, name); found {
		if updated, err := s.reg.Register(s.agents, agent, s.cfg.Now()); err == nil {
			s.agents = updated
		}
	}
}

func (s *Server) endStreamLocked(name string, closing *stream) {
	delete(s.streams, name)
	close(closing.done)
	for id, waiting := range s.pending {
		if waiting.stream == closing {
			delete(s.pending, id)
			close(waiting.lost)
		}
	}
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	if _, who := s.callerFor(r); who == unknownCaller {
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
		return
	}
	s.mu.Lock()
	s.expireLocked(requestTime(r))
	listed := append([]model.Agent{}, s.agents...)
	connected := map[string]bool{}
	for name := range s.streams {
		connected[name] = true
	}
	s.mu.Unlock()
	out := []model.AgentInfo{}
	for _, a := range listed {
		out = append(out, model.AgentInfo{
			ID: a.ID, Harness: a.Harness, Host: a.Host, Session: a.Session, Connected: connected[a.ID], LastSeen: a.LastSeen,
			Folder: a.Folder, Branch: a.Branch, Worktree: a.Worktree, HarnessVersion: a.HarnessVersion, LandlineVersion: a.LandlineVersion,
			LandlineCommit: a.LandlineCommit, LandlineDate: a.LandlineDate, Model: a.Model, Effort: a.Effort, Project: a.Project, Task: a.Task, TaskAt: a.TaskAt,
		})
	}
	write(w, http.StatusOK, out)
}

func (s *Server) logRead(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.secretOrMachine(w, r); !ok {
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
