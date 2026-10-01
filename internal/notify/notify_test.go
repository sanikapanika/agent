package notify

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

func TestChannelsAreComplete(t *testing.T) {
	for _, ch := range Channels() {
		if ch.Help == "" || len(ch.Fields) == 0 {
			t.Errorf("%s: needs help text and fields for the form", ch.Type)
		}
		for _, f := range ch.Fields {
			if f.Key == "" || f.Label == "" || f.Input == "" {
				t.Errorf("%s: incomplete field %+v", ch.Type, f)
			}
		}
	}
}

func TestNormalize(t *testing.T) {
	ok := []Notifier{
		{Name: "a", Type: "slack", Config: Config{URL: "https://hooks.slack.com/x"}},
		{Name: "a", Type: "telegram", Config: Config{BotToken: "1:abc", ChatID: "42"}},
	}
	for _, n := range ok {
		if err := n.Normalize(); err != nil {
			t.Errorf("%s: %v", n.Type, err)
		}
	}
	bad := []Notifier{
		{Name: "", Type: "slack", Config: Config{URL: "https://x"}},
		{Name: "a", Type: "slack", Config: Config{URL: "ftp://x"}},
		{Name: "a", Type: "telegram", Config: Config{BotToken: "1:abc"}},
		{Name: "a", Type: "pager", Config: Config{URL: "https://x"}},
	}
	for _, n := range bad {
		if err := n.Normalize(); err == nil {
			t.Errorf("%s %+v: expected an error", n.Type, n.Config)
		}
	}
}

// A failed request must not echo the endpoint, which can carry a secret.
func TestSendErrorHidesSecrets(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close() // nothing listens: the request fails before any response

	s := NewSender(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.PostJSON(ctx, "http://"+addr+"/hooks/SECRET-TOKEN", map[string]string{"text": "x"})
	if err == nil || strings.Contains(err.Error(), "SECRET-TOKEN") {
		t.Fatalf("error leaks the endpoint: %v", err)
	}
}
