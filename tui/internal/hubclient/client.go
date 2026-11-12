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

	"github.com/8thlight-daveloper/landline/tui/internal/clean"
)

const (
	requestTimeout   = 5 * time.Second
	maxResponseBytes = 8 << 20
)

var ErrControlSecret = errors.New("the hub secret contains a control character")

var ErrNoSecret = errors.New("there is no hub secret. Set LANDLINE_SECRET or run landline server join")

type Client struct {
	address string
	secret  string
	http    *http.Client
	mu      sync.Mutex
	token   string
}

func New(address, secret string) *Client {
	return newWithTimeout(address, secret, requestTimeout)
}

func newWithTimeout(address, secret string, timeout time.Duration) *Client {
	return &Client{address: address, secret: secret, http: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) Address() string { return c.address }

func (c *Client) Agents() ([]Agent, error) {
	if c.secret == "" {
		return nil, ErrNoSecret
	}
	if strings.ContainsFunc(c.secret, unicode.IsControl) {
		return nil, ErrControlSecret
	}
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
		return nil, fmt.Errorf("the hub answered %d to the agent list", status)
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

func (c *Client) register() (string, error) {
	body, _ := json.Marshal(map[string]string{"kind": "tui"})
	req, err := http.NewRequest(http.MethodPost, c.address+"/v1/observers", bytes.NewReader(body))
	if err != nil {
		return "", errors.New("the hub address is not valid")
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")
	raw, status, err := c.do(req)
	if err != nil {
		return "", err
	}
	switch {
	case status == http.StatusUnauthorized:
		return "", errors.New("the hub refused the hub secret")
	case status != http.StatusOK:
		return "", fmt.Errorf("the hub answered %d to observer registration", status)
	}
	var answer struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil || answer.Token == "" {
		return "", errors.New("the hub answered observer registration without a token")
	}
	c.mu.Lock()
	c.token = answer.Token
	c.mu.Unlock()
	return answer.Token, nil
}

func (c *Client) list(token string) ([]Agent, int, error) {
	req, err := http.NewRequest(http.MethodGet, c.address+"/v1/agents", nil)
	if err != nil {
		return nil, 0, errors.New("the hub address is not valid")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	raw, status, err := c.do(req)
	if err != nil || status != http.StatusOK {
		return nil, status, err
	}
	var agents []Agent
	if err := json.Unmarshal(raw, &agents); err != nil {
		return nil, status, errors.New("the hub answered the agent list with something that is not JSON agents")
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
		return nil, 0, errors.New(clean.Text(fmt.Sprintf("cannot reach the hub at %s: %v", c.address, err)))
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		return nil, 0, errors.New("the hub answered with a redirect")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return nil, 0, errors.New(clean.Text(fmt.Sprintf("cannot read the answer of the hub at %s: %v", c.address, err)))
	}
	if len(raw) > maxResponseBytes {
		return nil, 0, errors.New("the hub answered with more than 8 MiB")
	}
	return raw, res.StatusCode, nil
}
