package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Roles.
const (
	RoleAdmin  = "admin"  // everything, including managing users
	RoleViewer = "viewer" // read-only
)

// ErrDuplicate is returned when a username is already taken.
var ErrDuplicate = errors.New("already exists")

// User is an account that can sign in to the UI.
type User struct {
	ID                 int64      `json:"id"`
	Username           string     `json:"username"`
	PasswordHash       string     `json:"-"`
	Role               string     `json:"role"`
	MustChangePassword bool       `json:"must_change_password"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	// TwoFactor: signing in also takes a code from an authenticator app.
	TwoFactor bool `json:"two_factor"`
	// TOTPSecret is the authenticator secret, also set while setting up
	// (TwoFactor still false); TOTPLastStep is the last step a code was
	// accepted for.
	TOTPSecret   string `json:"-"`
	TOTPLastStep int64  `json:"-"`
}

// IsAdmin reports whether the user can change things.
func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

const userColumns = "id, username, password_hash, role, must_change_password, created_at, last_login_at, totp_enabled, totp_secret, totp_last_step"

func scanUser(row scanner) (User, error) {
	var (
		u         User
		created   int64
		lastLogin sql.NullInt64
	)
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.MustChangePassword, &created, &lastLogin,
		&u.TwoFactor, &u.TOTPSecret, &u.TOTPLastStep)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	if err != nil {
		return u, err
	}
	u.CreatedAt = fromMillis(created)
	if lastLogin.Valid {
		t := fromMillis(lastLogin.Int64)
		u.LastLoginAt = &t
	}
	return u, nil
}

// CountUsers returns how many users exist.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// CountAdmins returns how many admins exist.
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role = ?", RoleAdmin).Scan(&n)
	return n, err
}

// ListUsers returns all users ordered by username.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+userColumns+" FROM users ORDER BY username COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetUser returns a user by ID.
func (s *Store) GetUser(ctx context.Context, id int64) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE id = ?", id))
}

// GetUserByUsername returns a user by (case-insensitive) username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE username = ?", username))
}

// CreateUser inserts a user.
func (s *Store) CreateUser(ctx context.Context, u User) (User, error) {
	u.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO users (username, password_hash, role, must_change_password, created_at) VALUES (?, ?, ?, ?, ?)",
		u.Username, u.PasswordHash, u.Role, u.MustChangePassword, u.CreatedAt.UnixMilli())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return u, ErrDuplicate
		}
		return u, err
	}
	u.ID, err = res.LastInsertId()
	return u, err
}

// SetUserRole changes a user's role.
func (s *Store) SetUserRole(ctx context.Context, id int64, role string) error {
	return s.execOne(ctx, "UPDATE users SET role = ? WHERE id = ?", role, id)
}

// SetUserPassword stores a new password hash and whether it must be changed
// at next sign-in.
func (s *Store) SetUserPassword(ctx context.Context, id int64, hash string, mustChange bool) error {
	return s.execOne(ctx, "UPDATE users SET password_hash = ?, must_change_password = ? WHERE id = ?", hash, mustChange, id)
}

// TouchLogin records a successful sign-in.
func (s *Store) TouchLogin(ctx context.Context, id int64) error {
	return s.execOne(ctx, "UPDATE users SET last_login_at = ? WHERE id = ?", time.Now().UnixMilli(), id)
}

// DeleteUser removes a user; their sessions go with them.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	return s.execOne(ctx, "DELETE FROM users WHERE id = ?", id)
}

func (s *Store) execOne(ctx context.Context, q string, args ...any) error {
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateSession stores a hashed session token for a user.
func (s *Store) CreateSession(ctx context.Context, tokenHash string, userID int64, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)",
		tokenHash, userID, expires.UnixMilli())
	return err
}

// SessionUser returns the user behind a valid, unexpired session.
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+prefixed("u.", userColumns)+`
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, tokenHash, time.Now().UnixMilli()))
}

// CountUserSessions returns how many unexpired sessions a user has.
func (s *Store) CountUserSessions(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions WHERE user_id = ? AND expires_at > ?",
		userID, time.Now().UnixMilli()).Scan(&n)
	return n, err
}

// DeleteSession removes one session.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	return err
}

// DeleteUserSessions signs a user out everywhere except, optionally, the
// session identified by keepTokenHash.
func (s *Store) DeleteUserSessions(ctx context.Context, userID int64, keepTokenHash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND token_hash != ?", userID, keepTokenHash)
	return err
}

func prefixed(prefix, columns string) string {
	parts := strings.Split(columns, ", ")
	for i, p := range parts {
		parts[i] = prefix + p
	}
	return strings.Join(parts, ", ")
}

// SetTOTPSecret stores a new authenticator secret for a user setting up
// two-factor sign-in. It doesn't turn it on: EnableTOTP does, once a code
// from the app proves the secret was saved.
func (s *Store) SetTOTPSecret(ctx context.Context, userID int64, secret string) error {
	return s.execOne(ctx, "UPDATE users SET totp_secret = ? WHERE id = ? AND totp_enabled = 0", secret, userID)
}

// EnableTOTP turns two-factor sign-in on, records the step the confirming
// code was for, and replaces the recovery codes with codeHashes.
func (s *Store) EnableTOTP(ctx context.Context, userID, step int64, codeHashes []string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE users SET totp_enabled = 1, totp_last_step = ? WHERE id = ?", step, userID); err != nil {
			return err
		}
		return replaceRecoveryCodes(ctx, tx, userID, codeHashes)
	})
}

// ReplaceRecoveryCodes swaps a user's recovery codes for new ones.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, userID int64, codeHashes []string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error { return replaceRecoveryCodes(ctx, tx, userID, codeHashes) })
}

func replaceRecoveryCodes(ctx context.Context, tx *sql.Tx, userID int64, codeHashes []string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id = ?", userID); err != nil {
		return err
	}
	for _, h := range codeHashes {
		if _, err := tx.ExecContext(ctx, "INSERT INTO recovery_codes (user_id, code_hash) VALUES (?, ?)", userID, h); err != nil {
			return err
		}
	}
	return nil
}

// DisableTOTP turns two-factor sign-in off and forgets the secret and the
// recovery codes.
func (s *Store) DisableTOTP(ctx context.Context, userID int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE users SET totp_enabled = 0, totp_secret = '', totp_last_step = 0 WHERE id = ?", userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id = ?", userID)
		return err
	})
}

// UseTOTPStep accepts a code for step once: it reports false if a code for
// this step, or a later one, was already used.
func (s *Store) UseTOTPStep(ctx context.Context, userID, step int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE users SET totp_last_step = ? WHERE id = ? AND totp_last_step < ?", step, userID, step)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// UseRecoveryCode deletes a recovery code, reporting whether it existed.
func (s *Store) UseRecoveryCode(ctx context.Context, userID int64, codeHash string) (bool, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id = ? AND code_hash = ?", userID, codeHash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// CountRecoveryCodes returns how many unused recovery codes a user has.
func (s *Store) CountRecoveryCodes(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM recovery_codes WHERE user_id = ?", userID).Scan(&n)
	return n, err
}
