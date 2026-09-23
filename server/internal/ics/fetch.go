package ics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// HTTP cache validators
type Validators struct {
	ETag         string
	LastModified string
}

type FetchResult struct {
	Body        []byte
	Validators  Validators
	NotModified bool
}

type Fetcher struct {
	HTTP *http.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{HTTP: &http.Client{Timeout: 30 * time.Second}}
}

const maxBody = 10 << 20 // Max 10 MiB

func (f *Fetcher) Fetch(ctx context.Context, feedURL string, prev Validators) (FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch: %w", redact(err))
	}
	if prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}
	if prev.LastModified != "" {
		req.Header.Set("If-Modified-Since", prev.LastModified)
	}

	resp, err := f.HTTP.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch: %w", redact(err))
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return FetchResult{NotModified: true, Validators: prev}, nil
	case http.StatusOK:
		// handled below
	default:
		return FetchResult{}, fmt.Errorf("fetch: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return FetchResult{}, fmt.Errorf("read body: %w", err)
	}
	if len(body) > maxBody {
		return FetchResult{}, fmt.Errorf("feed exceeds %d bytes", maxBody)
	}

	return FetchResult{
		Body: body,
		Validators: Validators{
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
		},
	}, nil
}

// redact: strips URL from net/http errors to redact credential
func redact(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		uerr.URL = "<redacted>"
	}
	return err
}
