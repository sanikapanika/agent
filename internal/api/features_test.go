package api

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uptimy/agent/internal/monitor"
)

func TestAPITokens(t *testing.T) {
	c, st := newTestServerWithStore(t)
	c.login("admin", adminPassword)
	createTCP(t, st, "DB", "db:5432", monitor.SourceUI)

	code, created := c.do("POST", "/api/auth/tokens", map[string]any{"name": "Terraform"})
	token, _ := created["token"].(string)
	if code != 201 || !strings.HasPrefix(token, "upa_") {
		t.Fatalf("create: %d %v", code, created)
	}
	readCode, readOnly := c.do("POST", "/api/auth/tokens", map[string]any{"name": "Grafana", "read_only": true})
	if readCode != 201 {
		t.Fatalf("create read-only: %d", readCode)
	}
	if _, list := c.doList("GET", "/api/auth/tokens"); len(list) != 2 || list[0]["hint"] == nil || list[0]["token"] != nil {
		t.Fatalf("list must not include the secret: %v", list)
	}

	full := c.withToken(token)
	if code, list := full.doList("GET", "/api/healthchecks"); code != 200 || len(list) != 1 {
		t.Fatalf("read with token: %d %v", code, list)
	}
	if code, _ := full.do("POST", "/api/heartbeats", map[string]any{"name": "Job", "heartbeat": map[string]any{"every_seconds": 3600}}); code != 201 {
		t.Fatalf("write with token: %d", code)
	}
	// Tokens can't manage accounts or mint more tokens.
	for _, path := range []string{"/api/auth/tokens", "/api/users"} {
		if code, _ := full.do("POST", path, map[string]any{"name": "x", "username": "x", "password": "longenough", "role": "admin"}); code != 403 {
			t.Fatalf("%s with a token: %d", path, code)
		}
	}

	ro := c.withToken(readOnly["token"].(string))
	if code, _ := ro.doList("GET", "/api/heartbeats"); code != 200 {
		t.Fatalf("read-only read: %d", code)
	}
	if code, _ := ro.do("POST", "/api/heartbeats", map[string]any{"name": "Job 2", "heartbeat": map[string]any{"every_seconds": 3600}}); code != 403 {
		t.Fatalf("read-only write: %d", code)
	}

	id := strconv.Itoa(int(created["api_token"].(map[string]any)["id"].(float64)))
	if code, _ := c.do("DELETE", "/api/auth/tokens/"+id, nil); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ := full.doList("GET", "/api/healthchecks"); code != 401 {
		t.Fatalf("revoked token still works: %d", code)
	}
	if code, _ := c.withToken("upa_made-up").doList("GET", "/api/healthchecks"); code != 401 {
		t.Fatalf("made-up token: %d", code)
	}
}

func TestAlertRoutingAPI(t *testing.T) {
	c, st := newTestServerWithStore(t)
	c.login("admin", adminPassword)
	api := createTCP(t, st, "API", "api:80", monitor.SourceUI)

	// Without a choice, a new channel alerts for every monitor.
	_, slack := c.do("POST", "/api/notifiers", map[string]any{"name": "Slack", "type": "slack", "enabled": true,
		"config": map[string]any{"url": "https://hooks.slack.com/x"}})
	if slack["all_monitors"] != true {
		t.Fatalf("default: %v", slack)
	}
	code, pager := c.do("POST", "/api/notifiers", map[string]any{"name": "Pager", "type": "webhook", "enabled": true,
		"all_monitors": false, "monitor_ids": []int64{}, "config": map[string]any{"url": "https://example.com/hook"}})
	if code != 201 || pager["all_monitors"] != false {
		t.Fatalf("limited: %d %v", code, pager)
	}

	// Add the monitor to Pager from the monitor's side.
	pagerID := pager["id"].(float64)
	id := strconv.FormatInt(api.ID, 10)
	if code, _ := c.do("PUT", "/api/healthchecks/"+id, map[string]any{"name": "API", "paused": true,
		"check": map[string]any{"type": "tcp", "target": "api:80"}, "notifier_ids": []float64{pagerID}}); code != 200 {
		t.Fatalf("save routing: %d", code)
	}
	_, detail := c.do("GET", "/api/healthchecks/"+id, nil)
	if ids := detail["notifier_ids"].([]any); len(ids) != 1 || ids[0] != pagerID {
		t.Fatalf("detail routing: %v", detail["notifier_ids"])
	}
	// Saving without notifier_ids leaves routing alone.
	c.do("PUT", "/api/healthchecks/"+id, map[string]any{"name": "API 2", "paused": true,
		"check": map[string]any{"type": "tcp", "target": "api:80"}})
	if _, detail := c.do("GET", "/api/healthchecks/"+id, nil); len(detail["notifier_ids"].([]any)) != 1 {
		t.Fatal("routing lost on a save without it")
	}

	if code, _ := c.do("POST", "/api/notifiers", map[string]any{"name": "Bad", "type": "webhook", "all_monitors": false,
		"monitor_ids": []int{9999}, "config": map[string]any{"url": "https://example.com/hook"}}); code != 400 {
		t.Fatalf("unknown monitor: %d", code)
	}
}

func TestMaintenanceAPI(t *testing.T) {
	c, st := newTestServerWithStore(t)
	c.login("admin", adminPassword)
	db := createTCP(t, st, "Database", "db:5432", monitor.SourceUI)
	createTCP(t, st, "Website", "web:443", monitor.SourceUI)

	now := time.Now().UTC()
	code, w := c.do("POST", "/api/maintenance", map[string]any{
		"title": "Database upgrade", "description": "Postgres 17", "public": true,
		"starts_at": now.Add(-time.Minute), "ends_at": now.Add(time.Hour), "monitor_ids": []int64{db.ID},
	})
	if code != 201 || w["state"] != "active" {
		t.Fatalf("create: %d %v", code, w)
	}
	c.do("POST", "/api/maintenance", map[string]any{
		"title": "Network work", "public": true, "all_monitors": true,
		"starts_at": now.Add(48 * time.Hour), "ends_at": now.Add(50 * time.Hour),
	})
	for name, body := range map[string]map[string]any{
		"no monitors": {"title": "x", "starts_at": now, "ends_at": now.Add(time.Hour)},
		"backwards":   {"title": "x", "all_monitors": true, "starts_at": now, "ends_at": now.Add(-time.Hour)},
		"over":        {"title": "x", "all_monitors": true, "starts_at": now.Add(-2 * time.Hour), "ends_at": now.Add(-time.Hour)},
	} {
		if code, _ := c.do("POST", "/api/maintenance", body); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}

	if _, list := c.doList("GET", "/api/healthchecks"); list[0]["name"] != "Database" || list[0]["in_maintenance"] != true || list[1]["in_maintenance"] != false {
		t.Fatalf("summaries: %v", list)
	}

	code, text := pingURL(t, c, "/api/status", "")
	if code != 200 || !strings.Contains(text, `"overall":"maintenance"`) || !strings.Contains(text, `"Database upgrade"`) ||
		!strings.Contains(text, `"Network work"`) || !strings.Contains(text, `"in_maintenance":true`) {
		t.Fatalf("status page: %s", text)
	}

	id := strconv.Itoa(int(w["id"].(float64)))
	if code, ended := c.do("POST", "/api/maintenance/"+id+"/end", nil); code != 200 || ended["state"] != "ended" {
		t.Fatalf("end: %d %v", code, ended)
	}
	if code, _ := c.do("POST", "/api/maintenance/"+id+"/end", nil); code != 409 {
		t.Fatalf("end twice: %d", code)
	}
	if _, text := pingURL(t, c, "/api/status", ""); strings.Contains(text, `"overall":"maintenance"`) {
		t.Fatalf("still in maintenance: %s", text)
	}
}
