package api

import (
	"strconv"
	"testing"

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
		"no severity": {"kind": "incident", "title": "x", "status": "investigating", "message": "m"},
		"no message":  {"kind": "incident", "title": "x", "severity": "low", "status": "investigating"},
		"bad status":  {"kind": "incident", "title": "x", "severity": "low", "status": "fixed", "message": "m"},
		"bad kind":    {"kind": "banner", "title": "x", "message": "m"},
	} {
		if code, _ := c.do("POST", "/api/incidents", body); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}

	code, inc := c.do("POST", "/api/incidents", map[string]any{
		"kind": "incident", "title": " Slow checkout ", "severity": "medium", "status": "investigating",
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
	if p["uptimy_card"] != true {
		t.Fatalf("the Uptimy card is on by default: %v", p["uptimy_card"])
	}
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

	// A notice shows until it's ended, and doesn't change the overall status
	// (the reopened critical incident's).
	code, notice := c.do("POST", "/api/incidents", map[string]any{"kind": "notice", "title": "New region", "severity": "high", "message": "We now serve from Frankfurt."})
	if code != 201 || notice["severity"] != "" {
		t.Fatalf("notice: %d %v", code, notice)
	}
	nid := strconv.Itoa(int(notice["id"].(float64)))
	if code, _ := c.do("POST", "/api/incidents/"+nid+"/updates", map[string]any{"status": "investigating", "message": "x"}); code != 400 {
		t.Fatalf("notice took an incident status: %d", code)
	}
	if p := public(); len(p["incidents"].([]any)) != 2 || p["overall"] != "outage" {
		t.Fatalf("with a notice: %v %v", len(p["incidents"].([]any)), p["overall"])
	}
	c.do("POST", "/api/incidents/"+nid+"/updates", map[string]any{"status": "resolved", "message": "Ended."})
	if p := public(); len(p["incidents"].([]any)) != 1 {
		t.Fatalf("ended notice still shown: %v", p["incidents"])
	}
	_, list := c.doList("GET", "/api/incidents")
	if len(list) != 2 {
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
