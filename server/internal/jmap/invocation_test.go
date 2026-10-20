package jmap

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestInvocationRoundTrip(t *testing.T) {
	in := `["Email/get",{"accountId":"A","ids":[]},"c1"]`

	var inv Invocation
	if err := json.Unmarshal([]byte(in), &inv); err != nil {
		t.Fatal(err)
	}
	if inv.Name != "Email/get" || inv.CallID != "c1" {
		t.Fatalf("got %+v", inv)
	}

	// Encode inside a slice to catch the receiver bug described above.
	out, err := json.Marshal([]Invocation{inv})
	if err != nil {
		t.Fatal(err)
	}
	if want := "[" + in + "]"; string(out) != want {
		t.Fatalf("got %s, want %s", out, want)
	}
}

func TestInvocationRejectsBadShape(t *testing.T) {
	for _, in := range []string{`[]`, `["a","b"]`, `{"name":"x"}`, `[1,{},"c1"]`} {
		var inv Invocation
		if err := json.Unmarshal([]byte(in), &inv); err == nil {
			t.Errorf("%s: expected error", in)
		}
	}
}

func TestResult(t *testing.T) {
	resps := []Invocation{
		{Name: "Email/query", Args: json.RawMessage(`{"ids":["a"]}`), CallID: "c1"},
		{Name: "error", Args: json.RawMessage(`{"type":"cannotCalculateChanges"}`), CallID: "c2"},
		{Name: "Email/set", Args: json.RawMessage(`{}`), CallID: "c3"},
		{Name: "Email/copy", Args: json.RawMessage(`{}`), CallID: "c3"}, // implicit response sharing c3
	}

	if args, err := Result(resps, "c1", "Email/query"); err != nil || string(args) != `{"ids":["a"]}` {
		t.Errorf("c1: %s, %v", args, err)
	}

	_, err := Result(resps, "c2", "Email/changes")
	var me *MethodError
	if !errors.As(err, &me) || me.Type != "cannotCalculateChanges" || me.CallID != "c2" {
		t.Errorf("c2: err = %v", err)
	}

	if _, err := Result(resps, "c3", "Email/copy"); err != nil {
		t.Errorf("c3 copy: %v", err) // must skip the Email/set sharing the id
	}
	if _, err := Result(resps, "c9", "Email/get"); err == nil {
		t.Error("c9: expected not-found error")
	}
}
