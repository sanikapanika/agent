package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/uptimy/agent/internal/incident"
	"github.com/uptimy/agent/internal/store"
)

// incidentHistory is how long resolved incidents stay listed, in the UI and
// on the status page.
const incidentHistory = 14 * 24 * time.Hour

func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	out, err := s.Store.ListIncidents(r.Context(), time.Now().Add(-incidentHistory))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// createIncident posts an incident with its first update.
func (s *Server) createIncident(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title      string            `json:"title"`
		Severity   incident.Severity `json:"severity"`
		Status     incident.Status   `json:"status"`
		Message    string            `json:"message"`
		MonitorIDs []int64           `json:"monitor_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	inc := incident.Incident{Title: in.Title, Severity: in.Severity, MonitorIDs: in.MonitorIDs}
	first := incident.Update{Status: in.Status, Message: in.Message}
	if err := inc.Normalize(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := incident.NormalizeUpdate(&first); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	inc, err := s.Store.CreateIncident(r.Context(), inc, first)
	s.incidentSaved(w, r, inc, err, http.StatusCreated)
}

// updateIncident edits the title, severity and affected monitors.
func (s *Server) updateIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	cur, err := s.Store.GetIncident(r.Context(), id)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	var in struct {
		Title      string            `json:"title"`
		Severity   incident.Severity `json:"severity"`
		MonitorIDs []int64           `json:"monitor_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	cur.Title, cur.Severity, cur.MonitorIDs = in.Title, in.Severity, in.MonitorIDs
	if err := cur.Normalize(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	inc, err := s.Store.UpdateIncident(r.Context(), cur)
	s.incidentSaved(w, r, inc, err, http.StatusOK)
}

// addIncidentUpdate posts the next update; "resolved" resolves it.
func (s *Server) addIncidentUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var u incident.Update
	if !decode(w, r, &u) {
		return
	}
	if err := incident.NormalizeUpdate(&u); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	inc, err := s.Store.AddIncidentUpdate(r.Context(), id, u)
	s.incidentSaved(w, r, inc, err, http.StatusOK)
}

func (s *Server) editIncidentUpdate(w http.ResponseWriter, r *http.Request) {
	id, updateID, ok := incidentUpdatePath(w, r)
	if !ok {
		return
	}
	var in struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &in) {
		return
	}
	msg, err := incident.NormalizeMessage(in.Message)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	inc, err := s.Store.EditIncidentUpdate(r.Context(), id, updateID, msg)
	s.incidentSaved(w, r, inc, err, http.StatusOK)
}

func (s *Server) deleteIncidentUpdate(w http.ResponseWriter, r *http.Request) {
	id, updateID, ok := incidentUpdatePath(w, r)
	if !ok {
		return
	}
	inc, err := s.Store.DeleteIncidentUpdate(r.Context(), id, updateID)
	if errors.Is(err, store.ErrLastUpdate) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.incidentSaved(w, r, inc, err, http.StatusOK)
}

func (s *Server) deleteIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteIncident(r.Context(), id); err != nil {
		s.storeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) incidentSaved(w http.ResponseWriter, r *http.Request, inc incident.Incident, err error, status int) {
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, status, inc)
}

func incidentUpdatePath(w http.ResponseWriter, r *http.Request) (id, updateID int64, ok bool) {
	if id, ok = pathID(w, r); !ok {
		return 0, 0, false
	}
	updateID, err := strconv.ParseInt(r.PathValue("update"), 10, 64)
	if err != nil || updateID < 1 {
		writeError(w, http.StatusBadRequest, "invalid update id")
		return 0, 0, false
	}
	return id, updateID, true
}
