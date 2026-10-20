package jmap

import "fmt"

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
