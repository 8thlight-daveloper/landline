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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/8thlight-daveloper/landline/internal/agent/registry"
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
	Secret                string
	Host                  string
	KeepaliveInterval     time.Duration
	DeliveryTimeout       time.Duration
	DisconnectedRetention time.Duration
	ChallengeLimit        int
	ChallengeWindow       time.Duration
	Now                   func() time.Time
	NewID                 func() string
	NewToken              func() string
	NewSuffix             func() string
	NewCode               func() string
	Notify                func(string)
	StatePath             string
}

type Server struct {
	cfg     Config
	store   Store
	reg     registry.Registry
	rt      router.Router
	mu      sync.Mutex
	agents  []model.Agent
	tokens  map[string]string
	seq     int64
	attach  map[string]*attachChallenge
	streams map[string]*stream
	pending map[string]*pendingDelivery
	sent    map[string][]time.Time
}

type stream struct {
	out  chan model.Delivery
	done chan struct{}
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
		return nil, errors.New("the hub secret must be at least 16 characters")
	}
	if cfg.KeepaliveInterval == 0 {
		cfg.KeepaliveInterval = 15 * time.Second
	}
	if cfg.DeliveryTimeout == 0 {
		cfg.DeliveryTimeout = 20 * time.Second
	}
	if cfg.DisconnectedRetention == 0 {
		cfg.DisconnectedRetention = 10 * time.Minute
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
	if cfg.NewSuffix == nil {
		cfg.NewSuffix = func() string { return randomHex(2)[:3] }
	}
	if cfg.NewToken == nil {
		cfg.NewToken = func() string { return randomHex(24) }
	}
	if cfg.Notify == nil {
		cfg.Notify = func(string) {}
	}
	s := &Server{
		cfg: cfg, store: store,
		tokens: map[string]string{}, attach: map[string]*attachChallenge{},
		streams: map[string]*stream{}, pending: map[string]*pendingDelivery{},
		sent: map[string][]time.Time{},
		seq:  store.LastSeq(),
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
	mux.HandleFunc("GET /v1/stream", s.openStream)
	mux.HandleFunc("POST /v1/deliveries/{id}/report", s.report)
	mux.HandleFunc("POST /v1/attach", s.attachClaims)
	mux.HandleFunc("GET /v1/agents", s.list)
	mux.HandleFunc("GET /v1/log", s.logRead)
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
	if decoder.More() {
		fail(w, http.StatusBadRequest, "bad request: more than one JSON value")
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
	if (body.Name == "") == (body.Seed == "") {
		fail(w, http.StatusBadRequest, "give either a name or a seed, and not both")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	name := body.Name
	if body.Seed != "" {
		name = s.newHandle(body.Seed)
	}
	entry := model.Agent{
		Name: name, Harness: body.Harness, Host: body.Host, Session: body.Session,
		Folder: body.Folder, Branch: body.Branch, Worktree: body.Worktree, HarnessVersion: body.HarnessVersion,
		Model: body.Model, Effort: body.Effort, Label: body.Label,
	}
	updated, err := s.reg.Register(s.agents, entry, s.cfg.Now())
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.agents = updated
	for token, owner := range s.tokens {
		if owner == name {
			delete(s.tokens, token)
		}
	}
	token := s.cfg.NewToken()
	s.tokens[token] = name
	s.saveLocked()
	write(w, http.StatusOK, model.RegisterResponse{Name: name, Token: token})
}

var handleBase = regexp.MustCompile(`[^a-z0-9_-]+`)

func (s *Server) newHandle(seed string) string {
	base := strings.Trim(handleBase.ReplaceAllString(strings.ToLower(seed), "-"), "-_")
	if base == "" {
		base = "agent"
	}
	if len(base) > 24 {
		base = strings.TrimRight(base[:24], "-_")
	}
	for {
		handle := base + "-" + s.cfg.NewSuffix()
		if _, taken := s.reg.Find(s.agents, handle); !taken && handle != model.Broadcast {
			return handle
		}
	}
}

func (s *Server) listed(now time.Time) []model.Agent {
	out := []model.Agent{}
	for _, a := range s.agents {
		if s.streams[a.Name] != nil || now.Sub(a.LastSeen) <= s.cfg.DisconnectedRetention {
			out = append(out, a)
		}
	}
	return out
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
	if body.To != model.Broadcast {
		match, candidates := s.reg.Resolve(s.agents, s.listed(s.cfg.Now()), body.To)
		if len(candidates) > 1 {
			names := make([]string, len(candidates))
			for i, each := range candidates {
				names[i] = each.Name
			}
			s.mu.Unlock()
			fail(w, http.StatusConflict, "more than one agent matches "+body.To+": "+strings.Join(names, ", ")+". Use a handle")
			return
		}
		if match.Name != "" {
			body.To = match.Name
		}
	}
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

func (s *Server) deliver(ctx context.Context, name string, delivery model.Delivery) (int, string) {
	s.mu.Lock()
	open := s.streams[name]
	if open == nil {
		s.mu.Unlock()
		return http.StatusConflict, "not connected"
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
				s.mu.Unlock()
				return http.StatusConflict, "not connected"
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
		return http.StatusConflict, "not connected"
	case <-timer.C:
		return http.StatusGatewayTimeout, "no delivery report"
	case <-ctx.Done():
		return http.StatusGatewayTimeout, "no delivery report"
	}
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	name, ok := s.agentFor(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
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
	name, ok := s.agentFor(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
		return
	}
	mine := &stream{out: make(chan model.Delivery), done: make(chan struct{})}
	s.mu.Lock()
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
	delete(s.streams, name)
	close(closing.done)
	for id, waiting := range s.pending {
		if waiting.stream == closing {
			delete(s.pending, id)
			close(waiting.lost)
		}
	}
	if agent, found := s.reg.Find(s.agents, name); found {
		if updated, err := s.reg.Register(s.agents, agent, s.cfg.Now()); err == nil {
			s.agents = updated
		}
	}
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.agentFor(r); !ok {
		fail(w, http.StatusUnauthorized, "missing or unknown agent token")
		return
	}
	s.mu.Lock()
	listed := s.listed(s.cfg.Now())
	connected := map[string]bool{}
	for name := range s.streams {
		connected[name] = true
	}
	s.mu.Unlock()
	out := []model.AgentInfo{}
	for _, a := range listed {
		out = append(out, model.AgentInfo{
			Name: a.Name, Harness: a.Harness, Host: a.Host, Session: a.Session, Connected: connected[a.Name], LastSeen: a.LastSeen,
			Folder: a.Folder, Branch: a.Branch, Worktree: a.Worktree, HarnessVersion: a.HarnessVersion, Model: a.Model, Effort: a.Effort, Label: a.Label,
		})
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
