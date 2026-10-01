package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/uptimy/agent/internal/statuspage"
)

// statusPageLogos are the logos' URLs; empty when not set. The version in
// the query string lets browsers cache a logo until it's replaced.
type statusPageLogos struct {
	Light string `json:"light,omitempty"`
	Dark  string `json:"dark,omitempty"`
}

func (s *Server) statusPageLogos(ctx context.Context) (statusPageLogos, error) {
	var out statusPageLogos
	for v, dst := range map[statuspage.Variant]*string{statuspage.Light: &out.Light, statuspage.Dark: &out.Dark} {
		l, err := s.Store.StatusPageLogo(ctx, v)
		if err != nil {
			return out, err
		}
		if l != nil {
			*dst = fmt.Sprintf("/api/status/logo/%s?v=%d", v, l.Version)
		}
	}
	return out, nil
}

// logoVariant reads the variant from the path, or answers 404.
func logoVariant(w http.ResponseWriter, r *http.Request) (statuspage.Variant, bool) {
	v, ok := statuspage.ParseVariant(r.PathValue("variant"))
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
	}
	return v, ok
}

func (s *Server) putStatusPageLogo(w http.ResponseWriter, r *http.Request) {
	variant, ok := logoVariant(w, r)
	if !ok {
		return
	}
	// JSON with the image as base64, like every other mutation: requiring
	// application/json is the agent's CSRF guard.
	var in struct {
		Data string `json:"data"`
	}
	if !decode(w, r, &in) {
		return
	}
	body, err := base64.StdEncoding.DecodeString(in.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the image couldn't be read")
		return
	}
	logo, err := statuspage.NewLogo(body)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, statuspage.ErrLogoTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, status, err.Error())
		return
	}
	if err := s.Store.SetStatusPageLogo(r.Context(), variant, &logo); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.monitorsChanged(0) // refreshes open status page editors
	s.getStatusPageConfig(w, r)
}

func (s *Server) deleteStatusPageLogo(w http.ResponseWriter, r *http.Request) {
	variant, ok := logoVariant(w, r)
	if !ok {
		return
	}
	if err := s.Store.SetStatusPageLogo(r.Context(), variant, nil); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.monitorsChanged(0)
	s.getStatusPageConfig(w, r)
}

// statusPageLogo serves a logo publicly (it's shown on the public page).
func (s *Server) statusPageLogo(w http.ResponseWriter, r *http.Request) {
	variant, ok := logoVariant(w, r)
	if !ok {
		return
	}
	l, err := s.Store.StatusPageLogo(r.Context(), variant)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if l == nil {
		writeError(w, http.StatusNotFound, "no logo")
		return
	}
	h := w.Header()
	h.Set("Content-Type", l.Type)
	h.Set("X-Content-Type-Options", "nosniff")
	// An SVG opened directly could otherwise run script on the agent's origin.
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	if r.URL.Query().Get("v") == strconv.FormatInt(l.Version, 10) {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	w.Write(l.Data) //nolint:gosec // G705: an image checked on upload, served with nosniff and a sandboxing CSP
}
