package notify

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/uptimy/agent/internal/schema"
)

// uptimyWebhookPath is where an Uptimy Agent integration's ingest URL lives:
// /v1/webhooks/agent/<integration>/<secret>.
const uptimyWebhookPath = "/v1/webhooks/agent/"

// Uptimy gets the same JSON as the generic webhook; the platform turns down
// and recovery into incidents, which can trigger workflows there.
func init() {
	Register(Channel{
		Type:  "uptimy",
		Label: "Uptimy",
		Help: "In Uptimy, open Settings → Integrations, connect Uptimy Agent and copy its webhook URL. " +
			"Down and recovery alerts open and resolve incidents there, and can trigger workflows.",
		Order:    5,
		Fields:   []schema.Field{webhookURLField("https://workflows.upti.my/v1/webhooks/agent/…")},
		Validate: validateUptimyURL,
		Send: func(ctx context.Context, s *Sender, c Config, a Alert) error {
			return s.PostJSON(ctx, c.URL, a)
		},
	})
}

// validateUptimyURL also catches a pasted URL from another integration (a
// Railway one would be accepted by Uptimy and then silently ignored). Any
// host is allowed, for a local Uptimy in development.
func validateUptimyURL(c *Config) error {
	if err := validateWebhookURL(c); err != nil {
		return err
	}
	u, _ := url.Parse(c.URL)
	if !strings.HasPrefix(u.Path, uptimyWebhookPath) || len(strings.Split(strings.TrimPrefix(u.Path, uptimyWebhookPath), "/")) != 2 {
		return errors.New("paste the webhook URL of an Uptimy Agent integration (…/v1/webhooks/agent/…)")
	}
	return nil
}
