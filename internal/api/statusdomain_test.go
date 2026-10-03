package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatusDomain(t *testing.T) {
	c := newTestServer(t)
	if code, _ := c.login("admin", adminPassword); code != 200 {
		t.Fatal("login")
	}
	page := map[string]any{
		"enabled": true, "title": "Acme", "show_events": true,
		"sections": []map[string]string{{"id": "services", "name": "Services"}},
		"monitors": []any{}, "domain": "https://Status.Acme.com/",
	}
	if code, body := c.do("PUT", "/api/status-page", page); code != 200 || body["domain"] != "status.acme.com" {
		t.Fatalf("save: %d %v", code, body)
	}
	// The dashboard's own address can't become the status page's.
	page["domain"] = strings.TrimPrefix(c.base, "http://")
	if code, body := c.do("PUT", "/api/status-page", page); code != 400 {
		t.Fatalf("saved the dashboard's address: %d %v", code, body)
	}

	ui := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head><title>x</title></head></html>"))
	})
	h := c.srv.Handler(ui)
	get := func(host, method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	const domain = "status.acme.com:443"
	if w := get(domain, "GET", "/"); w.Code != 200 || !strings.Contains(w.Body.String(), "<head>"+statusViewMeta) {
		t.Fatalf("status page at /: %d %s", w.Code, w.Body)
	}
	if w := get(domain, "GET", "/status"); w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/" {
		t.Fatalf("/status: %d %v", w.Code, w.Header())
	}
	for _, path := range []string{"/api/status", "/healthz"} {
		if w := get(domain, "GET", path); w.Code != 200 {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	if w := get(domain, "GET", "/assets/index.js"); w.Code != 200 {
		t.Errorf("assets: %d", w.Code)
	}
	if w := get(domain, "POST", "/ping/no-such-token"); w.Code != 404 || !strings.Contains(w.Body.String(), "ping URL") {
		t.Errorf("ping isn't handled: %d %s", w.Code, w.Body)
	}
	for _, path := range []string{"/api/auth/state", "/api/healthchecks", "/metrics", "/healthchecks", "/settings"} {
		if w := get(domain, "GET", path); w.Code != 404 {
			t.Errorf("%s on the status domain: %d", path, w.Code)
		}
	}
	if w := get(domain, "POST", "/api/auth/login"); w.Code != 404 {
		t.Errorf("sign-in on the status domain: %d", w.Code)
	}

	// Every other host is the full app, without the marker.
	if w := get("agent.internal:8080", "GET", "/"); w.Code != 200 || strings.Contains(w.Body.String(), statusViewMeta) {
		t.Fatalf("dashboard host: %d %s", w.Code, w.Body)
	}
	if w := get("agent.internal:8080", "GET", "/api/auth/state"); w.Code != 200 {
		t.Fatalf("dashboard API: %d", w.Code)
	}

	// Clearing it gives the domain the full app again.
	page["domain"] = ""
	if code, _ := c.do("PUT", "/api/status-page", page); code != 200 {
		t.Fatal("clear")
	}
	if w := get(domain, "GET", "/api/auth/state"); w.Code != 200 {
		t.Fatalf("after clearing: %d", w.Code)
	}
}
