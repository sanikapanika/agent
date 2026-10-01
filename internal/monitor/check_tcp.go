package monitor

import (
	"errors"
	"strings"
)

func init() {
	RegisterCheckType(CheckType{
		Type:    TypeTCP,
		Label:   "TCP",
		Summary: "Queues, any open port",
		Order:   20,
		Target:  TargetSpec{Label: "Host and port", Placeholder: "rabbitmq.default.svc:5672"},
		Normalize: func(c *Check) error {
			if !strings.Contains(c.Target, ":") {
				return errors.New("target must be host:port")
			}
			return nil
		},
	})
}
