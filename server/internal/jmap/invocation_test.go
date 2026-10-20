package jmap

import (
	"encoding/json"
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
