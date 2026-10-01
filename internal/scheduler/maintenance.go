package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/uptimy/agent/internal/maintenance"
	"github.com/uptimy/agent/internal/monitor"
)

// During maintenance a monitor's down alert is held back. If it recovers
// before the window ends, nobody hears about it; if it's still down when the
// window ends, the alert goes out then. Status changes are recorded either
// way.

// maintenanceCheckEvery is how soon after a window ends held alerts go out.
const maintenanceCheckEvery = 15 * time.Second

type maintenanceState struct {
	mu      sync.Mutex
	windows maintenance.Schedule
	held    map[int64]bool // monitors whose down alert was held back
}

// ReloadMaintenance reads the current and upcoming windows; call it after
// they change.
func (s *Scheduler) ReloadMaintenance(ctx context.Context) error {
	ws, err := s.store.ListMaintenance(ctx, time.Now())
	if err != nil {
		return err
	}
	s.maint.mu.Lock()
	s.maint.windows = ws
	s.maint.mu.Unlock()
	s.releaseHeldAlerts() // a window may have been ended early
	return nil
}

// InMaintenance reports whether a monitor is in a maintenance window now.
func (s *Scheduler) InMaintenance(monitorID int64) bool {
	s.maint.mu.Lock()
	defer s.maint.mu.Unlock()
	return len(s.maint.windows.Covering(monitorID, time.Now())) > 0
}

// Maintenance returns the current and upcoming windows.
func (s *Scheduler) Maintenance() maintenance.Schedule {
	s.maint.mu.Lock()
	defer s.maint.mu.Unlock()
	return append(maintenance.Schedule(nil), s.maint.windows...)
}

// holdAlert reports whether an alert about m waits: a down alert during
// maintenance, or the recovery from a down alert that was never sent.
func (s *Scheduler) holdAlert(m monitor.Monitor, status monitor.Status) bool {
	s.maint.mu.Lock()
	defer s.maint.mu.Unlock()
	switch status {
	case monitor.StatusDown:
		if len(s.maint.windows.Covering(m.ID, time.Now())) > 0 {
			s.maint.held[m.ID] = true
			return true
		}
	case monitor.StatusUp:
		if s.maint.held[m.ID] {
			delete(s.maint.held, m.ID)
			return true
		}
	}
	return false
}

// watchMaintenance sends held alerts once their window ends.
func (s *Scheduler) watchMaintenance() {
	t := time.NewTicker(maintenanceCheckEvery)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.releaseHeldAlerts()
		}
	}
}

func (s *Scheduler) releaseHeldAlerts() {
	now := time.Now()
	s.maint.mu.Lock()
	var due []int64
	for id := range s.maint.held {
		if len(s.maint.windows.Covering(id, now)) == 0 {
			due = append(due, id)
			delete(s.maint.held, id)
		}
	}
	s.maint.mu.Unlock()

	for _, id := range due {
		m, err := s.store.GetMonitor(s.ctx, id)
		if err != nil || s.Status(m) != monitor.StatusDown {
			continue // deleted, paused, or recovered meanwhile
		}
		msg := "still down after maintenance"
		if m.Kind == monitor.KindHeartbeat {
			msg = "is still down after maintenance"
		}
		s.sendAlert(m, monitor.StatusDown, msg, now.UTC())
	}
}
