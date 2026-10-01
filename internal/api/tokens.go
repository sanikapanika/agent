package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/uptimy/agent/internal/store"
)

// API tokens let scripts and tools use the agent's API without a browser
// session: `Authorization: Bearer upa_…`. A token acts as the user who made
// it, read-only if they're a viewer or chose so. Tokens can't manage
// accounts, users or other tokens; that takes signing in with a password.

const (
	tokenPrefix      = "upa_"
	maxTokensPerUser = 20
)

func newAPIToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b) // never fails (crypto/rand panics instead)
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
}

// bearerToken returns the API token a request carries, if any.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(h, "Bearer ")
	if !ok {
		return "", false
	}
	return strings.TrimSpace(token), true
}

// serveWithToken authorizes a request made with an API token.
func (s *Server) serveWithToken(w http.ResponseWriter, r *http.Request, token string, readOnly bool, next http.Handler) {
	if !strings.HasPrefix(token, tokenPrefix) {
		writeError(w, http.StatusUnauthorized, "invalid API token")
		return
	}
	u, t, err := s.Store.TokenUser(r.Context(), hashToken(token))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusUnauthorized, "invalid API token")
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/auth/") || strings.HasPrefix(r.URL.Path, "/api/users"):
		writeError(w, http.StatusForbidden, "API tokens can't manage accounts or tokens; sign in to do that")
	case !readOnly && !preflighted(r):
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
	case !readOnly && (t.ReadOnly || !u.IsAdmin()):
		writeError(w, http.StatusForbidden, "this API token is read-only")
	case u.MustChangePassword:
		writeError(w, http.StatusForbidden, "password change required")
	default:
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	}
}

func (s *Server) listAPITokens(w http.ResponseWriter, r *http.Request) {
	ts, err := s.Store.ListAPITokens(r.Context(), currentUser(r).ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

// createAPIToken makes a token and returns it, the only time it's shown.
func (s *Server) createAPIToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		ReadOnly bool   `json:"read_only"`
	}
	if !decode(w, r, &in) {
		return
	}
	u := currentUser(r)
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 60 {
		writeError(w, http.StatusBadRequest, "give the token a name of up to 60 characters, e.g. what uses it")
		return
	}
	existing, err := s.Store.ListAPITokens(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if len(existing) >= maxTokensPerUser {
		writeError(w, http.StatusConflict, "you have 20 tokens already; revoke one you no longer use")
		return
	}
	token := newAPIToken()
	t, err := s.Store.CreateAPIToken(r.Context(), store.APIToken{
		UserID: u.ID, Name: in.Name, Hint: token[len(token)-4:],
		ReadOnly: in.ReadOnly || !u.IsAdmin(), // a viewer's token can only read
	}, hashToken(token))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "api_token": t})
}

func (s *Server) deleteAPIToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteAPIToken(r.Context(), currentUser(r).ID, id); err != nil {
		s.storeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
