package hubclient

import (
	"bytes"
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

	"github.com/8thlight-daveloper/landline/menubar/internal/clean"
)

const (
	requestTimeout   = 5 * time.Second
	maxResponseBytes = 8 << 20
)

var ErrControlSecret = errors.New("the switchboard secret contains a control character")

var ErrNoSecret = errors.New("there is no switchboard secret. Set LANDLINE_SECRET or run landline server join")

type Client struct {
	address string
	secret  string
	http    *http.Client
	mu      sync.Mutex
	token   string
	reread  func() string
}

func New(address, secret string) *Client {
	return newWithTimeout(address, secret, requestTimeout)
}

func newWithTimeout(address, secret string, timeout time.Duration) *Client {
	return &Client{address: address, secret: secret, http: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) Address() string { return c.address }

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
	raw, status, err := c.authorised("/v1/agents")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("the switchboard answered %d to the agent list", status)
	}
	var agents []Agent
	if err := json.Unmarshal(raw, &agents); err != nil {
		return nil, errors.New("the switchboard answered the agent list with something that is not JSON agents")
	}
	return agents, nil
}

func (c *Client) Build() (Build, bool, error) {
	raw, status, err := c.authorised("/v1/build")
	if err != nil {
		return Build{}, false, err
	}
	var build Build
	if status != http.StatusOK || json.Unmarshal(raw, &build) != nil {
		return Build{}, false, nil
	}
	return build, true, nil
}

func (c *Client) authorised(path string) ([]byte, int, error) {
	token, err := c.currentToken()
	if err != nil {
		return nil, 0, err
	}
	raw, status, err := c.get(path, token)
	if err != nil {
		return nil, 0, err
	}
	if status == http.StatusUnauthorized {
		if token, err = c.register(); err != nil {
			return nil, 0, err
		}
		return c.get(path, token)
	}
	return raw, status, nil
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

func (c *Client) register() (string, error) {
	secret := c.currentSecret()
	if secret == "" {
		return "", ErrNoSecret
	}
	if strings.ContainsFunc(secret, unicode.IsControl) {
		return "", ErrControlSecret
	}
	body, _ := json.Marshal(map[string]string{"kind": "menubar"})
	req, err := http.NewRequest(http.MethodPost, c.address+"/v1/observers", bytes.NewReader(body))
	if err != nil {
		return "", errors.New("the switchboard address is not valid")
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	raw, status, err := c.do(req)
	if err != nil {
		return "", err
	}
	switch {
	case status == http.StatusUnauthorized:
		return "", errors.New("the switchboard refused the switchboard secret")
	case status != http.StatusOK:
		return "", fmt.Errorf("the switchboard answered %d to observer registration", status)
	}
	var answer struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil || answer.Token == "" {
		return "", errors.New("the switchboard answered observer registration without a token")
	}
	c.mu.Lock()
	c.token = answer.Token
	c.mu.Unlock()
	return answer.Token, nil
}

func (c *Client) get(path, token string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, c.address+path, nil)
	if err != nil {
		return nil, 0, errors.New("the switchboard address is not valid")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, int, error) {
	res, err := c.http.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, 0, errors.New(clean.Text(fmt.Sprintf("cannot reach the switchboard at %s: %v", c.address, err)))
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		return nil, 0, errors.New("the switchboard answered with a redirect")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return nil, 0, errors.New(clean.Text(fmt.Sprintf("cannot read the answer of the switchboard at %s: %v", c.address, err)))
	}
	if len(raw) > maxResponseBytes {
		return nil, 0, errors.New("the switchboard answered with more than 8 MiB")
	}
	return raw, res.StatusCode, nil
}
