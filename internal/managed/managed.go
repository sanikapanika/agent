// Package managed keeps monitors that are defined outside the UI (in the
// monitors file, or discovered in Kubernetes) in step with the store. Each
// source owns its monitors, matched by SourceRef when the source sets one
// (so a rename keeps the history) and by name otherwise; monitors from the
// UI or another source are never touched.
package managed

import (
	"context"
	"reflect"

	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/store"
)

// Desired is a monitor as its source describes it.
type Desired struct {
	monitor.Monitor
	// GeneratedToken: the source didn't set the heartbeat's token, so an
	// existing heartbeat keeps the one it has.
	GeneratedToken bool
	// SetPaused, with KeepPaused, pauses or resumes the monitor anyway: the
	// source changed state (a CronJob was suspended or resumed).
	SetPaused *bool
}

// Options say which settings the source leaves to the UI.
type Options struct {
	// KeepPaused: pausing in the UI sticks, because the source has no
	// paused setting.
	KeepPaused bool
}

// Changes is what a Sync wrote.
type Changes struct {
	Created, Updated int
	Saved            []monitor.Monitor // created or updated, as stored
	Deleted          []int64
}

// Empty reports whether nothing changed.
func (c Changes) Empty() bool { return len(c.Saved) == 0 && len(c.Deleted) == 0 }

// Sync makes source's monitors in the store match desired, and only writes
// the ones that changed.
func Sync(ctx context.Context, st *store.Store, source string, desired []Desired, opts Options) (Changes, error) {
	var ch Changes
	existing, err := st.ListMonitors(ctx)
	if err != nil {
		return ch, err
	}
	byName, byRef := map[string]monitor.Monitor{}, map[string]monitor.Monitor{}
	left := map[int64]monitor.Monitor{} // not desired anymore, unless matched below
	for _, m := range existing {
		if m.Source == source {
			byName[m.Name] = m
			if m.SourceRef != "" {
				byRef[m.SourceRef] = m
			}
			left[m.ID] = m
		}
	}
	for _, d := range desired {
		d.Source = source
		cur, ok := byRef[d.SourceRef]
		if d.SourceRef == "" {
			cur, ok = byName[d.Name]
		} else if !ok {
			// Saved before monitors had a ref: match it by name once.
			if c, found := byName[d.Name]; found && c.SourceRef == "" {
				cur, ok = c, true
			}
		}
		if ok {
			if _, free := left[cur.ID]; !free {
				ok = false // already taken by another desired monitor
			}
		}
		if ok && cur.Kind != d.Kind {
			// Switched between healthcheck and heartbeat: a different monitor.
			if err := st.DeleteMonitor(ctx, cur.ID); err != nil {
				return ch, err
			}
			ch.Deleted = append(ch.Deleted, cur.ID)
			delete(left, cur.ID)
			ok = false
		}
		if !ok {
			m, err := st.CreateMonitor(ctx, d.Monitor)
			if err != nil {
				return ch, err
			}
			ch.Created++
			ch.Saved = append(ch.Saved, m)
			continue
		}
		delete(left, cur.ID)
		m := d.Monitor
		m.ID, m.CreatedAt, m.UpdatedAt = cur.ID, cur.CreatedAt, cur.UpdatedAt
		// The status page editor owns whether and how it's shown; the
		// source never sets it.
		m.Public, m.StatusLabel, m.StatusOrder, m.StatusSection = cur.Public, cur.StatusLabel, cur.StatusOrder, cur.StatusSection
		if opts.KeepPaused {
			m.Paused = cur.Paused
			if d.SetPaused != nil {
				m.Paused = *d.SetPaused
			}
		}
		if d.GeneratedToken {
			m.Heartbeat.Token = cur.Heartbeat.Token // keep the ping URL
		}
		if reflect.DeepEqual(m, cur) {
			continue
		}
		saved, err := st.UpdateMonitor(ctx, m)
		if err != nil {
			return ch, err
		}
		ch.Updated++
		ch.Saved = append(ch.Saved, saved)
	}
	for _, m := range left {
		if err := st.DeleteMonitor(ctx, m.ID); err != nil {
			return ch, err
		}
		ch.Deleted = append(ch.Deleted, m.ID)
	}
	return ch, nil
}
