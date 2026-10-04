// Package kuma reads an Uptime Kuma database (kuma.db, versions 1 and 2) and
// turns its monitors and notifications into the agent's, so switching
// doesn't mean re-entering everything. What can't be carried over is listed
// with the reason, rather than imported half-working.
package kuma

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // the same pure-Go driver the agent uses

	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/notify"
	"github.com/uptimy/agent/internal/statuspage"
)

// Plan is what a Kuma database becomes. Nothing is created until it's
// applied, so people can review it first.
type Plan struct {
	// Sections are the groups of Kuma's status pages, in their order. Each
	// monitor on a page has its section's ID in StatusSection and its place
	// in StatusOrder.
	Sections  []statuspage.Section `json:"sections"`
	Monitors  []Monitor            `json:"monitors"`
	Notifiers []Notifier           `json:"notifiers"`
	Skipped   []Skipped            `json:"skipped"`
}

// Monitor is a Kuma monitor as a healthcheck or heartbeat.
type Monitor struct {
	KumaID  int64           `json:"kuma_id"`
	Monitor monitor.Monitor `json:"monitor"`
	// Notes say what didn't carry over exactly.
	Notes []string `json:"notes"`
}

// Notifier is a Kuma notification as an alert channel. KumaMonitorIDs are
// the Kuma monitors it alerted for; the agent's IDs exist once they're
// created.
type Notifier struct {
	KumaID         int64           `json:"kuma_id"`
	Notifier       notify.Notifier `json:"notifier"`
	KumaMonitorIDs []int64         `json:"kuma_monitor_ids"`
	Notes          []string        `json:"notes"`
}

// Skipped is something that wasn't converted, and why.
type Skipped struct {
	What   string `json:"what"` // "monitor" or "notification"
	Name   string `json:"name"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// Read opens a kuma.db file read-only and plans its import. Stop Uptime Kuma
// first, or copy the file while it's stopped, so the copy is consistent.
func Read(ctx context.Context, path string) (Plan, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	if err != nil {
		return Plan{}, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	monitors, err := query(ctx, db, "SELECT * FROM monitor ORDER BY id")
	if err != nil {
		return Plan{}, fmt.Errorf("this doesn't look like an Uptime Kuma database: %w", err)
	}
	onStatusPage, err := idSet(ctx, db, "SELECT DISTINCT monitor_id FROM monitor_group")
	if err != nil {
		return Plan{}, err
	}
	sections, placement := statusPageGroups(ctx, db)
	plan := Plan{Sections: sections, Monitors: []Monitor{}, Notifiers: []Notifier{}, Skipped: []Skipped{}}
	imported := map[int64]bool{}
	for _, r := range monitors {
		m, notes, reason := convertMonitor(r)
		if reason == "" {
			m.Public = onStatusPage[r.int("id")]
			if p, ok := placement[r.int("id")]; ok {
				m.StatusSection, m.StatusOrder = p.section, p.order
			}
			if err := m.Normalize(); err != nil {
				reason = err.Error()
			}
		}
		if reason != "" {
			plan.Skipped = append(plan.Skipped, Skipped{What: "monitor", Name: r.str("name"), Type: r.str("type"), Reason: reason})
			continue
		}
		imported[r.int("id")] = true
		plan.Monitors = append(plan.Monitors, Monitor{KumaID: r.int("id"), Monitor: m, Notes: notes})
	}

	notifications, err := query(ctx, db, "SELECT * FROM notification ORDER BY id")
	if err != nil {
		return Plan{}, err
	}
	links, err := optional(ctx, db, "SELECT monitor_id, notification_id FROM monitor_notification")
	if err != nil {
		return Plan{}, err
	}
	for _, r := range notifications {
		var cfg map[string]any
		if err := json.Unmarshal([]byte(r.str("config")), &cfg); err != nil {
			plan.Skipped = append(plan.Skipped, Skipped{What: "notification", Name: r.str("name"), Reason: "its settings couldn't be read"})
			continue
		}
		n, notes, reason := convertNotification(row(cfg))
		n.Name, n.Enabled = r.str("name"), r.bool("active")
		var kumaMonitors []int64
		for _, l := range links {
			if l.int("notification_id") == r.int("id") && imported[l.int("monitor_id")] {
				kumaMonitors = append(kumaMonitors, l.int("monitor_id"))
			}
		}
		// Used by every imported monitor: alert for every monitor, including
		// ones added later, like Kuma's "apply on all existing monitors".
		n.AllMonitors = len(kumaMonitors) > 0 && len(kumaMonitors) == len(imported)
		if len(kumaMonitors) == 0 {
			notes = append(notes, "wasn't used by any imported monitor; choose its monitors after importing")
		}
		if reason == "" {
			if err := n.Normalize(); err != nil {
				reason = err.Error()
			}
		}
		if reason != "" {
			plan.Skipped = append(plan.Skipped, Skipped{What: "notification", Name: r.str("name"), Type: row(cfg).str("type"), Reason: reason})
			continue
		}
		if n.AllMonitors {
			kumaMonitors = nil
		}
		plan.Notifiers = append(plan.Notifiers, Notifier{KumaID: r.int("id"), Notifier: n, KumaMonitorIDs: kumaMonitors, Notes: notes})
	}
	return plan, nil
}

type place struct {
	section string
	order   int
}

// statusPageGroups reads the groups of Kuma's status pages as sections, in
// page and group order (same-named groups on different pages become one),
// and where each monitor goes: the first group it's in. A database without
// status page groups (or an old layout) gives none, and monitors land in the
// page's first section.
func statusPageGroups(ctx context.Context, db *sql.DB) ([]statuspage.Section, map[int64]place) {
	rows, err := query(ctx, db, `SELECT g.name, mg.monitor_id FROM monitor_group mg JOIN "group" g ON g.id = mg.group_id
		ORDER BY g.status_page_id, g.weight, g.id, mg.weight, mg.id`)
	sections := []statuspage.Section{}
	placement := map[int64]place{}
	if err != nil {
		return sections, placement
	}
	byName := map[string]string{} // lowercase name → section ID
	used := map[string]bool{}
	for _, r := range rows {
		name := strings.TrimSpace(r.str("name"))
		if name == "" {
			name = "Services"
		}
		if len(name) > 60 {
			name = name[:60]
		}
		id, ok := byName[strings.ToLower(name)]
		if !ok {
			id = sectionID(name, used)
			byName[strings.ToLower(name)] = id
			sections = append(sections, statuspage.Section{ID: id, Name: name})
		}
		if _, placed := placement[r.int("monitor_id")]; !placed {
			placement[r.int("monitor_id")] = place{section: id, order: len(placement) + 1}
		}
	}
	return sections, placement
}

// sectionID makes a unique section ID from a name: "Core APIs" is "core-apis".
func sectionID(name string, used map[string]bool) string {
	var b strings.Builder
	for _, c := range strings.ToLower(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	base := strings.Trim(b.String(), "-")
	if len(base) > 30 {
		base = strings.Trim(base[:30], "-")
	}
	if base == "" {
		base = "section"
	}
	id := base
	for i := 2; used[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	used[id] = true
	return id
}

// row is a database row, or a Kuma notification's settings, by column name.
// Kuma added columns over the years, so absent ones read as empty.
type row map[string]any

func (r row) str(k string) string {
	switch v := r[k].(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	case float64:
		return fmt.Sprint(int64(v))
	default:
		return fmt.Sprint(v)
	}
}

func (r row) int(k string) int64 {
	switch v := r[k].(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	case bool:
		if v {
			return 1
		}
	}
	return 0
}

func (r row) bool(k string) bool {
	if b, ok := r[k].(bool); ok {
		return b
	}
	return r.int(k) != 0 || r.str(k) == "true"
}

// optional runs a query on a table older Kuma versions may not have.
func optional(ctx context.Context, db *sql.DB, q string) ([]row, error) {
	rows, err := query(ctx, db, q)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil, nil
	}
	return rows, err
}

func query(ctx context.Context, db *sql.DB, q string) ([]row, error) {
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []row
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := row{}
		for i, c := range cols {
			r[c] = vals[i]
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func idSet(ctx context.Context, db *sql.DB, q string) (map[int64]bool, error) {
	rows, err := optional(ctx, db, q)
	out := map[int64]bool{}
	for _, r := range rows {
		for _, v := range r {
			out[row{"v": v}.int("v")] = true
		}
	}
	return out, err
}

func validURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
