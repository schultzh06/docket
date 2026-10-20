package jmap

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"
)

func TestParseSession(t *testing.T) {
	base, err := os.ReadFile("testdata/session.json")
	if err != nil {
		t.Fatal(err)
	}
	su, _ := url.Parse("https://api.fastmail.com/jmap/session")

	s, err := parseSession(base, su)
	if err != nil {
		t.Fatalf("real fixture rejected: %v", err)
	}
	if s.MailAccountID == "" || s.Core.MaxObjectsInGet < 1 {
		t.Fatalf("bad parse: %+v", s)
	}

	// Each case edits the decoded JSON, then expects rejection.
	cases := map[string]func(m map[string]any){
		"no core":      func(m map[string]any) { delete(m["capabilities"].(map[string]any), CapCore) },
		"no mail acct": func(m map[string]any) { m["primaryAccounts"] = map[string]any{} },
		"http apiUrl":  func(m map[string]any) { m["apiUrl"] = "http://api.fastmail.com/jmap/api/" },
		"other host":   func(m map[string]any) { m["apiUrl"] = "https://evil.example/jmap/api/" },
		"suffix trick": func(m map[string]any) { m["apiUrl"] = "https://evilapi.fastmail.com/jmap/api/" },
		"host prefix":  func(m map[string]any) { m["apiUrl"] = "https://api.fastmail.com.evil.example/jmap/api/" },
		"other port":   func(m map[string]any) { m["apiUrl"] = "https://phl.api.fastmail.com:8443/jmap/api/" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal(base, &m); err != nil {
				t.Fatal(err)
			}
			mutate(m)
			b, _ := json.Marshal(m)
			if _, err := parseSession(b, su); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
