package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/uptimy/agent/internal/config"
	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/statuspage"
)

// A 1x1 PNG.
const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

func TestStatusPageSectionsAndBranding(t *testing.T) {
	c, st := newTestServerWithConfig(t, config.Config{})
	c.login("admin", adminPassword)
	mk := func(name string) int64 {
		return createTCP(t, st, name, name+":1", monitor.SourceUI).ID
	}
	web, api, db, cache := mk("web"), mk("api"), mk("db"), mk("cache")

	page := func(extra map[string]any) map[string]any {
		body := map[string]any{
			"enabled": true, "title": "Acme", "show_events": true,
			"sections": []map[string]any{{"id": "apps", "name": "Apps"}, {"id": "data", "name": "Data"}, {"id": "empty", "name": "Empty"}},
			"monitors": []map[string]any{
				{"id": db, "public": true, "section": "data"},
				{"id": web, "public": true, "section": "apps"},
				{"id": api, "public": true, "section": "apps"},
				{"id": cache, "public": true, "section": "gone"}, // unknown section: off the page
			},
		}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}

	code, cfg := c.do("PUT", "/api/status-page", page(map[string]any{
		"accent_color": "#FF5500", "website_url": "https://acme.test",
	}))
	if code != 200 || cfg["accent_color"] != "#ff5500" {
		t.Fatalf("save: %d %v", code, cfg)
	}

	var pub struct {
		AccentColor string `json:"accent_color"`
		WebsiteURL  string `json:"website_url"`
		Sections    []struct {
			Name     string
			Monitors []struct{ Name string }
		}
		Logos map[string]string
	}
	get := func() {
		t.Helper()
		resp, err := http.Get(c.base + "/api/status")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		pub.Logos = nil
		if err := json.Unmarshal(raw, &pub); err != nil {
			t.Fatal(err)
		}
	}
	get()
	var layout []string
	for _, sec := range pub.Sections {
		var names []string
		for _, m := range sec.Monitors {
			names = append(names, m.Name)
		}
		layout = append(layout, sec.Name+":"+strings.Join(names, ","))
	}
	// Section order from the editor, monitor order within it, empty sections hidden.
	if strings.Join(layout, " ") != "Apps:web,api Data:db" {
		t.Fatalf("layout: %v", layout)
	}
	if pub.AccentColor != "#ff5500" || pub.WebsiteURL != "https://acme.test" {
		t.Fatalf("branding: %+v", pub)
	}

	for name, bad := range map[string]map[string]any{
		"color":      {"accent_color": "red"},
		"website":    {"website_url": "javascript:alert(1)"},
		"blank name": {"sections": []map[string]any{{"id": "a", "name": " "}}},
		"dup ids":    {"sections": []map[string]any{{"id": "a", "name": "A"}, {"id": "a", "name": "B"}}},
		"bad id":     {"sections": []map[string]any{{"id": "A B", "name": "A"}}},
	} {
		if code, _ := c.do("PUT", "/api/status-page", page(bad)); code != 400 {
			t.Errorf("%s accepted: %d", name, code)
		}
	}

	// Logos: uploaded as base64 JSON, checked by content, served publicly.
	code, cfg = c.do("PUT", "/api/status-page/logo/light", map[string]string{"data": tinyPNG})
	if code != 200 {
		t.Fatalf("upload: %d %v", code, cfg)
	}
	get()
	logo := pub.Logos["light"]
	if !strings.HasPrefix(logo, "/api/status/logo/light?v=") || pub.Logos["dark"] != "" {
		t.Fatalf("logos: %v", pub.Logos)
	}
	resp, err := http.Get(c.base + logo)
	if err != nil {
		t.Fatal(err)
	}
	img, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	want, _ := base64.StdEncoding.DecodeString(tinyPNG)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" || string(img) != string(want) ||
		!strings.Contains(resp.Header.Get("Cache-Control"), "immutable") || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("serving: %d %v", resp.StatusCode, resp.Header)
	}

	svg := base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	if code, _ := c.do("PUT", "/api/status-page/logo/dark", map[string]string{"data": svg}); code != 200 {
		t.Fatalf("svg upload: %d", code)
	}
	html := base64.StdEncoding.EncodeToString([]byte("<html><body>hi</body></html>"))
	if code, _ := c.do("PUT", "/api/status-page/logo/light", map[string]string{"data": html}); code != 400 {
		t.Fatalf("non-image accepted: %d", code)
	}
	big := base64.StdEncoding.EncodeToString(append(want, make([]byte, statuspage.MaxLogoBytes)...))
	if code, _ := c.do("PUT", "/api/status-page/logo/light", map[string]string{"data": big}); code != 413 && code != 400 {
		t.Fatalf("oversized logo accepted: %d", code)
	}
	if code, _ := c.do("DELETE", "/api/status-page/logo/light", nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	get()
	if pub.Logos["light"] != "" || pub.Logos["dark"] == "" {
		t.Fatalf("after delete: %v", pub.Logos)
	}

	// Viewers can't change branding.
	c.do("POST", "/api/users", map[string]string{"username": "vic", "password": "temp-pass-1", "role": "viewer"})
	v := c.another()
	v.login("vic", "temp-pass-1")
	v.do("POST", "/api/auth/password", map[string]string{"current": "temp-pass-1", "new": "vics-password"})
	if code, _ := v.do("PUT", "/api/status-page/logo/light", map[string]string{"data": tinyPNG}); code != 403 {
		t.Fatalf("viewer uploaded a logo: %d", code)
	}
}
