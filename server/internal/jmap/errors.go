package jmap

import (
	"encoding/json"
	"fmt"
)

type MethodError struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	CallID      string `json:"-"`
}

func (e *MethodError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("jmap: %s (call %s): %s", e.Type, e.CallID, e.Description)
	}
	return fmt.Sprintf("jmap: %s (call %s)", e.Type, e.CallID)
}

// RequestError is a whole-request failure (non-200 response).
type RequestError struct {
	Status int    `json:"status"`
	Type   string `json:"type"`
	Limit  string `json:"limit,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func (e *RequestError) Error() string {
	msg := fmt.Sprintf("jmap: request failed: HTTP %d", e.Status)
	if e.Type != "" {
		msg += " " + e.Type
	}
	if e.Limit != "" {
		msg += " (limit " + e.Limit + ")"
	}
	return msg
}

func parseRequestError(status int, b []byte) error {
	e := &RequestError{}
	_ = json.Unmarshal(b, e) // may not be JSON (a proxy's HTML error page)
	e.Status = status        // trust the HTTP status over the body
	return e
}
