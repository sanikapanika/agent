package connect

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeHeartbeatURL(t *testing.T) {
	cases := map[string]string{
		"abc123XYZ_-": DefaultHeartbeatBase + "abc123XYZ_-",
		"  https://heartbeats.upti.my/v1/monitors/tok  ": "https://heartbeats.upti.my/v1/monitors/tok",
		"http://localhost:9000/ping":                     "http://localhost:9000/ping",
	}
	for in, want := range cases {
		got, err := NormalizeHeartbeatURL(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x/y", "not a url", "abc"} {
		if _, err := NormalizeHeartbeatURL(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestPingNowRecordsOutcome(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/missing") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewWatchdog("", false, time.Minute, "test", func() (int, int) { return 3, 1 }, log)
	ctx := context.Background()

	if err := w.PingNow(ctx); err == nil {
		t.Fatal("ping without a URL should fail")
	}
	if w.Status().Enabled {
		t.Fatal("should start disabled")
	}

	w.SetURL(srv.URL + "/ok")
	if err := w.PingNow(ctx); err != nil {
		t.Fatal(err)
	}
	st := w.Status()
	if !st.Enabled || !st.LastOK || st.LastPingAt == nil || st.LastError != "" {
		t.Fatalf("unexpected status %+v", st)
	}
	if got["monitors_up"] != 3.0 || got["monitors_down"] != 1.0 || got["source"] != "uptimy-agent" {
		t.Fatalf("unexpected payload %v", got)
	}

	w.SetURL(srv.URL + "/missing")
	if st := w.Status(); st.LastPingAt != nil {
		t.Fatal("changing the URL should reset the last ping")
	}
	if err := w.PingNow(ctx); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected a 404 error, got %v", err)
	}
	if st := w.Status(); st.LastOK || st.LastError == "" {
		t.Fatalf("failure not recorded: %+v", st)
	}
}

func TestErrorsDontLeakToken(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewWatchdog("http://127.0.0.1:1/v1/monitors/SECRET-TOKEN", false, time.Minute, "test", func() (int, int) { return 0, 0 }, log)
	err := w.PingNow(context.Background())
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if strings.Contains(err.Error(), "SECRET-TOKEN") || strings.Contains(w.Status().LastError, "SECRET-TOKEN") {
		t.Fatalf("error leaks the heartbeat token: %q", err)
	}
}
