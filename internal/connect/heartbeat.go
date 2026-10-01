// Package connect links the agent to Uptimy.
//
// Today this is the "watch the watcher" heartbeat: the agent pings an Uptimy
// heartbeat monitor on an interval, so if the agent, its node or the whole
// cluster disappears, Uptimy notices the silence and alerts from outside.
package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DefaultHeartbeatBase is Uptimy's heartbeat ingest endpoint.
const DefaultHeartbeatBase = "https://heartbeats.upti.my/v1/monitors/"

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{6,128}$`)

// NormalizeHeartbeatURL accepts either a full heartbeat URL or just the token
// Uptimy shows next to it, and returns the URL to ping.
func NormalizeHeartbeatURL(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("paste the heartbeat URL from Uptimy")
	}
	if tokenPattern.MatchString(input) {
		return DefaultHeartbeatBase + input, nil
	}
	u, err := url.Parse(input)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", errors.New("that doesn't look like a heartbeat URL; it should start with " + DefaultHeartbeatBase)
	}
	return u.String(), nil
}

// CountsFunc reports how many monitors are currently up and down.
type CountsFunc func() (up, down int)

// Status is what the UI shows about the connection.
type Status struct {
	Enabled      bool       `json:"enabled"`
	URL          string     `json:"url,omitempty"`
	ManagedByEnv bool       `json:"managed_by_env"`
	Interval     int        `json:"interval_seconds"`
	LastPingAt   *time.Time `json:"last_ping_at"`
	LastOK       bool       `json:"last_ok"`
	LastError    string     `json:"last_error,omitempty"`
	// Paused: the heartbeat is paused in Uptimy, so the agent doesn't check in.
	Paused bool `json:"paused"`
}

// Watchdog pings an Uptimy heartbeat URL on an interval. The URL can be set,
// changed or cleared at runtime from the UI.
type Watchdog struct {
	interval     time.Duration
	version      string
	counts       CountsFunc
	log          *slog.Logger
	managedByEnv bool
	client       *http.Client

	mu        sync.Mutex
	url       string
	restart   chan struct{}
	lastPing  time.Time
	lastOK    bool
	lastError string
	paused    bool
}

// NewWatchdog creates a watchdog pinging url ("" = off). managedByEnv marks a
// URL that came from UPTIMY_HEARTBEAT_URL, which the UI shows as read-only.
func NewWatchdog(url string, managedByEnv bool, interval time.Duration, version string, counts CountsFunc, log *slog.Logger) *Watchdog {
	return &Watchdog{
		interval: interval, version: version, counts: counts, log: log,
		managedByEnv: managedByEnv,
		url:          url,
		restart:      make(chan struct{}, 1),
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// ManagedByEnv reports whether UPTIMY_HEARTBEAT_URL controls the URL.
func (w *Watchdog) ManagedByEnv() bool { return w.managedByEnv }

// SetURL changes the heartbeat URL without verifying it ("" disables it).
func (w *Watchdog) SetURL(u string) {
	w.mu.Lock()
	w.url = u
	w.lastPing, w.lastOK, w.lastError = time.Time{}, false, ""
	w.paused = false
	w.mu.Unlock()
	select {
	case w.restart <- struct{}{}:
	default:
	}
}

// Connect sends a check-in to target and, only if Uptimy accepts it, switches
// the watchdog to that URL. A wrong URL or token is rejected up front instead
// of being saved and failing quietly every minute.
func (w *Watchdog) Connect(ctx context.Context, target string) error {
	if err := w.ping(ctx, target); err != nil {
		return err
	}
	w.mu.Lock()
	w.url = target
	w.lastPing, w.lastOK, w.lastError = time.Now().UTC(), true, ""
	w.paused = false
	w.mu.Unlock()
	select {
	case w.restart <- struct{}{}:
	default:
	}
	return nil
}

// SetPaused stops (or resumes) checking in while the heartbeat is paused in
// Uptimy, which rejects check-ins to a paused heartbeat. Resuming checks in
// right away so Uptimy sees the agent again immediately.
func (w *Watchdog) SetPaused(ctx context.Context, paused bool) {
	w.mu.Lock()
	changed := w.paused != paused
	w.paused = paused
	if paused {
		w.lastError = ""
	}
	w.mu.Unlock()
	if changed && !paused {
		_ = w.PingNow(ctx) // failures are recorded in the status
	}
}

// Status returns the current connection state.
func (w *Watchdog) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := Status{
		Enabled: w.url != "", URL: w.url, ManagedByEnv: w.managedByEnv,
		Interval: int(w.interval.Seconds()), LastOK: w.lastOK, LastError: w.lastError,
		Paused: w.paused,
	}
	if !w.lastPing.IsZero() {
		t := w.lastPing
		s.LastPingAt = &t
	}
	return s
}

// Run pings until ctx is canceled. A failed ping is exactly the signal Uptimy
// is watching for, so failures are recorded and logged but never fatal.
func (w *Watchdog) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	_ = w.PingNow(ctx) // failures are recorded in the status
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.restart:
			// The URL changed: Connect has just checked in (or it was cleared),
			// so restart the interval rather than pinging again.
			t.Reset(w.interval)
			continue
		case <-t.C:
		}
		_ = w.PingNow(ctx) // failures are recorded in the status
	}
}

// PingNow sends one heartbeat immediately (if a URL is set) and returns the
// outcome. It is also used by the UI's "Send test ping".
func (w *Watchdog) PingNow(ctx context.Context) error {
	w.mu.Lock()
	target, paused := w.url, w.paused
	w.mu.Unlock()
	if target == "" {
		return errors.New("no heartbeat URL is set")
	}
	if paused {
		return errors.New("alerts are paused in Uptimy; resume them to check in")
	}
	err := w.ping(ctx, target)
	if ctx.Err() != nil {
		return err
	}

	w.mu.Lock()
	if w.url == target { // ignore results for a URL that was replaced mid-flight
		w.lastPing, w.lastOK, w.lastError = time.Now().UTC(), err == nil, ""
		if err != nil {
			w.lastError = err.Error()
		}
	}
	w.mu.Unlock()
	if err != nil {
		w.log.Warn("uptimy heartbeat failed", "err", err)
	}
	return err
}

func (w *Watchdog) ping(ctx context.Context, target string) error {
	up, down := w.counts()
	body, _ := json.Marshal(map[string]any{
		"source":        "uptimy-agent",
		"version":       w.version,
		"monitors_up":   up,
		"monitors_down": down,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "uptimy-agent/"+w.version)
	resp, err := w.client.Do(req)
	if err != nil {
		// url.Error embeds the request URL, and the URL carries the secret
		// heartbeat token; keep only the underlying cause for display.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("couldn't reach Uptimy: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return errors.New("Uptimy rejected the check-in (404): the heartbeat is paused or was deleted there")
	case resp.StatusCode >= 300:
		return fmt.Errorf("Uptimy returned HTTP %d", resp.StatusCode)
	}
	return nil
}
