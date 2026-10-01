package monitor

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/uptimy/agent/internal/schema"
)

var dnsRecordTypes = []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS"}

func init() {
	RegisterCheckType(CheckType{
		Type:    TypeDNS,
		Label:   "DNS",
		Summary: "Record resolves as expected",
		Order:   30,
		Target:  TargetSpec{Label: "Hostname", Placeholder: "example.com"},
		Fields: []schema.Field{
			{Key: "record_type", Label: "Record", Input: schema.Select, Options: dnsRecordTypes, Default: "A"},
			{Key: "expected", Label: "Expected value", Input: schema.Text, Hint: "Optional, substring match"},
			{Key: "resolver", Label: "Resolver", Input: schema.Text, Hint: "Optional, e.g. 1.1.1.1"},
		},
		Normalize: func(c *Check) error {
			if c.Target == "" {
				return errors.New("target hostname is required")
			}
			c.Config.RecordType = strings.ToUpper(c.Config.RecordType)
			if c.Config.RecordType == "" {
				c.Config.RecordType = "A"
			}
			if !slices.Contains(dnsRecordTypes, c.Config.RecordType) {
				return fmt.Errorf("unsupported record type %q", c.Config.RecordType)
			}
			return nil
		},
	})
}
