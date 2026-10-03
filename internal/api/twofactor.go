package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/uptimy/agent/internal/store"
	"github.com/uptimy/agent/internal/totp"
)

// Two-factor sign-in: after the password, a code from an authenticator app
// (TOTP) or a one-time recovery code. It's per user and set up from the
// account page. API tokens aren't affected: they're made by a signed-in
// user and are already a second credential.

const recoveryCodeCount = 10

// twoFactorStatus says whether two-factor sign-in is on and how many
// recovery codes are left.
func (s *Server) twoFactorStatus(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	left := 0
	if u.TwoFactor {
		var err error
		if left, err = s.Store.CountRecoveryCodes(r.Context(), u.ID); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": u.TwoFactor, "recovery_codes_left": left})
}

// setupTwoFactor starts setting up: a new secret, shown once as a QR code.
// Nothing changes for sign-in until enableTwoFactor confirms a code.
func (s *Server) setupTwoFactor(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u.TwoFactor {
		writeError(w, http.StatusConflict, "two-factor sign-in is already on; turn it off first to use another app")
		return
	}
	secret := totp.NewSecret()
	if err := s.Store.SetTOTPSecret(r.Context(), u.ID, secret); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret": secret,
		"uri":    totp.URI("Uptimy Agent ("+s.Config.AgentName+")", u.Username, secret),
	})
}

// enableTwoFactor turns it on once a code proves the app has the secret,
// returns the recovery codes (shown once) and signs out other sessions.
func (s *Server) enableTwoFactor(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var req struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &req) {
		return
	}
	if u.TwoFactor {
		writeError(w, http.StatusConflict, "two-factor sign-in is already on")
		return
	}
	if u.TOTPSecret == "" {
		writeError(w, http.StatusBadRequest, "start the setup first")
		return
	}
	step, ok := totp.Verify(u.TOTPSecret, req.Code, time.Now())
	if !ok {
		writeError(w, http.StatusBadRequest, "that code didn't match; check the time on your phone and try the newest code")
		return
	}
	codes, hashes := newRecoveryCodes()
	if err := s.Store.EnableTOTP(r.Context(), u.ID, step, hashes); err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.Store.DeleteUserSessions(r.Context(), u.ID, currentSessionHash(r)); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

// disableTwoFactor turns it off; it takes the password.
func (s *Server) disableTwoFactor(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !s.confirmPassword(w, r, u) {
		return
	}
	if err := s.Store.DisableTOTP(r.Context(), u.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// newRecoveryCodesHandler replaces the recovery codes; it takes the
// password.
func (s *Server) newRecoveryCodesHandler(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !u.TwoFactor {
		writeError(w, http.StatusConflict, "two-factor sign-in is off")
		return
	}
	if !s.confirmPassword(w, r, u) {
		return
	}
	codes, hashes := newRecoveryCodes()
	if err := s.Store.ReplaceRecoveryCodes(r.Context(), u.ID, hashes); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

// resetTwoFactor is for admins: it turns another user's two-factor sign-in
// off, for when they've lost their phone and their recovery codes.
func (s *Server) resetTwoFactor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.Store.DisableTOTP(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such user")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// confirmPassword checks {"password": ...} in the body against u's.
func (s *Server) confirmPassword(w http.ResponseWriter, r *http.Request, u store.User) bool {
	var req struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return false
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		writeError(w, http.StatusBadRequest, "password is wrong")
		return false
	}
	return true
}

// checkSecondFactor accepts a current authenticator code (once) or an
// unused recovery code (which is then used up).
func (s *Server) checkSecondFactor(ctx context.Context, u store.User, code string) (bool, error) {
	if step, ok := totp.Verify(u.TOTPSecret, code, time.Now()); ok {
		return s.Store.UseTOTPStep(ctx, u.ID, step)
	}
	norm := normalizeRecoveryCode(code)
	if len(norm) != 10 {
		return false, nil
	}
	return s.Store.UseRecoveryCode(ctx, u.ID, hashRecoveryCode(norm))
}

var recoveryAlphabet = base32.NewEncoding("abcdefghijkmnpqrstuvwxyz23456789").WithPadding(base32.NoPadding)

// newRecoveryCodes returns codes like "k7m2x-9qpre" and their hashes.
func newRecoveryCodes() (codes, hashes []string) {
	for range recoveryCodeCount {
		b := make([]byte, 7)
		_, _ = rand.Read(b)
		c := recoveryAlphabet.EncodeToString(b)[:10]
		codes = append(codes, c[:5]+"-"+c[5:])
		hashes = append(hashes, hashRecoveryCode(c))
	}
	return codes, hashes
}

func normalizeRecoveryCode(c string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(c)))
}

func hashRecoveryCode(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// currentSessionHash is the hash of the request's session token, or "".
func currentSessionHash(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return hashToken(c.Value)
	}
	return ""
}
