package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Client struct {
	URL   string
	Token string
	http  *nethttp.Client
}

func New(url, token string) *Client {
	return &Client{URL: url, Token: token, http: &nethttp.Client{Timeout: 90 * time.Second}}
}

func (c *Client) do(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := nethttp.NewRequest(method, c.URL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8*model.MaxBodyBytes))
	if resp.StatusCode != nethttp.StatusOK {
		var e model.ErrorResponse
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("%s (%d)", e.Error, resp.StatusCode)
		}
		return fmt.Errorf("the hub answered %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) Register(req model.RegisterRequest) (model.RegisterResponse, error) {
	var out model.RegisterResponse
	return out, c.do("POST", "/v1/register", req, &out)
}

func (c *Client) Send(body model.SendBody) (model.SendResponse, error) {
	var out model.SendResponse
	return out, c.do("POST", "/v1/send", body, &out)
}

func (c *Client) Inbox(waitSeconds int, threads string, after int64) (model.InboxResponse, error) {
	q := url.Values{}
	if waitSeconds > 0 {
		q.Set("wait", strconv.Itoa(waitSeconds))
	}
	if threads != "" {
		q.Set("thread", threads)
		q.Set("after", strconv.FormatInt(after, 10))
	}
	var out model.InboxResponse
	return out, c.do("GET", "/v1/inbox?"+q.Encode(), nil, &out)
}

func (c *Client) Agents() ([]model.AgentInfo, error) {
	var out []model.AgentInfo
	return out, c.do("GET", "/v1/agents", nil, &out)
}

func (c *Client) Log(after int64) ([]model.Envelope, error) {
	var out []model.Envelope
	return out, c.do("GET", "/v1/log?after="+strconv.FormatInt(after, 10), nil, &out)
}
