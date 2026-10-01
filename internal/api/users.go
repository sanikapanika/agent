package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/uptimy/agent/internal/store"
)

func validRole(role string) bool { return role == store.RoleAdmin || role == store.RoleViewer }

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := make([]meResponse, len(users))
	for i, u := range users {
		out[i] = s.me(u)
	}
	writeJSON(w, http.StatusOK, out)
}

// createUser adds a user with a temporary password, which they must change
// when they first sign in.
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if !usernamePattern.MatchString(in.Username) {
		writeError(w, http.StatusBadRequest, "username must be 2–64 characters: letters, numbers, . _ @ -")
		return
	}
	if !validRole(in.Role) {
		writeError(w, http.StatusBadRequest, "role must be admin or viewer")
		return
	}
	if len(in.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	u, err := s.Store.CreateUser(r.Context(), store.User{
		Username: in.Username, PasswordHash: hash, Role: in.Role, MustChangePassword: true,
	})
	if errors.Is(err, store.ErrDuplicate) {
		writeError(w, http.StatusConflict, "that username is taken")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.me(u))
}

// updateUser changes a user's role and/or resets their password. A reset
// password is temporary: the user must change it at next sign-in, and their
// existing sessions end.
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Role     *string `json:"role"`
		Password *string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	target, err := s.Store.GetUser(r.Context(), id)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	me := currentUser(r)

	if in.Role != nil && *in.Role != target.Role {
		if !validRole(*in.Role) {
			writeError(w, http.StatusBadRequest, "role must be admin or viewer")
			return
		}
		if target.ID == me.ID {
			writeError(w, http.StatusConflict, "you can't change your own role")
			return
		}
		if s.passwordManagedByEnv(target) {
			writeError(w, http.StatusConflict, "this admin is defined by ADMIN_USERNAME / ADMIN_PASSWORD and stays an admin")
			return
		}
		if err := s.Store.SetUserRole(r.Context(), id, *in.Role); err != nil {
			s.storeError(w, r, err)
			return
		}
	}

	if in.Password != nil {
		if target.ID == me.ID {
			writeError(w, http.StatusConflict, "change your own password from Settings")
			return
		}
		if s.passwordManagedByEnv(target) {
			writeError(w, http.StatusConflict, "this password is set by the ADMIN_PASSWORD environment variable")
			return
		}
		if len(*in.Password) < minPasswordLen {
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		hash, err := hashPassword(*in.Password)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if err := s.Store.SetUserPassword(r.Context(), id, hash, true); err != nil {
			s.storeError(w, r, err)
			return
		}
		if err := s.Store.DeleteUserSessions(r.Context(), id, ""); err != nil {
			s.internalError(w, r, err)
			return
		}
	}

	updated, err := s.Store.GetUser(r.Context(), id)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.me(updated))
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target, err := s.Store.GetUser(r.Context(), id)
	if err != nil {
		s.storeError(w, r, err)
		return
	}
	if target.ID == currentUser(r).ID {
		writeError(w, http.StatusConflict, "you can't delete your own account")
		return
	}
	if s.passwordManagedByEnv(target) {
		writeError(w, http.StatusConflict, "this admin is defined by ADMIN_USERNAME / ADMIN_PASSWORD and would be recreated on restart")
		return
	}
	if target.IsAdmin() {
		admins, err := s.Store.CountAdmins(r.Context())
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if admins <= 1 {
			writeError(w, http.StatusConflict, "you can't delete the last admin")
			return
		}
	}
	if err := s.Store.DeleteUser(r.Context(), id); err != nil {
		s.storeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
