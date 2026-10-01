package monitor

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Heartbeat is a heartbeat's settings: when the job is expected to run, and
// how late it may be before it counts as missed. The job reports in by
// calling its ping URL (/ping/<token>).
type Heartbeat struct {
	Token string `json:"token"`
	// The schedule: every EverySeconds, or on a Cron expression in Timezone.
	EverySeconds int    `json:"every_seconds,omitempty"`
	Cron         string `json:"cron,omitempty"`
	Timezone     string `json:"timezone,omitempty"` // IANA name; default UTC
	// GraceSeconds is how late a run may be before it counts as missed.
	GraceSeconds int `json:"grace_seconds"`
}

const (
	minEvery     = time.Minute
	maxEvery     = 366 * 24 * time.Hour
	defaultGrace = 5 * time.Minute
	minGrace     = time.Minute
	maxGrace     = 7 * 24 * time.Hour
)

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// cronParser accepts standard five-field expressions and the @daily-style
// shortcuts. Seconds aren't supported: heartbeats are at most once a minute.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// NewToken returns a new random ping token.
func NewToken() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b) // never fails (crypto/rand panics instead)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Grace returns the grace period.
func (h Heartbeat) Grace() time.Duration { return time.Duration(h.GraceSeconds) * time.Second }

func (h *Heartbeat) normalize() error {
	h.Token = strings.TrimSpace(h.Token)
	h.Cron = strings.Join(strings.Fields(h.Cron), " ")
	h.Timezone = strings.TrimSpace(h.Timezone)
	if !tokenPattern.MatchString(h.Token) {
		return errors.New("the ping token must be 16 to 64 letters, digits, - or _")
	}
	switch {
	case h.Cron != "" && h.EverySeconds != 0:
		return errors.New("set either an interval or a cron expression, not both")
	case h.Cron == "" && h.EverySeconds == 0:
		return errors.New("set a schedule: an interval or a cron expression")
	case h.EverySeconds != 0:
		if every := time.Duration(h.EverySeconds) * time.Second; every < minEvery || every > maxEvery {
			return errors.New("the interval must be between 1 minute and 366 days")
		}
		h.Timezone = ""
	default:
		if strings.HasPrefix(h.Cron, "TZ=") || strings.HasPrefix(h.Cron, "CRON_TZ=") {
			return errors.New("set the time zone separately, not in the cron expression")
		}
		if _, err := cronParser.Parse(h.Cron); err != nil {
			return fmt.Errorf("invalid cron expression %q: %w", h.Cron, err)
		}
		if h.Timezone == "" {
			h.Timezone = "UTC"
		}
		if _, err := time.LoadLocation(h.Timezone); err != nil {
			return fmt.Errorf("unknown time zone %q", h.Timezone)
		}
	}
	if h.GraceSeconds == 0 {
		h.GraceSeconds = int(defaultGrace.Seconds())
	}
	if g := h.Grace(); g < minGrace || g > maxGrace {
		return errors.New("the grace period must be between 1 minute and 7 days")
	}
	return nil
}

// NextDue returns when the next run is due after the given time: an
// interval after it, or the next time the cron expression fires.
func (h Heartbeat) NextDue(after time.Time) time.Time {
	if h.EverySeconds != 0 {
		return after.Add(time.Duration(h.EverySeconds) * time.Second)
	}
	sched, err := cronParser.Parse(h.Cron)
	if err != nil {
		return time.Time{} // unreachable after normalize
	}
	loc, err := time.LoadLocation(h.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return sched.Next(after.In(loc)).UTC()
}

// Upcoming returns the next n due times after from, for previews.
func (h Heartbeat) Upcoming(from time.Time, n int) []time.Time {
	out := make([]time.Time, 0, n)
	for t := from; len(out) < n; {
		t = h.NextDue(t)
		if t.IsZero() {
			break
		}
		out = append(out, t)
	}
	return out
}

// Describe returns the schedule in words, e.g. "every 5 minutes" or
// "0 3 * * * (Europe/Berlin)".
func (h Heartbeat) Describe() string {
	if h.EverySeconds != 0 {
		return "every " + describeInterval(time.Duration(h.EverySeconds)*time.Second)
	}
	if h.Timezone == "" || h.Timezone == "UTC" {
		return h.Cron + " (UTC)"
	}
	return fmt.Sprintf("%s (%s)", h.Cron, h.Timezone)
}

func describeInterval(d time.Duration) string {
	unit := func(n int64, one, many string) string {
		if n == 1 {
			return one
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	switch day := 24 * time.Hour; {
	case d%day == 0:
		return unit(int64(d/day), "day", "days")
	case d%time.Hour == 0:
		return unit(int64(d/time.Hour), "hour", "hours")
	default:
		return unit(int64(d/time.Minute), "minute", "minutes")
	}
}

// HeartbeatState is where a heartbeat stands against its schedule.
type HeartbeatState string

const (
	// StateWaiting: no run yet, and the first isn't overdue.
	StateWaiting HeartbeatState = "waiting"
	// StateOnTime: the last run succeeded and the next isn't due yet.
	StateOnTime HeartbeatState = "on_time"
	// StateLate: a run is due but still inside the grace period.
	StateLate HeartbeatState = "late"
	// StateMissed: a run didn't arrive within the grace period.
	StateMissed HeartbeatState = "missed"
	// StateFailed: the last run reported a failure.
	StateFailed HeartbeatState = "failed"
	StatePaused HeartbeatState = "paused"
)

// Status maps the state onto the status shared with healthchecks: a late
// run isn't an outage yet; a missed or failed one is.
func (s HeartbeatState) Status() Status {
	switch s {
	case StateOnTime, StateLate:
		return StatusUp
	case StateMissed, StateFailed:
		return StatusDown
	case StatePaused:
		return StatusPaused
	default:
		return StatusPending
	}
}

// RunOutcome is how a heartbeat run ended.
type RunOutcome string

const (
	OutcomeRunning RunOutcome = "running" // started (/start), not finished yet
	OutcomeSuccess RunOutcome = "success"
	OutcomeFailure RunOutcome = "failure" // the job reported a failure
	OutcomeMissed  RunOutcome = "missed"  // nothing arrived within the grace period
)

// Run is one expected run of a heartbeat's job.
type Run struct {
	ID        int64 `json:"id"`
	MonitorID int64 `json:"monitor_id"`
	// DueAt is when the run was due, by the schedule.
	DueAt      *time.Time `json:"due_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Outcome    RunOutcome `json:"outcome"`
	// OnTime: finished by DueAt plus the grace period.
	OnTime     bool   `json:"on_time"`
	DurationMS *int64 `json:"duration_ms"`
	Message    string `json:"message"`
}
