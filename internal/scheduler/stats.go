package scheduler

import (
	"sync"
	"time"

	"github.com/uptimy/agent/internal/monitor"
)

// Stats is what a monitor has done since the agent started, for /metrics.
// Counters start at zero on each start, as Prometheus counters do; they
// survive edits to the monitor.
type Stats struct {
	// Healthchecks.
	Successes, Failures uint64
	LastLatency         time.Duration
	LastCheck           time.Time
	// Heartbeats: runs by outcome.
	Runs map[monitor.RunOutcome]uint64
}

type statsTable struct {
	mu sync.Mutex
	by map[int64]*Stats
}

func (t *statsTable) get(id int64) *Stats {
	if t.by == nil {
		t.by = map[int64]*Stats{}
	}
	st, ok := t.by[id]
	if !ok {
		st = &Stats{Runs: map[monitor.RunOutcome]uint64{}}
		t.by[id] = st
	}
	return st
}

func (s *Scheduler) countCheck(id int64, ok bool, latency time.Duration, at time.Time) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	st := s.stats.get(id)
	if ok {
		st.Successes++
	} else {
		st.Failures++
	}
	st.LastLatency, st.LastCheck = latency, at
}

func (s *Scheduler) countRun(id int64, outcome monitor.RunOutcome) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.stats.get(id).Runs[outcome]++
}

func (s *Scheduler) forgetStats(id int64) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	delete(s.stats.by, id)
}

// Stats returns a copy of every monitor's counters.
func (s *Scheduler) Stats() map[int64]Stats {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	out := make(map[int64]Stats, len(s.stats.by))
	for id, st := range s.stats.by {
		c := *st
		c.Runs = make(map[monitor.RunOutcome]uint64, len(st.Runs))
		for k, v := range st.Runs {
			c.Runs[k] = v
		}
		out[id] = c
	}
	return out
}
