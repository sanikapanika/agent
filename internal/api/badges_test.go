package api

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uptimy/agent/internal/monitor"
)

func TestBadges(t *testing.T) {
	c, st := newTestServerWithStore(t)
	api := createTCP(t, st, "API", "api:1", monitor.SourceUI)
	private := createPrivateTCP(t, st, "Internal", "db:1")
	now := time.Now()
	for i := range 1000 {
		st.InsertResult(t.Context(), monitor.Result{MonitorID: api.ID, Time: now.Add(-time.Duration(i) * time.Minute), OK: i != 0})
	}

	get := func(path string) (int, string) {
		t.Helper()
		resp, err := http.Get(c.base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
			t.Fatalf("%s: content type %q", path, ct)
		}
		return resp.StatusCode, string(body)
	}
	id := strconv.FormatInt(api.ID, 10)

	if code, svg := get("/badge/" + id + "/status.svg"); code != 200 || !strings.Contains(svg, "<title>API: paused</title>") {
		t.Fatalf("status: %d %s", code, svg)
	}
	if code, svg := get("/badge/" + id + "/uptime.svg?period=24h"); code != 200 || !strings.Contains(svg, "uptime 24h: 99.9%") {
		t.Fatalf("uptime: %d %s", code, svg)
	}
	if code, svg := get("/badge/" + id + "/uptime.svg"); code != 200 || !strings.Contains(svg, "uptime 30d: 99.9%") {
		t.Fatalf("default period: %d %s", code, svg)
	}
	if code, _ := get("/badge/" + id + "/uptime.svg?period=1y"); code != 400 {
		t.Fatalf("bad period: %d", code)
	}
	if code, svg := get("/badge/" + id + "/status.svg?label=" + "%3Cscript%3E"); code != 200 || strings.Contains(svg, "<script>") || !strings.Contains(svg, "&lt;script&gt;") {
		t.Fatalf("label not escaped: %s", svg)
	}
	if code, svg := get("/badge/status.svg"); code != 200 || !strings.Contains(svg, "status: operational") {
		t.Fatalf("overall: %d %s", code, svg)
	}

	// Monitors that aren't on the page, and ones that don't exist, look the same.
	for _, path := range []string{"/badge/" + strconv.FormatInt(private.ID, 10) + "/status.svg", "/badge/999/uptime.svg", "/badge/x/status.svg"} {
		if code, svg := get(path); code != 404 || !strings.Contains(svg, "not found") {
			t.Errorf("%s: %d %s", path, code, svg)
		}
	}

	// No badges while the status page is off.
	sp, _ := st.StatusPage(t.Context())
	sp.Enabled = false
	if err := st.SaveStatusPage(t.Context(), sp, nil); err != nil {
		t.Fatal(err)
	}
	if code, _ := get("/badge/" + id + "/status.svg"); code != 404 {
		t.Fatalf("page off: %d", code)
	}
}

func TestFormatPercent(t *testing.T) {
	for in, want := range map[float64]string{1: "100%", 0.99999: "99.99%", 0.995: "99.5%", 0: "0%"} {
		if got := formatPercent(in); got != want {
			t.Errorf("formatPercent(%v) = %q, want %q", in, got, want)
		}
	}
}
