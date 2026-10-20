package jmap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const MaxSessionBytes = 1 << 20 // 1 MiB

var ErrUnauthorized = errors.New("jmap: unauthorized")

type CoreCapability struct {
	MaxSizeRequest    int64 `json:"maxSizeRequest"`
	MaxCallsInRequest int   `json:"maxCallsInRequest"`
	MaxObjectsInGet   int   `json:"maxObjectsInGet"`
}

type Session struct {
	APIURL          string                     `json:"apiUrl"`
	PrimaryAccounts map[string]string          `json:"primaryAccounts"`
	Capabilities    map[string]json.RawMessage `json:"capabilities"`
	State           string                     `json:"state"`

	// Derived and validated
	API           *url.URL       `json:"-"`
	Core          CoreCapability `json:"-"`
	MailAccountID string         `json:"-"`
}

func fetchSession(ctx context.Context, hc *http.Client, sessionURL *url.URL, token string) (*Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sessionURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("jmap: session request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jmap: fetch session: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("jmap: fetch session: HTTP %d", resp.StatusCode)
	}

	b, err := readLimited(resp.Body, MaxSessionBytes)
	if err != nil {
		return nil, fmt.Errorf("jmap: fetch session: %w", err)
	}
	return parseSession(b, sessionURL)
}

func parseSession(b []byte, sessionURL *url.URL) (*Session, error) {
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("jmap: decode session: %w", err)
	}

	raw, ok := s.Capabilities[CapCore]
	if !ok {
		return nil, errors.New("jmap: session missing core capability")
	}
	if err := json.Unmarshal(raw, &s.Core); err != nil {
		return nil, fmt.Errorf("jmap: decode core capability: %w", err)
	}
	if s.Core.MaxCallsInRequest < 1 || s.Core.MaxObjectsInGet < 1 {
		return nil, fmt.Errorf("jmap: implausible core limits %+v", s.Core)
	}

	s.MailAccountID = s.PrimaryAccounts[CapMail]
	if s.MailAccountID == "" {
		return nil, errors.New("jmap: session has no primary mail account")
	}

	api, err := url.Parse(s.APIURL)
	if err != nil {
		return nil, fmt.Errorf("jmap: parse apiUrl: %w", err)
	}
	api = sessionURL.ResolveReference(api) // handles relative apiUrl too
	if api.Scheme != "https" || !sameOrSubdomain(api, sessionURL) {
		return nil, fmt.Errorf("jmap: refusing apiUrl %q: must be https on %s", api, sessionURL.Host)
	}
	s.API = api
	return &s, nil
}

// sameOrSubdomain reports whether api is on sessionURL's host or a subdomain
// of it (Fastmail hands out regional hosts like phl.api.fastmail.com).
func sameOrSubdomain(api, sessionURL *url.URL) bool {
	if api.Port() != sessionURL.Port() {
		return false
	}
	h, base := strings.ToLower(api.Hostname()), strings.ToLower(sessionURL.Hostname())
	return h == base || strings.HasSuffix(h, "."+base)
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("response exceeds %d bytes", max)
	}
	return b, nil
}
