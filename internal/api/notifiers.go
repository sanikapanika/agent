package api

import (
	"context"
	"net/http"
	"time"

	"github.com/uptimy/agent/internal/notify"
)

func (s *Server) listNotifiers(w http.ResponseWriter, r *http.Request) {
	ns, err := s.Store.ListNotifiers(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !currentUser(r).IsAdmin() {
		// Webhook URLs and bot tokens are credentials; viewers see names only.
		for i := range ns {
			ns[i].Config = notify.Config{}
		}
	}
	writeJSON(w, http.StatusOK, ns)
}

type notifierInput struct {
	Name    string        `json:"name"`
	Type    string        `json:"type"`
	Enabled bool          `json:"enabled"`
	Config  notify.Config `json:"config"`
	// AllMonitors defaults to true, so a notifier made without choosing
	// alerts for everything rather than for nothing.
	AllMonitors *bool   `json:"all_monitors"`
	MonitorIDs  []int64 `json:"monitor_ids"`
}

func (in notifierInput) notifier(id int64) notify.Notifier {
	all := in.AllMonitors == nil || *in.AllMonitors
	return notify.Notifier{ID: id, Name: in.Name, Type: in.Type, Enabled: in.Enabled, Config: in.Config, AllMonitors: all, MonitorIDs: in.MonitorIDs}
}

func (s *Server) createNotifier(w http.ResponseWriter, r *http.Request) {
	var in notifierInput
	if !decode(w, r, &in) {
		return
	}
	n := in.notifier(0)
	if err := n.Normalize(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := s.Store.CreateNotifier(r.Context(), n)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) updateNotifier(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in notifierInput
	if !decode(w, r, &in) {
		return
	}
	n := in.notifier(id)
	if err := n.Normalize(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := s.Store.UpdateNotifier(r.Context(), n)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) deleteNotifier(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteNotifier(r.Context(), id); err != nil {
		s.storeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testNotifier(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	n, err := s.Store.GetNotifier(r.Context(), id)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	err = s.Sender.Send(ctx, n, notify.Alert{
		MonitorName: "Test monitor", Kind: "healthcheck", Target: "uptimy-agent", Status: "down",
		Message: "this is a test notification from Uptimy Agent", Time: time.Now().UTC(), Test: true,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
