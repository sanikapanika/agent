package monitor

import (
	"errors"

	"github.com/uptimy/agent/internal/schema"
)

func init() {
	RegisterCheckType(CheckType{
		Type:    TypeTLS,
		Label:   "TLS cert",
		Summary: "Certificate expiry",
		Order:   40,
		Target:  TargetSpec{Label: "Host", Placeholder: "example.com or example.com:8443"},
		Fields: []schema.Field{
			{Key: "min_days_remaining", Label: "Alert when fewer than this many days remain", Input: schema.Number, Default: 14, Wide: true},
		},
		Normalize: func(c *Check) error {
			if c.Target == "" {
				return errors.New("target host is required")
			}
			if c.Config.MinDaysRemaining == 0 {
				c.Config.MinDaysRemaining = 14
			}
			if c.Config.MinDaysRemaining < 1 {
				return errors.New("days remaining must be at least 1")
			}
			return nil
		},
	})
}
