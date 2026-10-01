package notify

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/uptimy/agent/internal/schema"
)

// pagerDutyURL is the Events API v2 endpoint; a variable for tests.
var pagerDutyURL = "https://events.pagerduty.com/v2/enqueue"

func init() {
	Register(Channel{
		Type:  "pagerduty",
		Label: "PagerDuty",
		Help: "In a PagerDuty service, add an \"Events API V2\" integration and paste its integration key. " +
			"A monitor going down opens an incident; recovering resolves it.",
		Order: 60,
		Fields: []schema.Field{
			{Key: "routing_key", Label: "Integration key", Input: schema.Password, Required: true, Wide: true},
		},
		Validate: func(c *Config) error {
			c.RoutingKey = strings.TrimSpace(c.RoutingKey)
			if len(c.RoutingKey) != 32 {
				return errors.New("the integration key is the 32-character key of an Events API V2 integration")
			}
			return nil
		},
		Send: func(ctx context.Context, s *Sender, c Config, a Alert) error {
			// One incident per monitor: the dedup key ties the trigger to its
			// resolve, and repeated triggers to the same incident.
			key := "uptimy-agent/" + strconv.FormatInt(a.MonitorID, 10)
			if a.Test {
				key = "uptimy-agent/test"
			}
			action := "trigger"
			if a.Status == "up" {
				action = "resolve"
			}
			if err := s.PostJSON(ctx, pagerDutyURL, pagerDutyEvent(c, a, key, action)); err != nil {
				return err
			}
			// A test shouldn't leave an incident open for someone to close.
			if a.Test {
				return s.PostJSON(ctx, pagerDutyURL, pagerDutyEvent(c, a, key, "resolve"))
			}
			return nil
		},
	})
}

func pagerDutyEvent(c Config, a Alert, key, action string) map[string]any {
	source := a.Target
	if source == "" {
		source = "Uptimy Agent"
	}
	summary := a.Title()
	if len(summary) > 1024 {
		summary = summary[:1024]
	}
	return map[string]any{
		"routing_key":  c.RoutingKey,
		"event_action": action,
		"dedup_key":    key,
		"payload": map[string]any{
			"summary":        summary,
			"source":         source,
			"severity":       "critical",
			"timestamp":      a.Time.UTC().Format("2006-01-02T15:04:05.000Z"),
			"component":      a.MonitorName,
			"custom_details": map[string]string{"message": a.Message, "kind": a.Kind},
		},
	}
}
