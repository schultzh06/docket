package jmap

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestLiveFastmail(t *testing.T) {
	token := os.Getenv("DOCKET_FASTMAIL_TOKEN")
	if os.Getenv("JMAP_LIVE") == "" || token == "" {
		t.Skip("set JMAP_LIVE=1 and DOCKET_FASTMAIL_TOKEN to run against Fastmail")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c, err := New("https://api.fastmail.com/jmap/session", token, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.getSession(ctx) // internal test, so unexported access is fine
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("limits: %+v", s.Core)

	args, _ := json.Marshal(map[string]any{
		"accountId":  s.MailAccountID,
		"ids":        nil, // null = all mailboxes
		"properties": []string{"id", "name", "role"},
	})
	resps, err := c.Call(ctx, []Invocation{
		{Name: "Core/echo", Args: json.RawMessage(`{"ping":"docket"}`), CallID: "c1"},
		{Name: "Mailbox/get", Args: args, CallID: "c2"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if echo, err := Result(resps, "c1", "Core/echo"); err != nil {
		t.Fatal(err)
	} else {
		t.Logf("echo: %s", echo)
	}
	mb, err := Result(resps, "c2", "Mailbox/get")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("mailboxes: %s", mb)
}
