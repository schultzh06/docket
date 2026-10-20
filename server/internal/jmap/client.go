package jmap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const maxResponseBytes = 32 << 20 // 32 MiB

type Client struct {
	hc         *http.Client
	sessionURL *url.URL
	token      string

	mu      sync.Mutex
	session *Session
}

func New(sessionURL, token string, hc *http.Client) (*Client, error) {
	u, err := url.Parse(sessionURL)
	if err != nil {
		return nil, fmt.Errorf("jmap: session url: %w", err)
	}
	if u.Scheme != "https" {
		return nil, errors.New("jmap: session url must be https")
	}
	if token == "" {
		return nil, errors.New("jmap: empty token")
	}
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{hc: hc, sessionURL: u, token: token}, nil
}

func (c *Client) getSession(ctx context.Context) (*Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return c.session, nil
	}
	s, err := fetchSession(ctx, c.hc, c.sessionURL, c.token)
	if err != nil {
		return nil, err
	}
	c.session = s
	return s, nil
}

// Clears the cache if it still holds s
func (c *Client) invalidate(s *Session) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == s {
		c.session = nil
	}
}

// Call: sends one batched request and returns the raw method responses.
// Use result to extract args or *MethodError
func (c *Client) Call(ctx context.Context, calls []Invocation) ([]Invocation, error) {
	s, err := c.getSession(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := c.post(ctx, s, calls)
	if errors.Is(err, ErrUnauthorized) {
		c.invalidate(s)
		if s, err = c.getSession(ctx); err != nil {
			return nil, err
		}
		resp, err = c.post(ctx, s, calls)
	}
	if err != nil {
		return nil, err
	}

	if resp.SessionState != s.State {
		c.invalidate(s)
	}

	return resp.MethodResponses, nil
}

func (c *Client) post(ctx context.Context, s *Session, calls []Invocation) (*Response, error) {
	if len(calls) > s.Core.MaxCallsInRequest {
		return nil, fmt.Errorf("jmap: %d calls exceeds maxCallsInRequest %d", len(calls), s.Core.MaxCallsInRequest)
	}
	body, err := json.Marshal(Request{Using: []string{CapCore, CapMail}, MethodCalls: calls})
	if err != nil {
		return nil, fmt.Errorf("jmap: encode request: %w", err)
	}
	if s.Core.MaxSizeRequest > 0 && int64(len(body)) > s.Core.MaxSizeRequest {
		return nil, fmt.Errorf("jmap: request is %d bytes, maxSizeRequest = %d", len(body), s.Core.MaxSizeRequest)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.API.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jmap: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jmap: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	b, err := readLimited(resp.Body, maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("jmap: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseRequestError(resp.StatusCode, b)
	}

	var r Response
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("jmap: decode response: %w", err)
	}
	return &r, nil
}
