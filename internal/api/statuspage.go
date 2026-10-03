package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/statuspage"
	"github.com/uptimy/agent/internal/store"
)

type statusPageMonitor struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`
	Kind      monitor.Kind `json:"kind"`
	TypeLabel string       `json:"type_label"`
	Source    string       `json:"source"`
	Public    bool         `json:"public"`
	Label     string       `json:"label"`
	Section   string       `json:"section"`
}

type statusPageConfig struct {
	statuspage.Settings
	Logos    statusPageLogos     `json:"logos"`
	Monitors []statusPageMonitor `json:"monitors"`
}

func (s *Server) statusPageConfig(ctx context.Context) (statusPageConfig, error) {
	settings, err := s.Store.StatusPage(ctx)
	if err != nil {
		return statusPageConfig{}, err
	}
	monitors, err := s.Store.ListMonitors(ctx)
	if err != nil {
		return statusPageConfig{}, err
	}
	logos, err := s.statusPageLogos(ctx)
	if err != nil {
		return statusPageConfig{}, err
	}
	cfg := statusPageConfig{Settings: settings, Logos: logos, Monitors: []statusPageMonitor{}}
	for _, m := range statuspage.Ordered(monitors) {
		cfg.Monitors = append(cfg.Monitors, statusPageMonitor{
			ID: m.ID, Name: m.Name, Kind: m.Kind, TypeLabel: typeLabel(m), Source: m.Source, Public: m.Public, Label: m.StatusLabel,
			Section: settings.SectionOf(m),
		})
	}
	return cfg, nil
}

func (s *Server) getStatusPageConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.statusPageConfig(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// saveStatusPageConfig stores the settings and the monitor layout. The order
// of `monitors` is the display order.
func (s *Server) saveStatusPageConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		statuspage.Settings
		Monitors []store.StatusPageEntry `json:"monitors"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.Domain != "" && in.Domain == statuspage.NormalizeHost(r.Host) {
		// It would serve only the status page, signing everyone out here.
		writeError(w, http.StatusBadRequest, "that's the address you're using for the dashboard; the status page needs its own, e.g. status.example.com")
		return
	}
	existing, err := s.Store.ListMonitors(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	exists := map[int64]bool{}
	for _, m := range existing {
		exists[m.ID] = true
	}
	for i, e := range in.Monitors {
		if !exists[e.ID] {
			writeError(w, http.StatusBadRequest, "unknown monitor in layout (it may have been deleted); reload and try again")
			return
		}
		in.Monitors[i].Label = strings.TrimSpace(e.Label)
		// A monitor is on the page in one of its sections, or not at all.
		if !in.HasSection(e.Section) {
			in.Monitors[i].Public = false
		}
		if len(in.Monitors[i].Label) > 100 {
			writeError(w, http.StatusBadRequest, "keep display names under 100 characters")
			return
		}
	}

	if err := s.Store.SaveStatusPage(r.Context(), in.Settings, in.Monitors); err != nil {
		s.storeError(w, r, err)
		return
	}
	s.setStatusDomain(in.Domain)
	s.monitorsChanged(0)
	s.getStatusPageConfig(w, r)
}
