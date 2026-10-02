// Package managed keeps monitors that are defined outside the UI (in the
// monitors file, or discovered in Kubernetes) in step with the store. Each
// source owns its monitors, matched by name; monitors from the UI or another
// source are never touched.
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

// Sync makes source's monitors in the store match desired, matching by name,
// and only writes the ones that changed.
func Sync(ctx context.Context, st *store.Store, source string, desired []Desired, opts Options) (Changes, error) {
	var ch Changes
	existing, err := st.ListMonitors(ctx)
	if err != nil {
		return ch, err
	}
	byName := map[string]monitor.Monitor{}
	for _, m := range existing {
		if m.Source == source {
			byName[m.Name] = m
		}
	}
	for _, d := range desired {
		d.Source = source
		cur, ok := byName[d.Name]
		if ok && cur.Kind != d.Kind {
			// Switched between healthcheck and heartbeat: a different monitor.
			if err := st.DeleteMonitor(ctx, cur.ID); err != nil {
				return ch, err
			}
			ch.Deleted = append(ch.Deleted, cur.ID)
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
		delete(byName, d.Name)
		m := d.Monitor
		m.ID, m.CreatedAt, m.UpdatedAt = cur.ID, cur.CreatedAt, cur.UpdatedAt
		// The status page editor owns whether and how it's shown; the
		// source never sets it.
		m.Public, m.StatusLabel, m.StatusOrder, m.StatusSection = cur.Public, cur.StatusLabel, cur.StatusOrder, cur.StatusSection
		if opts.KeepPaused {
			m.Paused = cur.Paused
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
	for _, m := range byName {
		if err := st.DeleteMonitor(ctx, m.ID); err != nil {
			return ch, err
		}
		ch.Deleted = append(ch.Deleted, m.ID)
	}
	return ch, nil
}
