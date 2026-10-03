package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptimy/agent/internal/incident"
)

// ErrLastUpdate is returned when deleting an incident's only update.
var ErrLastUpdate = errors.New("an incident keeps at least one update; delete the incident instead")

// ListIncidents returns open incidents, and those resolved
// after since, newest first, with their updates.
func (s *Store) ListIncidents(ctx context.Context, since time.Time) ([]incident.Incident, error) {
	return s.queryIncidents(ctx, "WHERE resolved_at IS NULL OR resolved_at > ?", since.UnixMilli())
}

// GetIncident returns one incident with its updates.
func (s *Store) GetIncident(ctx context.Context, id int64) (incident.Incident, error) {
	out, err := s.queryIncidents(ctx, "WHERE id = ?", id)
	if err != nil {
		return incident.Incident{}, err
	}
	if len(out) == 0 {
		return incident.Incident{}, ErrNotFound
	}
	return out[0], nil
}

func (s *Store) queryIncidents(ctx context.Context, where string, args ...any) ([]incident.Incident, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, title, severity, created_at, resolved_at FROM incidents "+where+" ORDER BY created_at DESC, id DESC", args...) //nolint:gosec // G202: where is a constant
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []incident.Incident{}
	byID := map[int64]int{}
	for rows.Next() {
		var (
			in       incident.Incident
			created  int64
			resolved sql.NullInt64
		)
		if err := rows.Scan(&in.ID, &in.Title, &in.Severity, &created, &resolved); err != nil {
			return nil, err
		}
		in.CreatedAt, in.ResolvedAt = fromMillis(created), optTime(resolved)
		in.MonitorIDs, in.Updates = []int64{}, []incident.Update{}
		byID[in.ID] = len(out)
		out = append(out, in)
	}
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}

	// Only the timelines of the incidents found.
	of := " WHERE incident_id IN (SELECT id FROM incidents " + where + ")"
	updates, err := s.db.QueryContext(ctx, "SELECT id, incident_id, status, message, created_at FROM incident_updates"+of+" ORDER BY created_at DESC, id DESC", args...) //nolint:gosec // G202: where is a constant
	if err != nil {
		return nil, err
	}
	defer updates.Close()
	for updates.Next() {
		var (
			u       incident.Update
			iid, at int64
		)
		if err := updates.Scan(&u.ID, &iid, &u.Status, &u.Message, &at); err != nil {
			return nil, err
		}
		u.CreatedAt = fromMillis(at)
		if i, ok := byID[iid]; ok {
			out[i].Updates = append(out[i].Updates, u)
		}
	}
	if err := updates.Err(); err != nil {
		return nil, err
	}

	links, err := s.db.QueryContext(ctx, "SELECT incident_id, monitor_id FROM incident_monitors"+of+" ORDER BY monitor_id", args...) //nolint:gosec // G202: where is a constant
	if err != nil {
		return nil, err
	}
	defer links.Close()
	for links.Next() {
		var iid, mid int64
		if err := links.Scan(&iid, &mid); err != nil {
			return nil, err
		}
		if i, ok := byID[iid]; ok {
			out[i].MonitorIDs = append(out[i].MonitorIDs, mid)
		}
	}
	if err := links.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if len(out[i].Updates) > 0 {
			out[i].Status = out[i].Updates[0].Status
		}
	}
	return out, nil
}

// CreateIncident stores a new incident with its first update.
func (s *Store) CreateIncident(ctx context.Context, in incident.Incident, first incident.Update) (incident.Incident, error) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "INSERT INTO incidents (title, severity, created_at) VALUES (?, ?, ?)",
			in.Title, in.Severity, now.UnixMilli())
		if err != nil {
			return err
		}
		if in.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		if err := setIncidentMonitors(ctx, tx, in); err != nil {
			return err
		}
		return addIncidentUpdate(ctx, tx, in.ID, first, now)
	})
	if err != nil {
		return in, err
	}
	return s.GetIncident(ctx, in.ID)
}

// UpdateIncident saves an incident's title, severity and monitors.
func (s *Store) UpdateIncident(ctx context.Context, in incident.Incident) (incident.Incident, error) {
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE incidents SET title = ?, severity = ? WHERE id = ?", in.Title, in.Severity, in.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return setIncidentMonitors(ctx, tx, in)
	})
	if err != nil {
		return in, err
	}
	return s.GetIncident(ctx, in.ID)
}

func setIncidentMonitors(ctx context.Context, tx *sql.Tx, in incident.Incident) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM incident_monitors WHERE incident_id = ?", in.ID); err != nil {
		return err
	}
	for _, mid := range in.MonitorIDs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO incident_monitors (incident_id, monitor_id) VALUES (?, ?)", in.ID, mid); err != nil {
			return unknownMonitor(err)
		}
	}
	return nil
}

// AddIncidentUpdate adds to the timeline. A "resolved" update resolves the
// incident; any other reopens it.
func (s *Store) AddIncidentUpdate(ctx context.Context, id int64, u incident.Update) (incident.Incident, error) {
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		return addIncidentUpdate(ctx, tx, id, u, time.Now().UTC().Truncate(time.Millisecond))
	})
	if err != nil {
		return incident.Incident{}, err
	}
	return s.GetIncident(ctx, id)
}

func addIncidentUpdate(ctx context.Context, tx *sql.Tx, id int64, u incident.Update, at time.Time) error {
	var resolved any
	if u.Status == incident.Resolved {
		resolved = at.UnixMilli()
	}
	res, err := tx.ExecContext(ctx, "UPDATE incidents SET resolved_at = ? WHERE id = ?", resolved, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO incident_updates (incident_id, status, message, created_at) VALUES (?, ?, ?, ?)",
		id, u.Status, u.Message, at.UnixMilli())
	return err
}

// EditIncidentUpdate changes an update's message, to fix a typo.
func (s *Store) EditIncidentUpdate(ctx context.Context, id, updateID int64, message string) (incident.Incident, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE incident_updates SET message = ? WHERE id = ? AND incident_id = ?", message, updateID, id)
	if err != nil {
		return incident.Incident{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return incident.Incident{}, ErrNotFound
	}
	return s.GetIncident(ctx, id)
}

// DeleteIncidentUpdate removes an update that isn't the incident's only
// one. Whether it's resolved follows the update that's then the latest.
func (s *Store) DeleteIncidentUpdate(ctx context.Context, id, updateID int64) (incident.Incident, error) {
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM incident_updates WHERE incident_id = ?", id).Scan(&n); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "DELETE FROM incident_updates WHERE id = ? AND incident_id = ?", updateID, id)
		if err != nil {
			return err
		}
		if d, _ := res.RowsAffected(); d == 0 {
			return ErrNotFound
		}
		if n <= 1 {
			return ErrLastUpdate
		}
		var (
			status string
			at     int64
		)
		if err := tx.QueryRowContext(ctx, "SELECT status, created_at FROM incident_updates WHERE incident_id = ? ORDER BY created_at DESC, id DESC LIMIT 1", id).Scan(&status, &at); err != nil {
			return err
		}
		var resolved any
		if incident.Status(status) == incident.Resolved {
			resolved = at
		}
		_, err = tx.ExecContext(ctx, "UPDATE incidents SET resolved_at = ? WHERE id = ?", resolved, id)
		return err
	})
	if err != nil {
		return incident.Incident{}, err
	}
	return s.GetIncident(ctx, id)
}

// DeleteIncident removes an incident and its timeline.
func (s *Store) DeleteIncident(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM incidents WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
