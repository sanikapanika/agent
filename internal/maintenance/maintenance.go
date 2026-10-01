// Package maintenance models planned work: a window of time in which some or
// all monitors may go down without alerting anyone, announced on the status
// page if it's public.
package maintenance

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// MaxLength is the longest window allowed, so a typo can't silence alerts
// for months.
const MaxLength = 31 * 24 * time.Hour

// Window is one maintenance window.
type Window struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	// AllMonitors covers every monitor; otherwise only MonitorIDs.
	AllMonitors bool    `json:"all_monitors"`
	MonitorIDs  []int64 `json:"monitor_ids"`
	// Public announces the window on the status page.
	Public    bool      `json:"public"`
	CreatedAt time.Time `json:"created_at"`
}

// State is where a window is relative to now.
type State string

const (
	Scheduled State = "scheduled"
	Active    State = "active"
	Ended     State = "ended"
)

// Normalize validates the window and cleans it up.
func (w *Window) Normalize() error {
	w.Title = strings.TrimSpace(w.Title)
	w.Description = strings.TrimSpace(w.Description)
	w.StartsAt = w.StartsAt.UTC().Truncate(time.Millisecond)
	w.EndsAt = w.EndsAt.UTC().Truncate(time.Millisecond)
	if w.AllMonitors || w.MonitorIDs == nil {
		w.MonitorIDs = []int64{}
	}
	slices.Sort(w.MonitorIDs)
	w.MonitorIDs = slices.Compact(w.MonitorIDs)
	switch {
	case w.Title == "" || len(w.Title) > 100:
		return errors.New("give the maintenance a title of up to 100 characters")
	case len(w.Description) > 1000:
		return errors.New("keep the description under 1000 characters")
	case w.StartsAt.IsZero() || w.EndsAt.IsZero():
		return errors.New("set when the maintenance starts and ends")
	case !w.EndsAt.After(w.StartsAt):
		return errors.New("the maintenance must end after it starts")
	case w.EndsAt.Sub(w.StartsAt) > MaxLength:
		return errors.New("a maintenance window can last at most 31 days")
	case !w.AllMonitors && len(w.MonitorIDs) == 0:
		return errors.New("choose the monitors it covers, or all monitors")
	}
	return nil
}

// StateAt returns the window's state at t.
func (w Window) StateAt(t time.Time) State {
	switch {
	case t.Before(w.StartsAt):
		return Scheduled
	case t.Before(w.EndsAt):
		return Active
	default:
		return Ended
	}
}

// Covers reports whether the window includes a monitor.
func (w Window) Covers(monitorID int64) bool {
	return w.AllMonitors || slices.Contains(w.MonitorIDs, monitorID)
}

// Schedule is a set of windows.
type Schedule []Window

// Covering returns the windows active at t that include a monitor.
func (s Schedule) Covering(monitorID int64, t time.Time) Schedule {
	var out Schedule
	for _, w := range s {
		if w.StateAt(t) == Active && w.Covers(monitorID) {
			out = append(out, w)
		}
	}
	return out
}
