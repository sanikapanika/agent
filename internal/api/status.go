package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/uptimy/agent/internal/maintenance"
	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/statuspage"
	"github.com/uptimy/agent/internal/store"
)

const statusPageDays = 30

// publicMonitor deliberately omits the target: in-cluster targets are
// internal hostnames that shouldn't leak onto a public page.
type publicMonitor struct {
	Name       string         `json:"name"`
	Kind       monitor.Kind   `json:"kind"`
	TypeLabel  string         `json:"type_label"`
	Status     monitor.Status `json:"status"`
	LastChange *time.Time     `json:"last_change"`
	// Ratio is how well it did over the period: a healthcheck's uptime, or
	// the share of a heartbeat's runs that succeeded on time. Days is the
	// same per day.
	Ratio *float64      `json:"ratio"`
	Days  []store.Daily `json:"days"`
	// LastRun is when a heartbeat's job last finished.
	LastRun *time.Time `json:"last_run,omitempty"`
	// InMaintenance is set during a public maintenance window.
	InMaintenance bool `json:"in_maintenance,omitempty"`
}

// publicMaintenance announces planned work. Monitors are the public names of
// those it covers; empty means all of them.
type publicMaintenance struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Active      bool      `json:"active"`
	Monitors    []string  `json:"monitors"`
}

// maintenanceNotice is how far ahead the status page announces maintenance.
const maintenanceNotice = 7 * 24 * time.Hour

type publicSection struct {
	Name     string          `json:"name"`
	Monitors []publicMonitor `json:"monitors"`
}

type publicIncident struct {
	Monitor string         `json:"monitor"`
	Status  monitor.Status `json:"status"`
	Time    time.Time      `json:"time"`
}

func (s *Server) publicStatus(w http.ResponseWriter, r *http.Request) {
	settings, err := s.Store.StatusPage(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !settings.Enabled {
		writeError(w, http.StatusNotFound, "status page is disabled")
		return
	}
	monitors, err := s.Store.ListMonitors(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	since := time.Now().Add(-statusPageDays * 24 * time.Hour)
	names := map[int64]string{}
	bySection := map[string][]publicMonitor{}
	overall := "operational"
	for _, m := range statuspage.Ordered(monitors) {
		if !m.Public {
			continue
		}
		names[m.ID] = m.PublicName()
		pm := publicMonitor{Name: m.PublicName(), Kind: m.Kind, TypeLabel: typeLabel(m), Status: s.Scheduler.Status(m)}
		if e, err := s.Store.LastEvent(r.Context(), m.ID); err == nil {
			pm.LastChange = &e.Time
		} else if !store.IsNotFound(err) {
			s.internalError(w, r, err)
			return
		}
		if err := s.publicHistory(r, m, since, &pm); err != nil {
			s.internalError(w, r, err)
			return
		}
		pm.InMaintenance = s.publicMaintenanceCovers(m.ID)
		// Same rule as the hosted Uptimy status pages: anything down is an
		// outage, unless it's down for announced maintenance.
		if pm.Status == monitor.StatusDown && !pm.InMaintenance {
			overall = "outage"
		}
		sec := settings.SectionOf(m)
		bySection[sec] = append(bySection[sec], pm)
	}
	// Sections in the editor's order; empty ones aren't shown.
	sections := []publicSection{}
	for _, sec := range settings.Sections {
		if ms := bySection[sec.ID]; len(ms) > 0 {
			sections = append(sections, publicSection{Name: sec.Name, Monitors: ms})
		}
	}
	logos, err := s.statusPageLogos(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	events, err := s.Store.ListEvents(r.Context(), 0, 200)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	// Events are newest first. Show every "down", and an "up" only when it
	// ends an outage; the first successful check after startup isn't news.
	// Walking newest→oldest, an up is a recovery if the next older
	// up/down event for that monitor is a down.
	incidents := []publicIncident{}
	if !settings.ShowEvents {
		events = nil
	}
	pendingUp := map[int64]int{} // monitor → index of an up awaiting its predecessor
	for _, e := range events {
		name, ok := names[e.MonitorID]
		if !ok || (e.Status != monitor.StatusDown && e.Status != monitor.StatusUp) {
			continue
		}
		if i, ok := pendingUp[e.MonitorID]; ok && e.Status == monitor.StatusUp {
			incidents[i].Status = "" // previous event was up too: not a recovery
		}
		delete(pendingUp, e.MonitorID)
		incidents = append(incidents, publicIncident{Monitor: name, Status: e.Status, Time: e.Time})
		if e.Status == monitor.StatusUp {
			pendingUp[e.MonitorID] = len(incidents) - 1
		}
	}
	for _, i := range pendingUp {
		incidents[i].Status = "" // no older down: first check, not a recovery
	}
	shown := []publicIncident{}
	for _, inc := range incidents {
		if inc.Status != "" {
			shown = append(shown, inc)
		}
		if len(shown) == 20 {
			break
		}
	}

	notices := s.publicMaintenance(names)
	if overall == "operational" {
		for _, n := range notices {
			if n.Active {
				overall = "maintenance"
			}
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, map[string]any{
		"title":        settings.Title,
		"description":  settings.Description,
		"logos":        logos,
		"accent_color": settings.AccentColor,
		"website_url":  settings.WebsiteURL,
		"overall":      overall,
		"maintenance":  notices,
		"sections":     sections,
		"incidents":    shown,
		"updated":      time.Now().UTC(),
	})
}

// stream pushes live updates to the UI over Server-Sent Events.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx, some PaaS routers)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	msgs, unsubscribe := s.Hub.Subscribe()
	defer unsubscribe()
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": ping\n\n")
		case m, ok := <-msgs:
			if !ok {
				return
			}
			data, err := json.Marshal(m)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		flusher.Flush()
	}
}

// publicHistory fills in how a monitor did over the period, in the measure
// that fits its kind.
func (s *Server) publicHistory(r *http.Request, m monitor.Monitor, since time.Time, pm *publicMonitor) error {
	ctx := r.Context()
	var err error
	if m.Kind == monitor.KindHeartbeat {
		if pm.Days, err = s.Store.DailyOnTime(ctx, m.ID, statusPageDays); err != nil {
			return err
		}
		stats, err := s.Store.RunStatsSince(ctx, m.ID, since)
		if err != nil {
			return err
		}
		pm.Ratio = stats.Ratio
		if last, err := s.Store.LastRun(ctx, m.ID); err == nil {
			pm.LastRun = last.FinishedAt
		} else if !store.IsNotFound(err) {
			return err
		}
		return nil
	}
	if pm.Days, err = s.Store.DailyUptimes(ctx, m.ID, statusPageDays); err != nil {
		return err
	}
	u, err := s.Store.UptimeSince(ctx, m.ID, since)
	pm.Ratio = u.Ratio
	return err
}

// publicMaintenanceCovers reports whether a public window covers a monitor
// now. Private windows only silence alerts.
func (s *Server) publicMaintenanceCovers(monitorID int64) bool {
	for _, w := range s.Scheduler.Maintenance().Covering(monitorID, time.Now()) {
		if w.Public {
			return true
		}
	}
	return false
}

// publicMaintenance lists public windows that are active or start within
// maintenanceNotice and cover a monitor on the page. names maps the page's
// monitors to their public names.
func (s *Server) publicMaintenance(names map[int64]string) []publicMaintenance {
	now := time.Now()
	out := []publicMaintenance{}
	for _, w := range s.Scheduler.Maintenance() {
		state := w.StateAt(now)
		if !w.Public || state == maintenance.Ended || w.StartsAt.Sub(now) > maintenanceNotice {
			continue
		}
		notice := publicMaintenance{
			Title: w.Title, Description: w.Description, StartsAt: w.StartsAt, EndsAt: w.EndsAt,
			Active: state == maintenance.Active, Monitors: []string{},
		}
		if !w.AllMonitors {
			for _, id := range w.MonitorIDs {
				if name, ok := names[id]; ok {
					notice.Monitors = append(notice.Monitors, name)
				}
			}
			if len(notice.Monitors) == 0 {
				continue // covers nothing on the page
			}
		}
		out = append(out, notice)
	}
	return out
}
