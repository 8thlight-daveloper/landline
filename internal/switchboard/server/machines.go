package server

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	machineregistry "github.com/8thlight-daveloper/landline/internal/machine/registry"
	"github.com/8thlight-daveloper/landline/internal/model"
)

const (
	machineScheme        = "Landline-Machine"
	maxRedeemBytes       = 4096
	inviteRetries        = 10
	machineRefusal       = "missing or wrong switchboard secret or machine signature"
	defaultInviteTimeout = 5 * time.Minute
	defaultWindow        = 5 * time.Second
)

var machineHeaderField = regexp.MustCompile(`(name|time|nonce|signature)=([^,\s]+)`)

type InviteRequest struct {
	Name string `json:"name"`
}

type InviteResponse struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type RedeemResponse struct {
	Name string `json:"name"`
}

type MachineRefusal struct {
	Error           string `json:"error"`
	Reason          string `json:"reason"`
	ClockDifference *int64 `json:"clock_difference,omitempty"`
}

const (
	reasonClock   = "clock"
	reasonRefused = "refused"
)

func refuseMachine(w http.ResponseWriter) {
	write(w, http.StatusUnauthorized, MachineRefusal{Error: machineRefusal, Reason: reasonRefused})
}

func refuseMachineClock(w http.ResponseWriter, skew *model.ClockSkewError) {
	write(w, http.StatusUnauthorized, MachineRefusal{Error: skew.Error(), Reason: reasonClock, ClockDifference: &skew.Seconds})
}

func machineSigned(header string) bool {
	return header == machineScheme || strings.HasPrefix(header, machineScheme+" ")
}

type MachineInfo struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	Created     string `json:"created"`
}

func fillRandom(buffer []byte) { rand.Read(buffer) }

func readMachines(path string) ([]model.Machine, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s cannot be read: %v", path, err)
	}
	var machines []model.Machine
	if err := json.Unmarshal(raw, &machines); err != nil {
		return nil, fmt.Errorf("%s is not a list of machines: %v", path, err)
	}
	if machines == nil {
		return nil, fmt.Errorf("%s is not a list of machines", path)
	}
	return machines, nil
}

func CarryOverMachines(current, former string) (string, error) {
	if sameFile(current, former) {
		return "", nil
	}
	_, currentErr := os.Stat(current)
	_, formerErr := os.Stat(former)
	switch {
	case errors.Is(formerErr, os.ErrNotExist):
		return "", nil
	case !errors.Is(currentErr, os.ErrNotExist):
		return fmt.Sprintf("%s is no longer read, because the switchboard reads %s. You may delete %s", former, current, former), nil
	}
	machines, err := readMachines(former)
	if err != nil {
		return "", err
	}
	if err := writeMachines(current, machines); err != nil {
		return "", fmt.Errorf("the machines of %s could not be copied to %s: %v", former, current, err)
	}
	return fmt.Sprintf("copied the machines of %s to %s, and now reads %s. %s is left as it was", former, current, current, former), nil
}

func sameFile(one, other string) bool {
	oneAbsolute, oneErr := filepath.Abs(one)
	otherAbsolute, otherErr := filepath.Abs(other)
	if oneErr == nil && otherErr == nil && oneAbsolute == otherAbsolute {
		return true
	}
	oneInfo, oneErr := os.Stat(one)
	otherInfo, otherErr := os.Stat(other)
	return oneErr == nil && otherErr == nil && os.SameFile(oneInfo, otherInfo)
}

func CheckLogIsItsOwnFile(logPath string, kept ...string) error {
	for _, path := range kept {
		if sameFile(logPath, path) {
			return fmt.Errorf("the log path %s names the same file as %s. Choose another log path", logPath, path)
		}
	}
	return nil
}

type DataDirectoryLock struct{ file *os.File }

func LockDataDirectory(path string) (DataDirectoryLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return DataDirectoryLock{}, err
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return DataDirectoryLock{}, fmt.Errorf("%s cannot be opened: %v", path, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return DataDirectoryLock{}, fmt.Errorf("another switchboard is running on this machine, because it holds %s", path)
	}
	return DataDirectoryLock{file}, nil
}

func (lock DataDirectoryLock) Release() { lock.file.Close() }

func writeMachines(path string, machines []model.Machine) error {
	stored := make([]model.Machine, len(machines))
	for i, machine := range machines {
		stored[i] = model.Machine{Name: machine.Name, PublicKey: machine.PublicKey, Created: machine.Created.UTC()}
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.saving")
	if err != nil {
		return err
	}
	_, writeErr := temporary.Write(append(raw, '\n'))
	closeErr := temporary.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		os.Remove(temporary.Name())
		return err
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		os.Remove(temporary.Name())
		return err
	}
	return nil
}

func (s *Server) saveMachinesLocked(state model.Machines) error {
	if s.cfg.MachinesPath == "" {
		return nil
	}
	return writeMachines(s.cfg.MachinesPath, state.Active)
}

type verifiedMachine struct {
	name string
	key  []byte
}

func (machine verifiedMachine) signed() bool { return machine.name != "" }

func (s *Server) secretOrMachine(w http.ResponseWriter, r *http.Request) (verifiedMachine, bool) {
	if s.isSecret(r) {
		return verifiedMachine{}, true
	}
	if header := r.Header.Get("Authorization"); machineSigned(header) {
		return s.verifyMachine(w, r, header)
	}
	return verifiedMachine{}, s.secretOnly(w, r)
}

func (s *Server) verifyMachine(w http.ResponseWriter, r *http.Request, header string) (verifiedMachine, bool) {
	fields := map[string]string{}
	for _, found := range machineHeaderField.FindAllStringSubmatch(strings.TrimPrefix(header, machineScheme), -1) {
		fields[found[1]] = found[2]
	}
	unix, timeErr := strconv.ParseInt(fields["time"], 10, 64)
	signature, signatureErr := base64.StdEncoding.DecodeString(fields["signature"])
	body, bodyErr := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	nonce, nonceErr := base64.StdEncoding.DecodeString(fields["nonce"])
	if bodyErr != nil || timeErr != nil || signatureErr != nil || nonceErr != nil || len(nonce) != 16 || fields["name"] == "" {
		refuseMachine(w)
		return verifiedMachine{}, false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	request := model.SignedRequest{Name: fields["name"], Time: unix, Nonce: fields["nonce"], Signature: signature, Method: r.Method, Path: r.URL.RequestURI(), Body: body}
	s.mu.Lock()
	next, name, err := s.machines.Verify(s.state, request, s.cfg.Now(), s.started, s.cfg.SignatureWindow)
	var verified verifiedMachine
	if err == nil {
		s.state = next
		verified = verifiedMachine{name: name, key: s.activeKeyLocked(name)}
	}
	s.mu.Unlock()
	var skew *model.ClockSkewError
	switch {
	case err == nil:
		return verified, true
	case errors.As(err, &skew):
		refuseMachineClock(w, skew)
	default:
		refuseMachine(w)
	}
	return verifiedMachine{}, false
}

func (s *Server) invite(w http.ResponseWriter, r *http.Request) {
	if !s.secretOnly(w, r) {
		return
	}
	var body InviteRequest
	if !decode(w, r, &body) {
		return
	}
	now := requestTime(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	for attempt := 0; attempt < inviteRetries; attempt++ {
		random := make([]byte, 16)
		s.cfg.NewRandom(random)
		next, code, err := s.machines.Invite(s.state, body.Name, random, now, s.cfg.InviteLifetime)
		switch {
		case err == nil:
			s.state = next
			write(w, http.StatusOK, InviteResponse{Name: body.Name, Code: code})
			return
		case errors.Is(err, model.ErrBadMachineName):
			fail(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, model.ErrMachineHeld):
			fail(w, http.StatusConflict, err.Error())
			return
		case errors.Is(err, model.ErrCodeInUse):
			continue
		default:
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	fail(w, http.StatusInternalServerError, "could not make an invite code")
}

func readRedeem(w http.ResponseWriter, r *http.Request) (string, []byte) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRedeemBytes))
	if err != nil {
		return "", nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return "", nil
	}
	var code string
	if json.Unmarshal(fields["code"], &code) != nil {
		code = ""
	}
	var encoded string
	var key []byte
	if json.Unmarshal(fields["public_key"], &encoded) == nil {
		if decoded, decodeErr := base64.StdEncoding.DecodeString(encoded); decodeErr == nil {
			key = decoded
		}
	}
	return code, key
}

func (s *Server) redeem(w http.ResponseWriter, r *http.Request) {
	code, key := readRedeem(w, r)
	s.mu.Lock()
	defer s.mu.Unlock()
	next, name, err := s.machines.Redeem(s.state, code, key, requestTime(r))
	switch {
	case errors.Is(err, model.ErrInvalidCode):
		s.state = next
		fail(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, model.ErrBadPublicKey):
		fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		fail(w, http.StatusInternalServerError, err.Error())
	default:
		if saveErr := s.saveMachinesLocked(next); saveErr != nil {
			fail(w, http.StatusInternalServerError, "the machine could not be saved: "+saveErr.Error())
			return
		}
		s.state = next
		write(w, http.StatusOK, RedeemResponse{Name: name})
	}
}

func (s *Server) listMachines(w http.ResponseWriter, r *http.Request) {
	if !s.secretOnly(w, r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []MachineInfo{}
	for _, machine := range s.state.Active {
		out = append(out, MachineInfo{Name: machine.Name, Fingerprint: s.machines.Fingerprint(machine.PublicKey), Created: machine.Created.UTC().Format("2006-01-02")})
	}
	write(w, http.StatusOK, out)
}

func (s *Server) revokeMachine(w http.ResponseWriter, r *http.Request) {
	if !s.secretOnly(w, r) {
		return
	}
	name := r.PathValue("name")
	s.mu.Lock()
	defer s.mu.Unlock()
	next, err := s.machines.Revoke(s.state, name)
	if errors.Is(err, model.ErrUnknownMachine) {
		fail(w, http.StatusNotFound, "no machine named "+name)
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if saveErr := s.saveMachinesLocked(next); saveErr != nil {
		fail(w, http.StatusInternalServerError, "the machine list could not be saved: "+saveErr.Error())
		return
	}
	s.state = next
	s.endMachineLocked(func(machine string) bool { return machine == name })
	write(w, http.StatusOK, struct{}{})
}

func (s *Server) revokeAllMachines(w http.ResponseWriter, r *http.Request) {
	if !s.secretOnly(w, r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.machines.RevokeAll(s.state)
	if saveErr := s.saveMachinesLocked(next); saveErr != nil {
		fail(w, http.StatusInternalServerError, "the machine list could not be saved: "+saveErr.Error())
		return
	}
	s.state = next
	s.endMachineLocked(func(string) bool { return true })
	write(w, http.StatusOK, struct{}{})
}

func (s *Server) endMachineLocked(matches func(string) bool) {
	for token, machine := range s.machineTokens {
		if !matches(machine) {
			continue
		}
		if id, isAgent := s.tokens[token]; isAgent {
			s.removeAgentLocked(id)
		} else {
			delete(s.observers, token)
		}
		delete(s.machineTokens, token)
	}
	for id, open := range s.streams {
		if open.machine != "" && matches(open.machine) {
			s.endStreamLocked(id, open)
		}
	}
}

func (s *Server) removeAgentLocked(id string) {
	kept := make([]model.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		if agent.ID != id {
			kept = append(kept, agent)
		}
	}
	s.agents = kept
	if open := s.streams[id]; open != nil {
		s.endStreamLocked(id, open)
	}
	s.forgetLocked(id)
}

func newMachineSettings(cfg *Config) {
	if cfg.InviteLifetime == 0 {
		cfg.InviteLifetime = defaultInviteTimeout
	}
	if cfg.SignatureWindow == 0 {
		cfg.SignatureWindow = defaultWindow
	}
	if cfg.NewRandom == nil {
		cfg.NewRandom = fillRandom
	}
}

func newMachineRegistry() machineregistry.Registry { return machineregistry.New() }

func (s *Server) activeKeyLocked(name string) []byte {
	i := slices.IndexFunc(s.state.Active, func(machine model.Machine) bool { return machine.Name == name })
	if i < 0 {
		return nil
	}
	return slices.Clone(s.state.Active[i].PublicKey)
}

func (s *Server) stillActiveLocked(machine verifiedMachine) bool {
	key := s.activeKeyLocked(machine.name)
	return key != nil && bytes.Equal(key, machine.key)
}
