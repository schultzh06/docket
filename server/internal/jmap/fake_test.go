package jmap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

// apiFunc handles one POST /api; hit is 1 for the first request, 2 for the second...
type apiFunc func(t *testing.T, w http.ResponseWriter, r *http.Request, hit int32)

type fake struct {
	srv         *httptest.Server
	sessionHits atomic.Int32
	apiHits     atomic.Int32
}

func newFake(t *testing.T, mutate func(m map[string]any), api apiFunc) *fake {
	t.Helper()
	base, err := os.ReadFile("testdata/session.json")
	if err != nil {
		t.Fatal(err)
	}

	f := &fake{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /session", func(w http.ResponseWriter, r *http.Request) {
		f.sessionHits.Add(1)
		var m map[string]any
		if err := json.Unmarshal(base, &m); err != nil {
			t.Error(err)
			return
		}
		m["apiUrl"] = "https://" + r.Host + "/api" // same host as the session, so it passes the check
		m["state"] = "s1"
		if mutate != nil {
			mutate(m)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("POST /api", func(w http.ResponseWriter, r *http.Request) {
		api(t, w, r, f.apiHits.Add(1))
	})

	f.srv = httptest.NewTLSServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) client(t *testing.T) *Client {
	t.Helper()
	c, err := New(f.srv.URL+"/session", "test-token", f.srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func writeResp(t *testing.T, w http.ResponseWriter, state string, invs ...Invocation) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(Response{MethodResponses: invs, SessionState: state}); err != nil {
		t.Error(err)
	}
}

// echo checks the request and answers each call with its own name/args.
func echo(t *testing.T, w http.ResponseWriter, r *http.Request, _ int32) {
	if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q", got)
	}
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Error(err)
		return
	}
	if len(req.Using) != 2 || req.Using[0] != CapCore || req.Using[1] != CapMail {
		t.Errorf("using = %v", req.Using)
	}
	writeResp(t, w, "s1", req.MethodCalls...)
}
