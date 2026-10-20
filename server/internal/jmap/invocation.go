package jmap

import (
	"encoding/json"
	"fmt"
)

const (
	CapCore = "urn:ietf:params:jmap:core"
	CapMail = "urn:ietf:params:jmap:mail"
)

type Invocation struct {
	Name   string
	Args   json.RawMessage
	CallID string
}

func (i Invocation) MarshalJSON() ([]byte, error) {
	return json.Marshal([3]any{i.Name, i.Args, i.CallID})
}

func (i *Invocation) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("jmap: invocation: %w", err)
	}
	if len(raw) != 3 {
		return fmt.Errorf("jmap: invocation has %d elements, want 3", len(raw))
	}
	if err := json.Unmarshal(raw[0], &i.Name); err != nil {
		return fmt.Errorf("jmap: invocation name: %w", err)
	}
	i.Args = raw[1]
	if err := json.Unmarshal(raw[2], &i.CallID); err != nil {
		return fmt.Errorf("jmap: invocation call id: %w", err)
	}
	return nil
}

type Request struct {
	Using       []string     `json:"using"`
	MethodCalls []Invocation `json:"methodCalls"`
}

type Response struct {
	MethodResponses []Invocation `json:"methodResponses"`
	SessionState    string       `json:"sessionState"`
}

// Result returns the arguments of the response to callID, which must be named name.
// A server "error" response is returned as a *MethodError.
func Result(resps []Invocation, callID, name string) (json.RawMessage, error) {
	for _, r := range resps {
		if r.CallID != callID {
			continue
		}
		switch r.Name {
		case name:
			return r.Args, nil
		case "error":
			me := &MethodError{CallID: callID}
			if err := json.Unmarshal(r.Args, me); err != nil {
				return nil, fmt.Errorf("jmap: decode error for call %s: %w", callID, err)
			}
			return nil, me
		}
	}
	return nil, fmt.Errorf("jmap: no %s response for call %s", name, callID)
}
