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
}

// IsAdmin reports whether the user can change things.
func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

const userColumns = "id, username, password_hash, role, must_change_password, created_at, last_login_at"

func scanUser(row scanner) (User, error) {
	var (
		u         User
		created   int64
		lastLogin sql.NullInt64
	)
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.MustChangePassword, &created, &lastLogin)
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
