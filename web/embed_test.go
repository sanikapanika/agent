package web

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevServerProxy(t *testing.T) {
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "websocket" {
			// Minimal stand-in for Vite's HMR socket: accept the upgrade, echo a line.
			conn, rw, _ := w.(http.Hijacker).Hijack()
			defer conn.Close()
			rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			rw.Flush()
			line, _ := rw.ReadString('\n')
			rw.WriteString("echo:" + line)
			rw.Flush()
			return
		}
		io.WriteString(w, "vite:"+r.URL.Path)
	}))
	defer vite.Close()

	h, err := Handler(vite.URL)
	if err != nil {
		t.Fatal(err)
	}
	agent := httptest.NewServer(h)
	defer agent.Close()

	resp, err := http.Get(agent.URL + "/src/main.tsx")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "vite:/src/main.tsx" {
		t.Fatalf("proxied body = %q", body)
	}

	// Hot reload rides on a websocket; the upgrade must pass through.
	conn, err := net.Dial("tcp", strings.TrimPrefix(agent.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	br := bufio.NewReader(conn)
	status, _ := br.ReadString('\n')
	if !strings.Contains(status, "101") {
		t.Fatalf("upgrade not proxied: %q", status)
	}
	for {
		line, _ := br.ReadString('\n')
		if line == "\r\n" || line == "" {
			break
		}
	}
	io.WriteString(conn, "ping\n")
	if echo, _ := br.ReadString('\n'); echo != "echo:ping\n" {
		t.Fatalf("websocket traffic not relayed: %q", echo)
	}
}

func TestDevServerDown(t *testing.T) {
	h, err := Handler("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "Waiting for the UI dev server") {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestInvalidDevServer(t *testing.T) {
	if _, err := Handler("localhost:5173"); err == nil {
		t.Fatal("expected an error for a URL without a scheme")
	}
}
