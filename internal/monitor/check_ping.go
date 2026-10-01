package monitor

import (
	"errors"
	"net"
	"strings"
)

func init() {
	RegisterCheckType(CheckType{
		Type:    TypePing,
		Label:   "Ping",
		Summary: "Host answers ICMP echo",
		Order:   15,
		Target:  TargetSpec{Label: "Host", Placeholder: "10.0.0.5 or db.internal"},
		Normalize: func(c *Check) error {
			if net.ParseIP(c.Target) != nil {
				return nil
			}
			if c.Target == "" || strings.ContainsAny(c.Target, "/: ") {
				return errors.New("target must be a hostname or IP address, without a scheme or port")
			}
			return nil
		},
	})
}
