package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/uptimy/agent/internal/kuma"
	"github.com/uptimy/agent/internal/monitor"
)

// Importing from Uptime Kuma takes two steps: upload kuma.db and get back a
// plan of what would be created (planKumaImport), then send back the parts
// to import (applyKumaImport). Nothing is kept between the two.

// maxKumaUpload is generous: kuma.db holds Kuma's whole check history.
const maxKumaUpload = 2 << 30

func (s *Server) planKumaImport(w http.ResponseWriter, r *http.Request) {
	// The file is opened with SQLite, so it goes to disk first, next to the
	// agent's own database (the root filesystem may be read-only).
	f, err := os.CreateTemp(s.Config.DataDir, ".kuma-import-*.db")
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer os.Remove(f.Name())
	_, err = io.Copy(f, http.MaxBytesReader(w, r.Body, maxKumaUpload))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "that file is over 2 GB; is it kuma.db?")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "the upload didn't finish; try again")
		return
	}
	plan, err := kuma.Read(r.Context(), f.Name())
	if err != nil {
		writeError(w, http.StatusBadRequest, "couldn't read the file: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

type importedItem struct {
	ID   int64        `json:"id"`
	Name string       `json:"name"`
	Kind monitor.Kind `json:"kind,omitempty"`
}

func (s *Server) applyKumaImport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Monitors  []kuma.Monitor  `json:"monitors"`
		Notifiers []kuma.Notifier `json:"notifiers"`
	}
	if !decode(w, r, &in) {
		return
	}
	existing, err := s.Store.ListMonitors(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	taken := map[string]bool{}
	for _, m := range existing {
		taken[strings.ToLower(m.Name)] = true
	}

	out := struct {
		Monitors  []importedItem `json:"monitors"`
		Notifiers []importedItem `json:"notifiers"`
		Skipped   []kuma.Skipped `json:"skipped"`
	}{[]importedItem{}, []importedItem{}, []kuma.Skipped{}}
	skip := func(what, name, reason string) {
		out.Skipped = append(out.Skipped, kuma.Skipped{What: what, Name: name, Reason: reason})
	}

	ids := map[int64]int64{} // Kuma monitor ID → new monitor ID
	for _, km := range in.Monitors {
		m := km.Monitor
		m.ID, m.Source = 0, monitor.SourceUI
		if m.Heartbeat != nil {
			m.Heartbeat.Token = monitor.NewToken()
		}
		if taken[strings.ToLower(strings.TrimSpace(m.Name))] {
			skip("monitor", m.Name, "a monitor with this name already exists")
			continue
		}
		if err := m.Normalize(); err != nil {
			skip("monitor", m.Name, err.Error())
			continue
		}
		created, err := s.Store.CreateMonitor(r.Context(), m)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		taken[strings.ToLower(created.Name)] = true
		ids[km.KumaID] = created.ID
		s.Scheduler.Upsert(created)
		out.Monitors = append(out.Monitors, importedItem{ID: created.ID, Name: created.Name, Kind: created.Kind})
	}

	for _, kn := range in.Notifiers {
		n := kn.Notifier
		n.ID, n.MonitorIDs = 0, []int64{}
		for _, kid := range kn.KumaMonitorIDs {
			if id, ok := ids[kid]; ok {
				n.MonitorIDs = append(n.MonitorIDs, id)
			}
		}
		if err := n.Normalize(); err != nil {
			skip("notification", n.Name, err.Error())
			continue
		}
		created, err := s.Store.CreateNotifier(r.Context(), n)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		out.Notifiers = append(out.Notifiers, importedItem{ID: created.ID, Name: created.Name})
	}

	s.monitorsChanged(0)
	writeJSON(w, http.StatusOK, out)
}
