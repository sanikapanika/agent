package api

import (
	"net/http"
)

// kubernetesStatus says how discovery is doing: when it last scanned, what
// it found, and what it couldn't use (a bad annotation, missing RBAC).
func (s *Server) kubernetesStatus(w http.ResponseWriter, r *http.Request) {
	if s.Discovery == nil {
		writeError(w, http.StatusNotFound, "Kubernetes discovery is off: the agent isn't running in a cluster, or KUBERNETES_DISCOVERY is false")
		return
	}
	writeJSON(w, http.StatusOK, s.Discovery.Status())
}

// kubernetesResources lists what discovery could monitor, labeled or not,
// for the resource browser.
func (s *Server) kubernetesResources(w http.ResponseWriter, r *http.Request) {
	if s.Discovery == nil {
		writeError(w, http.StatusNotFound, "Kubernetes discovery is off")
		return
	}
	res, err := s.Discovery.Resources(r.Context(), s.Store)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
