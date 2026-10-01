package checks

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uptimy/agent/internal/monitor"
)

// normalized returns c with its defaults filled in, as a saved healthcheck has.
func normalized(t *testing.T, c monitor.Check) monitor.Check {
	t.Helper()
	m := monitor.Monitor{Kind: monitor.KindHealthcheck, Name: "t", Check: &c}
	if err := m.Normalize(); err != nil {
		t.Fatal(err)
	}
	return *m.Check
}

func httpMonitor(t *testing.T, target string, cfg monitor.Config) monitor.Check {
	return normalized(t, monitor.Check{Type: monitor.TypeHTTP, Target: target, Config: cfg})
}

func TestHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()
	c := New(nil)
	ctx := context.Background()

	if out := c.Run(ctx, httpMonitor(t, srv.URL, monitor.Config{})); !out.OK {
		t.Fatalf("expected ok: %s", out.Message)
	}
	if out := c.Run(ctx, httpMonitor(t, srv.URL+"/fail", monitor.Config{})); out.OK || !strings.Contains(out.Message, "500") {
		t.Fatalf("expected 500 failure: %+v", out)
	}
	if out := c.Run(ctx, httpMonitor(t, srv.URL+"/fail", monitor.Config{ExpectedStatus: "500"})); !out.OK {
		t.Fatalf("expected 500 to be accepted: %s", out.Message)
	}
	if out := c.Run(ctx, httpMonitor(t, srv.URL, monitor.Config{Keyword: "degraded"})); out.OK {
		t.Fatal("missing keyword should fail")
	}
}

func TestTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ctx := context.Background()
	if out := tcp(ctx, addr); !out.OK {
		t.Fatalf("expected connect: %s", out.Message)
	}
	ln.Close()
	if out := tcp(ctx, addr); out.OK {
		t.Fatal("expected failure after close")
	}
}

func TestKubernetesUnavailable(t *testing.T) {
	m := monitor.Check{Type: monitor.TypeKubernetes, Target: "default/deployment/api"}
	if out := New(nil).Run(context.Background(), m); out.OK {
		t.Fatal("expected failure outside a cluster")
	}
}
