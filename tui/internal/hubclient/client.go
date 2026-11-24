package hubclient

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/8thlight-daveloper/landline/tui/internal/clean"
)

const (
	requestTimeout   = 5 * time.Second
	maxResponseBytes = 8 << 20
)

var ErrControlSecret = errors.New("the switchboard secret contains a control character")

var ErrNoSecret = errors.New("there is no switchboard credential. Run landline server join <host:port> --code <code>, or set LANDLINE_SECRET")

type Client struct {
	address string
	secret  string
	http    *http.Client
	mu      sync.Mutex
	token   string
	reread  func() string

	machine          string
	key              ed25519.PrivateKey
	rereadCredential func() (string, Credential)
}

func New(address, secret string) *Client {
	return newWithTimeout(address, secret, requestTimeout)
}

func newWithTimeout(address, secret string, timeout time.Duration) *Client {
	return &Client{address: address, secret: secret, http: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) Address() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.address
}

func NewWith(address string, credential Credential) *Client {
	client := New(address, credential.Secret)
	client.machine, client.key = credential.Machine, credential.Key
	return client
}

func (c *Client) RereadCredentialWith(reread func() (string, Credential)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rereadCredential = reread
}

func (c *Client) currentCredential() Credential {
	c.mu.Lock()
	reread := c.rereadCredential
	c.mu.Unlock()
	if reread != nil {
		if address, fresh := reread(); address != "" && !fresh.empty() {
			c.mu.Lock()
			if address != c.address {
				c.address, c.token = address, ""
			}
			c.secret, c.machine, c.key = fresh.Secret, fresh.Machine, fresh.Key
			c.mu.Unlock()
		}
	} else {
		c.currentSecret()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return Credential{Secret: c.secret, Machine: c.machine, Key: c.key}
}

func (c *Client) RereadSecretWith(reread func() string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reread = reread
}

func (c *Client) currentSecret() string {
	c.mu.Lock()
	reread := c.reread
	c.mu.Unlock()
	if reread != nil {
		if fresh := reread(); fresh != "" {
			c.mu.Lock()
			c.secret = fresh
			c.mu.Unlock()
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.secret
}

func (c *Client) Agents() ([]Agent, error) {
	token, err := c.currentToken()
	if err != nil {
		return nil, err
	}
	agents, status, err := c.list(token)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		if token, err = c.register(); err != nil {
			return nil, err
		}
		if agents, status, err = c.list(token); err != nil {
			return nil, err
		}
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("the switchboard answered %d to the agent list", status)
	}
	return agents, nil
}

func (c *Client) currentToken() (string, error) {
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()
	if token != "" {
		return token, nil
	}
	return c.register()
}

const (
	refusalRetryInterval = time.Second
	refusalRetryPeriod   = 6 * time.Second
)

func (c *Client) register() (string, error) {
	token, keyRefused, err := c.registerOnce()
	if !keyRefused {
		return token, err
	}
	lastRetry := time.Now().Add(refusalRetryPeriod)
	for keyRefused && time.Now().Before(lastRetry) {
		time.Sleep(refusalRetryInterval)
		token, keyRefused, err = c.registerOnce()
	}
	return token, err
}

func (c *Client) registerOnce() (string, bool, error) {
	credential := c.currentCredential()
	if credential.empty() {
		return "", false, ErrNoSecret
	}
	if credential.Key == nil && strings.ContainsFunc(credential.Secret, unicode.IsControl) {
		return "", false, ErrControlSecret
	}
	address := c.Address()
	body, _ := json.Marshal(map[string]string{"kind": "tui"})
	req, err := http.NewRequest(http.MethodPost, address+"/v1/observers", bytes.NewReader(body))
	if err != nil {
		return "", false, errors.New("the switchboard address is not valid")
	}
	if credential.Key != nil {
		req.Header.Set("Authorization", signedHeader(credential.Machine, credential.Key, http.MethodPost, "/v1/observers", body, time.Now()))
	} else {
		req.Header.Set("Authorization", "Bearer "+credential.Secret)
	}
	req.Header.Set("Content-Type", "application/json")
	raw, status, err := c.do(req)
	if err != nil {
		return "", false, err
	}
	switch {
	case status == http.StatusUnauthorized && credential.Key != nil:
		refusal := machineRefusalIn(raw)
		return "", !refusal.clock, errors.New(refusal.advice())
	case status == http.StatusUnauthorized:
		return "", false, errors.New("the switchboard refused the switchboard secret")
	case status != http.StatusOK:
		return "", false, fmt.Errorf("the switchboard answered %d to observer registration", status)
	}
	var answer struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil || answer.Token == "" {
		return "", false, errors.New("the switchboard answered observer registration without a token")
	}
	c.mu.Lock()
	if c.address == address {
		c.token = answer.Token
	}
	c.mu.Unlock()
	return answer.Token, false, nil
}

type machineRefusal struct {
	message         string
	clock           bool
	clockDifference int64
}

func machineRefusalIn(raw []byte) machineRefusal {
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	var message, reason string
	json.Unmarshal(fields["error"], &message)
	refusal := machineRefusal{message: clean.Text(message)}
	if json.Unmarshal(fields["reason"], &reason) != nil || reason != "clock" {
		return refusal
	}
	if string(fields["clock_difference"]) == "null" || json.Unmarshal(fields["clock_difference"], &refusal.clockDifference) != nil {
		return refusal
	}
	refusal.clock = true
	return refusal
}

func (r machineRefusal) advice() string {
	if r.clock {
		seconds, direction := uint64(r.clockDifference), "ahead of"
		if r.clockDifference < 0 {
			seconds, direction = uint64(-r.clockDifference), "behind"
		}
		return fmt.Sprintf("the clock of this machine runs %d seconds %s the switchboard's. Set the clock", seconds, direction)
	}
	generic := "the switchboard refused this machine's key. The shared secret may have been rotated, this machine may have been revoked, or its clock may run behind the switchboard's. If the refusal persists, run landline server join <host:port> --code <code> to join again"
	if r.message == "" {
		return generic
	}
	return r.message + ". " + generic
}

func (c *Client) list(token string) ([]Agent, int, error) {
	req, err := http.NewRequest(http.MethodGet, c.Address()+"/v1/agents", nil)
	if err != nil {
		return nil, 0, errors.New("the switchboard address is not valid")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	raw, status, err := c.do(req)
	if err != nil || status != http.StatusOK {
		return nil, status, err
	}
	var agents []Agent
	if err := json.Unmarshal(raw, &agents); err != nil {
		return nil, status, errors.New("the switchboard answered the agent list with something that is not JSON agents")
	}
	return agents, status, nil
}

func (c *Client) do(req *http.Request) ([]byte, int, error) {
	res, err := c.http.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, 0, errors.New(clean.Text(fmt.Sprintf("cannot reach the switchboard at %s: %v", req.URL.Scheme+"://"+req.URL.Host, err)))
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		return nil, 0, errors.New("the switchboard answered with a redirect")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return nil, 0, errors.New(clean.Text(fmt.Sprintf("cannot read the answer of the switchboard at %s: %v", req.URL.Scheme+"://"+req.URL.Host, err)))
	}
	if len(raw) > maxResponseBytes {
		return nil, 0, errors.New("the switchboard answered with more than 8 MiB")
	}
	return raw, res.StatusCode, nil
}
