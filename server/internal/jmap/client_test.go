package jmap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestCallHappyPath(t *testing.T) {
	f := newFake(t, nil, echo)
	c := f.client(t)

	resps, err := c.Call(context.Background(), []Invocation{
		{Name: "Core/echo", Args: json.RawMessage(`{"hello":true}`), CallID: "c1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	args, err := Result(resps, "c1", "Core/echo")
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != `{"hello":true}` {
		t.Errorf("args = %s", args)
	}
	if s, a := f.sessionHits.Load(), f.apiHits.Load(); s != 1 || a != 1 {
		t.Errorf("hits: session=%d api=%d, want 1/1", s, a)
	}
}

func TestCallReusesSession(t *testing.T) {
	f := newFake(t, nil, echo)
	c := f.client(t)
	for range 2 {
		if _, err := c.Call(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.sessionHits.Load(); got != 1 {
		t.Errorf("session fetched %d times, want 1", got)
	}
}

func TestCallRetriesOnceOn401(t *testing.T) {
	f := newFake(t, nil, func(t *testing.T, w http.ResponseWriter, r *http.Request, hit int32) {
		if hit == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		echo(t, w, r, hit)
	})
	if _, err := f.client(t).Call(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if s, a := f.sessionHits.Load(), f.apiHits.Load(); s != 2 || a != 2 {
		t.Errorf("hits: session=%d api=%d, want 2/2", s, a)
	}
}

func TestCallRevokedTokenDoesNotLoop(t *testing.T) {
	f := newFake(t, nil, func(_ *testing.T, w http.ResponseWriter, _ *http.Request, _ int32) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := f.client(t).Call(context.Background(), nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if a := f.apiHits.Load(); a != 2 {
		t.Errorf("api hit %d times, want exactly 2", a)
	}
}

func TestCallRefetchesSessionOnStateChange(t *testing.T) {
	f := newFake(t, nil, func(t *testing.T, w http.ResponseWriter, _ *http.Request, _ int32) {
		writeResp(t, w, "s2") // differs from the session's "s1"
	})
	c := f.client(t)
	for range 2 {
		if _, err := c.Call(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.sessionHits.Load(); got != 2 {
		t.Errorf("session fetched %d times, want 2", got)
	}
}

func TestCallRejectsTooManyCalls(t *testing.T) {
	f := newFake(t, func(m map[string]any) {
		core := m["capabilities"].(map[string]any)[CapCore].(map[string]any)
		core["maxCallsInRequest"] = 2
	}, echo)

	calls := make([]Invocation, 3)
	for i := range calls {
		calls[i] = Invocation{Name: "Core/echo", Args: json.RawMessage(`{}`), CallID: "c"}
	}
	if _, err := f.client(t).Call(context.Background(), calls); err == nil {
		t.Fatal("expected error")
	}
	if a := f.apiHits.Load(); a != 0 {
		t.Errorf("api hit %d times; limit should be enforced client-side", a)
	}
}

func TestCallRequestError(t *testing.T) {
	cases := map[string]struct {
		status   int
		body     string
		wantType string
	}{
		"problem details": {400, `{"type":"urn:ietf:params:jmap:error:limit","limit":"maxSizeRequest","status":400}`,
			"urn:ietf:params:jmap:error:limit"},
		"html from proxy": {502, `<html>Bad Gateway</html>`, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake(t, nil, func(_ *testing.T, w http.ResponseWriter, _ *http.Request, _ int32) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := f.client(t).Call(context.Background(), nil)
			var re *RequestError
			if !errors.As(err, &re) {
				t.Fatalf("err = %v, want *RequestError", err)
			}
			if re.Status != tc.status || re.Type != tc.wantType {
				t.Errorf("got %+v", re)
			}
		})
	}
}

func TestCallCanceledContext(t *testing.T) {
	f := newFake(t, nil, echo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	_, err := f.client(t).Call(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if s := f.sessionHits.Load(); s != 0 {
		t.Errorf("session hit %d times after cancel", s)
	}
}
