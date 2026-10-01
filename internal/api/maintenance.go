package api

import (
	"net/http"
	"time"

	"github.com/uptimy/agent/internal/maintenance"
)

// maintenanceHistory is how long ended windows stay listed.
const maintenanceHistory = 14 * 24 * time.Hour

type maintenanceItem struct {
	maintenance.Window
	State maintenance.State `json:"state"`
}

func (s *Server) listMaintenance(w http.ResponseWriter, r *http.Request) {
	ws, err := s.Store.ListMaintenance(r.Context(), time.Now().Add(-maintenanceHistory))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	now := time.Now()
	out := make([]maintenanceItem, len(ws))
	for i, mw := range ws {
		out[i] = maintenanceItem{Window: mw, State: mw.StateAt(now)}
	}
	writeJSON(w, http.StatusOK, out)
}

type maintenanceInput struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	AllMonitors bool      `json:"all_monitors"`
	MonitorIDs  []int64   `json:"monitor_ids"`
	Public      bool      `json:"public"`
}

func (in maintenanceInput) window(id int64) maintenance.Window {
	return maintenance.Window{
		ID: id, Title: in.Title, Description: in.Description, StartsAt: in.StartsAt, EndsAt: in.EndsAt,
		AllMonitors: in.AllMonitors, MonitorIDs: in.MonitorIDs, Public: in.Public,
	}
}

func (s *Server) createMaintenance(w http.ResponseWriter, r *http.Request) {
	var in maintenanceInput
	if !decode(w, r, &in) {
		return
	}
	mw := in.window(0)
	if err := mw.Normalize(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !mw.EndsAt.After(time.Now()) {
		writeError(w, http.StatusBadRequest, "that maintenance would already be over; choose an end in the future")
		return
	}
	mw, err := s.Store.CreateMaintenance(r.Context(), mw)
	s.maintenanceSaved(w, r, mw, err, http.StatusCreated)
}

func (s *Server) updateMaintenance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in maintenanceInput
	if !decode(w, r, &in) {
		return
	}
	mw := in.window(id)
	if err := mw.Normalize(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	mw, err := s.Store.UpdateMaintenance(r.Context(), mw)
	s.maintenanceSaved(w, r, mw, err, http.StatusOK)
}

// endMaintenance ends a running window now. Alerts held during it go out if
// their monitors are still down.
func (s *Server) endMaintenance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	mw, err := s.Store.GetMaintenance(r.Context(), id)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	if mw.StateAt(time.Now()) != maintenance.Active {
		writeError(w, http.StatusConflict, "this maintenance isn't running")
		return
	}
	mw.EndsAt = time.Now().UTC().Truncate(time.Millisecond)
	mw, err = s.Store.UpdateMaintenance(r.Context(), mw)
	s.maintenanceSaved(w, r, mw, err, http.StatusOK)
}

func (s *Server) deleteMaintenance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteMaintenance(r.Context(), id); err != nil {
		s.storeError(w, r, err)
		return
	}
	s.reloadMaintenance(r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) maintenanceSaved(w http.ResponseWriter, r *http.Request, mw maintenance.Window, err error, status int) {
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	s.reloadMaintenance(r)
	writeJSON(w, status, maintenanceItem{Window: mw, State: mw.StateAt(time.Now())})
}

// reloadMaintenance tells the scheduler, and open pages, about a change.
func (s *Server) reloadMaintenance(r *http.Request) {
	if err := s.Scheduler.ReloadMaintenance(r.Context()); err != nil {
		s.Log.Error("reloading maintenance", "err", err)
	}
	s.monitorsChanged(0)
}
