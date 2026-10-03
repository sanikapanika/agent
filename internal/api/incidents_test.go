package api

import (
	"strconv"
	"testing"
	"time"

	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/store"
)

func TestIncidents(t *testing.T) {
	c, st := newTestServerWithStore(t)
	if code, _ := c.login("admin", adminPassword); code != 200 {
		t.Fatal("login")
	}
	api := createTCP(t, st, "API", "api:1", monitor.SourceUI)
	private := createPrivateTCP(t, st, "Internal", "db:1") // not on the page

	for name, body := range map[string]map[string]any{
		"no severity": {"title": "x", "status": "investigating", "message": "m"},
		"no message":  {"title": "x", "severity": "low", "status": "investigating"},
		"bad status":  {"title": "x", "severity": "low", "status": "fixed", "message": "m"},
		"no title":    {"severity": "low", "status": "investigating", "message": "m"},
	} {
		if code, _ := c.do("POST", "/api/incidents", body); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}

	code, inc := c.do("POST", "/api/incidents", map[string]any{
		"title": " Slow checkout ", "severity": "medium", "status": "investigating",
		"message": "Payments are slow.", "monitor_ids": []int64{api.ID, private.ID},
	})
	if code != 201 || inc["title"] != "Slow checkout" || inc["status"] != "investigating" {
		t.Fatalf("create: %d %v", code, inc)
	}
	id := strconv.Itoa(int(inc["id"].(float64)))

	public := func() map[string]any {
		t.Helper()
		_, body := c.another().do("GET", "/api/status", nil)
		return body
	}
	p := public()
	if p["overall"] != "degraded" {
		t.Fatalf("open incident: overall %v", p["overall"])
	}
	shown := p["incidents"].([]any)[0].(map[string]any)
	if ms := shown["monitors"].([]any); len(ms) != 1 || ms[0] != "API" {
		t.Fatalf("affected monitors leak or are missing: %v", ms)
	}

	if code, _ := c.do("PUT", "/api/incidents/"+id, map[string]any{"title": "Checkout down", "severity": "critical", "monitor_ids": []int64{api.ID}}); code != 200 {
		t.Fatalf("edit: %d", code)
	}
	if p := public(); p["overall"] != "outage" {
		t.Fatalf("critical incident: overall %v", p["overall"])
	}

	code, inc = c.do("POST", "/api/incidents/"+id+"/updates", map[string]any{"status": "identified", "message": "A bad deploy."})
	if code != 200 || inc["status"] != "identified" || len(inc["updates"].([]any)) != 2 {
		t.Fatalf("update: %d %v", code, inc)
	}
	code, inc = c.do("POST", "/api/incidents/"+id+"/updates", map[string]any{"status": "resolved", "message": "Rolled back."})
	if code != 200 || inc["resolved_at"] == nil {
		t.Fatalf("resolve: %d %v", code, inc)
	}
	p = public()
	if p["overall"] != "operational" {
		t.Fatalf("resolved: overall %v", p["overall"])
	}
	past := p["incidents"].([]any)[0].(map[string]any)
	if past["resolved_at"] == nil || len(past["updates"].([]any)) != 3 || past["updates"].([]any)[0].(map[string]any)["status"] != "resolved" {
		t.Fatalf("resolved incident on the page: %v", past)
	}

	// Deleting the resolution reopens it; an update can be corrected.
	latest := int(inc["updates"].([]any)[0].(map[string]any)["id"].(float64))
	code, inc = c.do("DELETE", "/api/incidents/"+id+"/updates/"+strconv.Itoa(latest), nil)
	if code != 200 || inc["resolved_at"] != nil || inc["status"] != "identified" {
		t.Fatalf("delete resolution: %d %v", code, inc)
	}
	first := int(inc["updates"].([]any)[1].(map[string]any)["id"].(float64))
	if code, inc = c.do("PUT", "/api/incidents/"+id+"/updates/"+strconv.Itoa(first), map[string]any{"message": "Payments are slow for some cards."}); code != 200 ||
		inc["updates"].([]any)[1].(map[string]any)["message"] != "Payments are slow for some cards." {
		t.Fatalf("edit update: %d %v", code, inc)
	}
	second := int(inc["updates"].([]any)[0].(map[string]any)["id"].(float64))
	c.do("DELETE", "/api/incidents/"+id+"/updates/"+strconv.Itoa(second), nil)
	if code, _ := c.do("DELETE", "/api/incidents/"+id+"/updates/"+strconv.Itoa(first), nil); code != 409 {
		t.Fatalf("deleted the only update: %d", code)
	}

	_, list := c.doList("GET", "/api/incidents")
	if len(list) != 1 {
		t.Fatalf("list: %v", list)
	}

	if code, _ := c.do("DELETE", "/api/incidents/"+id, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := c.do("POST", "/api/incidents/"+id+"/updates", map[string]any{"status": "resolved", "message": "x"}); code != 404 {
		t.Fatalf("update after delete: %d", code)
	}
}

func createPrivateTCP(t *testing.T, st *store.Store, name, target string) monitor.Monitor {
	t.Helper()
	m := monitor.Monitor{Kind: monitor.KindHealthcheck, Name: name, Paused: true, Source: monitor.SourceUI,
		Check: &monitor.Check{Type: monitor.TypeTCP, Target: target}}
	if err := m.Normalize(); err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateMonitor(t.Context(), m)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAnnouncement(t *testing.T) {
	c := newTestServer(t)
	if code, _ := c.login("admin", adminPassword); code != 200 {
		t.Fatal("login")
	}
	public := func() any {
		t.Helper()
		_, body := c.another().do("GET", "/api/status", nil)
		return body["announcement"]
	}
	if a := public(); a != nil {
		t.Fatalf("none yet: %v", a)
	}
	if code, _ := c.do("PUT", "/api/status-page/announcement", map[string]any{"title": " ", "message": "x"}); code != 400 {
		t.Fatalf("no title: %d", code)
	}
	past := time.Now().Add(-time.Minute)
	if code, _ := c.do("PUT", "/api/status-page/announcement", map[string]any{"title": "x", "show_until": past}); code != 400 {
		t.Fatalf("show until in the past: %d", code)
	}

	code, a := c.do("PUT", "/api/status-page/announcement", map[string]any{"title": " Moving to Frankfurt ", "message": "Saturday, 02:00 UTC."})
	if code != 200 || a["title"] != "Moving to Frankfurt" || a["posted_at"] == nil {
		t.Fatalf("post: %d %v", code, a)
	}
	if p, ok := public().(map[string]any); !ok || p["message"] != "Saturday, 02:00 UTC." {
		t.Fatalf("on the page: %v", p)
	}

	// Editing keeps when it was posted. Saving the page's settings doesn't touch it.
	time.Sleep(1100 * time.Millisecond)
	_, edited := c.do("PUT", "/api/status-page/announcement", map[string]any{"title": "Moving to Frankfurt", "message": "Now Sunday."})
	if edited["posted_at"] != a["posted_at"] || edited["updated_at"] == a["updated_at"] {
		t.Fatalf("edit: %v then %v", a, edited)
	}
	_, cfg := c.do("GET", "/api/status-page", nil)
	delete(cfg, "logos")
	cfg["monitors"] = []any{}
	if code, _ := c.do("PUT", "/api/status-page", cfg); code != 200 {
		t.Fatalf("save page: %d", code)
	}
	if p, ok := public().(map[string]any); !ok || p["message"] != "Now Sunday." {
		t.Fatalf("after saving the page: %v", p)
	}

	if code, _ := c.do("DELETE", "/api/status-page/announcement", nil); code != 204 {
		t.Fatalf("remove: %d", code)
	}
	if a := public(); a != nil {
		t.Fatalf("still shown: %v", a)
	}
}
