package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("the hub answered %d", e.Status)
	}
	return fmt.Sprintf("%s (%d)", e.Message, e.Status)
}

const maxStreamLine = 2 << 20

type Client struct {
	URL       string
	Token     string
	http      *nethttp.Client
	streaming *nethttp.Client
}

func New(url, token string) *Client {
	return &Client{URL: url, Token: token, http: &nethttp.Client{Timeout: 90 * time.Second}, streaming: &nethttp.Client{}}
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
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode != nethttp.StatusOK {
		var e model.ErrorResponse
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return &StatusError{Status: resp.StatusCode, Message: e.Error}
		}
		return &StatusError{Status: resp.StatusCode}
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

func (c *Client) Attach(req model.AttachRequest) (model.AttachResponse, error) {
	var out model.AttachResponse
	return out, c.do("POST", "/v1/attach", req, &out)
}

func (c *Client) Send(body model.SendBody) (model.SendResponse, error) {
	var out model.SendResponse
	return out, c.do("POST", "/v1/send", body, &out)
}

func (c *Client) Agents() ([]model.AgentInfo, error) {
	var out []model.AgentInfo
	return out, c.do("GET", "/v1/agents", nil, &out)
}

func (c *Client) Log(after int64) ([]model.Envelope, error) {
	var out []model.Envelope
	return out, c.do("GET", "/v1/log?after="+strconv.FormatInt(after, 10), nil, &out)
}

func (c *Client) Report(id string, report model.DeliveryReport) error {
	return c.do("POST", "/v1/deliveries/"+url.PathEscape(id)+"/report", report, nil)
}

func (c *Client) Stream(ctx context.Context, handle func(model.Delivery)) error {
	req, err := nethttp.NewRequestWithContext(ctx, "GET", c.URL+"/v1/stream", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.streaming.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var e model.ErrorResponse
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return &StatusError{Status: resp.StatusCode, Message: e.Error}
		}
		return &StatusError{Status: resp.StatusCode}
	}
	stop := context.AfterFunc(ctx, func() { resp.Body.Close() })
	defer stop()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxStreamLine)
	var event string
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if event == "delivery" && len(data) > 0 {
				var delivery model.Delivery
				if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &delivery); err != nil {
					return fmt.Errorf("the hub sent a delivery that is not valid JSON: %w", err)
				}
				handle(delivery)
			}
			event, data = "", nil
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return scanner.Err()
}
