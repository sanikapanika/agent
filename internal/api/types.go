package api

import (
	"net/http"

	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/notify"
)

// The UI builds its healthcheck and notification forms from these, so a new
// type added in Go shows up in the UI without frontend changes.

func (s *Server) checkTypes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, monitor.CheckTypes())
}

func (s *Server) notifierTypes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, notify.Channels())
}

// typeLabel is what a monitor is, for display: its check type ("PostgreSQL")
// or "Heartbeat".
func typeLabel(m monitor.Monitor) string {
	if m.Check == nil {
		return "Heartbeat"
	}
	if ct, ok := monitor.LookupCheckType(m.Check.Type); ok {
		return ct.Label
	}
	return string(m.Check.Type)
}
