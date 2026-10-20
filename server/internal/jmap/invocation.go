package jmap

import (
	"encoding/json"
	"fmt"
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
