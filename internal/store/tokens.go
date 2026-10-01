package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// APIToken is a personal token for the agent's API. It acts as its owner,
// read-only if ReadOnly is set or the owner is a viewer.
type APIToken struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"-"`
	Name       string     `json:"name"`
	Hint       string     `json:"hint"` // the token's last characters
	ReadOnly   bool       `json:"read_only"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// tokenUseResolution is how often last_used_at is written, so a busy
// automation doesn't write on every request.
const tokenUseResolution = time.Minute

// CreateAPIToken stores a token by its hash.
func (s *Store) CreateAPIToken(ctx context.Context, t APIToken, tokenHash string) (APIToken, error) {
	t.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO api_tokens (user_id, name, token_hash, hint, read_only, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		t.UserID, t.Name, tokenHash, t.Hint, t.ReadOnly, t.CreatedAt.UnixMilli())
	if err != nil {
		return t, err
	}
	t.ID, err = res.LastInsertId()
	return t, err
}

// ListAPITokens returns a user's tokens, newest first.
func (s *Store) ListAPITokens(ctx context.Context, userID int64) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, name, hint, read_only, created_at, last_used_at
		FROM api_tokens WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func scanAPIToken(row scanner) (APIToken, error) {
	var (
		t        APIToken
		created  int64
		lastUsed sql.NullInt64
	)
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Hint, &t.ReadOnly, &created, &lastUsed); err != nil {
		return t, err
	}
	t.CreatedAt = fromMillis(created)
	if lastUsed.Valid {
		at := fromMillis(lastUsed.Int64)
		t.LastUsedAt = &at
	}
	return t, nil
}

// DeleteAPIToken revokes one of a user's tokens.
func (s *Store) DeleteAPIToken(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM api_tokens WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TokenUser returns the token with this hash and its owner, and notes that
// it was used.
func (s *Store) TokenUser(ctx context.Context, tokenHash string) (User, APIToken, error) {
	t, err := scanAPIToken(s.db.QueryRowContext(ctx, `SELECT id, user_id, name, hint, read_only, created_at, last_used_at
		FROM api_tokens WHERE token_hash = ?`, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, t, ErrNotFound
	}
	if err != nil {
		return User{}, t, err
	}
	u, err := s.GetUser(ctx, t.UserID)
	if err != nil {
		return u, t, err
	}
	now := time.Now()
	if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) > tokenUseResolution {
		if _, err := s.db.ExecContext(ctx, "UPDATE api_tokens SET last_used_at = ? WHERE id = ?", now.UnixMilli(), t.ID); err != nil {
			return u, t, err
		}
	}
	return u, t, nil
}
