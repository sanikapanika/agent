package api

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// pingURL sends a ping like a cron job would: plain HTTP, no session.
func pingURL(t *testing.T, c *client, path, body string) (int, string) {
	t.Helper()
	method := http.MethodGet
	if body != "" {
		method = http.MethodPost
	}
	req, _ := http.NewRequest(method, c.base+path, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestHeartbeatLifecycle(t *testing.T) {
	c := newTestServer(t)
	c.login("admin", adminPassword)

	// The form previews a schedule as it's typed.
	_, preview := c.do("POST", "/api/heartbeats/preview", map[string]any{"cron": "0 3 * * *", "timezone": "Europe/Berlin"})
	if preview["schedule"] != "0 3 * * * (Europe/Berlin)" || len(preview["upcoming"].([]any)) != 3 {
		t.Fatalf("preview: %v", preview)
	}
	if _, bad := c.do("POST", "/api/heartbeats/preview", map[string]any{"cron": "every day"}); bad["error"] == nil {
		t.Fatalf("invalid cron previewed: %v", bad)
	}

	code, hb := c.do("POST", "/api/heartbeats", map[string]any{
		"name": "Nightly backup", "heartbeat": map[string]any{"cron": "0 3 * * *", "timezone": "Europe/Berlin"},
	})
	if code != 201 || hb["kind"] != "heartbeat" {
		t.Fatalf("create: %d %v", code, hb)
	}
	id := strconv.Itoa(int(hb["id"].(float64)))
	spec := hb["heartbeat"].(map[string]any)
	token := spec["token"].(string)
	if len(token) < 16 || spec["grace_seconds"] != float64(300) {
		t.Fatalf("defaults: %v", spec)
	}

	// Every ping style.
	steps := []struct {
		path, body string
		want       int
	}{
		{"/ping/" + token + "/start", "", 200},
		{"/ping/" + token, "", 200},
		{"/ping/" + token + "/fail", "disk full", 200},
		{"/ping/" + token + "/0", "", 200},
		{"/ping/" + token + "/3", "", 200},
		{"/ping/" + token + "/300", "", 404},
		{"/ping/nobody-has-this-token", "", 404},
	}
	for _, s := range steps {
		if code, text := pingURL(t, c, s.path, s.body); code != s.want {
			t.Fatalf("%s: %d %q", s.path, code, text)
		}
		time.Sleep(20 * time.Millisecond) // pings are applied in order by the heartbeat's goroutine
	}

	waitFor(t, "four runs", func() bool {
		_, d := c.do("GET", "/api/heartbeats/"+id, nil)
		st := d["heartbeat"].(map[string]any)["stats_30d"].(map[string]any)
		return st["runs"] == float64(4)
	})
	_, detail := c.do("GET", "/api/heartbeats/"+id, nil)
	sum := detail["heartbeat"].(map[string]any)
	if sum["schedule"] != "0 3 * * * (Europe/Berlin)" || sum["tracking"].(map[string]any)["state"] != "failed" || sum["status"] != "down" {
		t.Fatalf("summary: %v", sum)
	}
	stats := detail["stats"].(map[string]any)["30d"].(map[string]any)
	if stats["on_time"] != float64(2) || stats["failed"] != float64(2) {
		t.Fatalf("stats: %v", stats)
	}
	if last := sum["last_run"].(map[string]any); last["message"] != "exit code 3" {
		t.Fatalf("last run: %v", last)
	}
	if len(detail["daily"].([]any)) != 30 || len(detail["upcoming"].([]any)) != 3 {
		t.Fatalf("detail: %v", detail)
	}
	if next := detail["upcoming"].([]any)[0]; next != sum["tracking"].(map[string]any)["due_at"] {
		t.Fatalf("the first upcoming run %v isn't the one being waited for", next)
	}
	code, runs := c.doList("GET", "/api/heartbeats/"+id+"/runs")
	if code != 200 || len(runs) != 4 || runs[0]["duration_ms"] == nil || runs[1]["message"] != "disk full" {
		t.Fatalf("runs: %d %v", code, runs)
	}

	// Editing keeps the ping URL; rotating replaces it.
	if code, _ := c.do("PUT", "/api/heartbeats/"+id, map[string]any{"name": "Backup", "heartbeat": map[string]any{"every_seconds": 86400}}); code != 200 {
		t.Fatalf("edit: %d", code)
	}
	if code, _ := pingURL(t, c, "/ping/"+token, ""); code != 200 {
		t.Fatalf("token changed on edit: %d", code)
	}
	waitFor(t, "an interval's next run", func() bool {
		_, d := c.do("GET", "/api/heartbeats/"+id, nil)
		due := d["heartbeat"].(map[string]any)["tracking"].(map[string]any)["due_at"]
		return d["upcoming"].([]any)[0] == due && d["heartbeat"].(map[string]any)["schedule"] == "every day"
	})
	_, rotated := c.do("POST", "/api/heartbeats/"+id+"/token", nil)
	if newToken := rotated["heartbeat"].(map[string]any)["token"]; newToken == token {
		t.Fatal("token not rotated")
	}
	if code, _ := pingURL(t, c, "/ping/"+token, ""); code != 404 {
		t.Fatalf("old ping URL still works: %d", code)
	}

	// On the status page, it shows the on-time rate, not targets or tokens.
	hbID, _ := strconv.ParseInt(id, 10, 64)
	if code, body := c.do("PUT", "/api/status-page", map[string]any{
		"enabled": true, "title": "Status", "sections": []map[string]any{{"id": "jobs", "name": "Jobs"}},
		"monitors": []map[string]any{{"id": hbID, "public": true, "section": "jobs"}},
	}); code != 200 {
		t.Fatalf("add to status page: %d %v", code, body)
	}
	code, text := pingURL(t, c, "/api/status", "")
	if code != 200 || !strings.Contains(text, `"kind":"heartbeat"`) || strings.Contains(text, "token") {
		t.Fatalf("status page: %d %s", code, text)
	}

	// Editing the heartbeat keeps it on the page: the page owns that.
	c.do("PUT", "/api/heartbeats/"+id, map[string]any{"name": "Nightly DB backup", "heartbeat": map[string]any{"every_seconds": 86400}})
	if _, text := pingURL(t, c, "/api/status", ""); !strings.Contains(text, `"Nightly DB backup"`) {
		t.Fatalf("taken off the status page by an edit: %s", text)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
