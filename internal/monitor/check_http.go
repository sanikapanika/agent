package monitor

import (
	"errors"
	"net/url"
	"strings"

	"github.com/uptimy/agent/internal/schema"
)

func init() {
	RegisterCheckType(CheckType{
		Type:    TypeHTTP,
		Label:   "HTTP",
		Summary: "Websites and APIs",
		Order:   10,
		Target:  TargetSpec{Label: "URL", Placeholder: "https://api.example.com/health"},
		Fields: []schema.Field{
			{Key: "method", Label: "Method", Input: schema.Select, Options: []string{"GET", "HEAD", "POST", "PUT", "OPTIONS"}, Default: "GET"},
			{Key: "expected_status", Label: "Accepted status codes", Input: schema.Text, Placeholder: "200-399", Hint: "e.g. 200-299,301"},
			{Key: "keyword", Label: "Response must contain", Input: schema.Text, Hint: "Optional keyword check", Wide: true},
			{Key: "ignore_tls", Label: "Ignore TLS certificate errors", Input: schema.Switch, Wide: true},
		},
		// Request headers (e.g. auth) are set in YAML; they may carry secrets.
		ExtraKeys: []string{"headers"},
		Normalize: func(c *Check) error {
			u, err := url.Parse(c.Target)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return errors.New("target must be an http(s) URL")
			}
			c.Config.Method = strings.ToUpper(c.Config.Method)
			if c.Config.Method == "" {
				c.Config.Method = "GET"
			}
			if c.Config.ExpectedStatus == "" {
				c.Config.ExpectedStatus = "200-399"
			}
			_, err = ParseStatusRanges(c.Config.ExpectedStatus)
			return err
		},
	})
}
