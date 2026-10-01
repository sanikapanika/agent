// Package notify delivers alerts to external channels.
package notify

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
	"slices"
	"strings"
	"time"

	"github.com/uptimy/agent/internal/schema"
)

// Notifier is a configured alert channel.
type Notifier struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Enabled   bool      `json:"enabled"`
	Config    Config    `json:"config"`
	CreatedAt time.Time `json:"created_at"`

	// AllMonitors sends alerts for every monitor, including ones added
	// later. Otherwise only the monitors in MonitorIDs alert here.
	AllMonitors bool    `json:"all_monitors"`
	MonitorIDs  []int64 `json:"monitor_ids"`
}

// Config holds channel settings: one flat set of keys shared by all
// channels; each Channel declares the keys it uses.
type Config struct {
	URL      string `json:"url,omitempty"`       // webhook, slack, discord, teams; ntfy's server
	BotToken string `json:"bot_token,omitempty"` // telegram
	ChatID   string `json:"chat_id,omitempty"`   // telegram

	// ntfy
	Topic string `json:"topic,omitempty"`
	Token string `json:"token,omitempty"` // access token, for protected topics

	// PagerDuty
	RoutingKey string `json:"routing_key,omitempty"` // an Events API v2 integration key

	// Email
	SMTPHost     string `json:"smtp_host,omitempty"`
	SMTPPort     int    `json:"smtp_port,omitempty"`
	SMTPSecurity string `json:"smtp_security,omitempty"` // starttls (default), tls or none
	SMTPUsername string `json:"smtp_username,omitempty"`
	SMTPPassword string `json:"smtp_password,omitempty"`
	SMTPFrom     string `json:"smtp_from,omitempty"`
	SMTPTo       string `json:"smtp_to,omitempty"` // comma-separated
}

// Normalize validates the notifier.
func (n *Notifier) Normalize() error {
	n.Name = strings.TrimSpace(n.Name)
	if n.Name == "" {
		return errors.New("name is required")
	}
	if n.AllMonitors || n.MonitorIDs == nil {
		n.MonitorIDs = []int64{}
	}
	slices.Sort(n.MonitorIDs)
	n.MonitorIDs = slices.Compact(n.MonitorIDs)
	ch, ok := Lookup(n.Type)
	if !ok {
		return fmt.Errorf("unsupported notifier type %q", n.Type)
	}
	// Settings left over from another channel type would only confuse.
	cfg, err := n.Config.onlyKeys(schema.Keys(ch.Fields))
	if err != nil {
		return err
	}
	n.Config = cfg
	return ch.Validate(&n.Config)
}

func (c Config) onlyKeys(keys []string) (Config, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return c, err
	}
	for k := range all {
		if !slices.Contains(keys, k) {
			delete(all, k)
		}
	}
	raw, _ = json.Marshal(all)
	var out Config
	return out, json.Unmarshal(raw, &out)
}

// Alert is what gets sent when a monitor changes state.
type Alert struct {
	MonitorID   int64     `json:"monitor_id"`
	MonitorName string    `json:"monitor_name"`
	Kind        string    `json:"kind"`   // "healthcheck" or "heartbeat"
	Target      string    `json:"target"` // what it watches: a target, or a heartbeat's schedule
	Status      string    `json:"status"` // "up" or "down"
	Message     string    `json:"message"`
	Time        time.Time `json:"time"`
	// Test is set for "Send test" from the UI.
	Test bool `json:"test,omitempty"`
}

// Title says what happened in a few words: "Checkout API is down", "Nightly
// backup missed its run due 03:00 CEST". For subjects and headings.
func (a Alert) Title() string {
	switch {
	case a.Kind == "heartbeat" && a.Message != "":
		// A heartbeat's message already reads as what happened to the job.
		return a.MonitorName + " " + a.Message
	case a.Status == "up":
		return a.MonitorName + " is back up"
	default:
		return a.MonitorName + " is down"
	}
}

// Icon is the alert's status as an emoji.
func (a Alert) Icon() string {
	if a.Status == "up" {
		return "✅"
	}
	return "🔴"
}

// Text renders the alert as a single human-readable line.
func (a Alert) Text() string {
	switch {
	case a.Kind == "heartbeat":
		return a.Icon() + " " + a.Title()
	case a.Status == "up":
		return fmt.Sprintf("%s %s (%s)", a.Icon(), a.Title(), a.Target)
	default:
		return fmt.Sprintf("%s %s (%s): %s", a.Icon(), a.Title(), a.Target, a.Message)
	}
}

// Sender delivers alerts over HTTP.
type Sender struct {
	client *http.Client
	log    *slog.Logger
}

// NewSender returns a Sender with sensible timeouts.
func NewSender(log *slog.Logger) *Sender {
	return &Sender{client: &http.Client{Timeout: 10 * time.Second}, log: log}
}

// SendAll delivers the alert to every enabled notifier, logging failures.
func (s *Sender) SendAll(ctx context.Context, notifiers []Notifier, a Alert) {
	for _, n := range notifiers {
		if !n.Enabled {
			continue
		}
		if err := s.Send(ctx, n, a); err != nil {
			s.log.Warn("notification failed", "notifier", n.Name, "type", n.Type, "err", err)
		}
	}
}

// Send delivers one alert to one notifier.
func (s *Sender) Send(ctx context.Context, n Notifier, a Alert) error {
	ch, ok := Lookup(n.Type)
	if !ok {
		return fmt.Errorf("unsupported notifier type %q", n.Type)
	}
	return ch.Send(ctx, s, n.Config, a)
}

// PostJSON sends body as JSON to endpoint; for channels that call a webhook.
func (s *Sender) PostJSON(ctx context.Context, endpoint string, body any) error {
	return s.PostJSONWithHeaders(ctx, endpoint, body, nil)
}

// PostJSONWithHeaders is PostJSON with extra request headers, e.g. auth.
func (s *Sender) PostJSONWithHeaders(ctx context.Context, endpoint string, body any, headers map[string]string) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "uptimy-agent")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		// url.Error repeats the endpoint, which can hold a secret (Telegram's
		// bot token, a webhook's key); keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}
