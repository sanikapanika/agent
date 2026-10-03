// Package statuspage is the public status page's model: its settings,
// sections, logos and the order monitors are shown in. Which monitors appear,
// and under what name, is stored on each monitor (monitor.Monitor's Public
// and Status* fields), so it follows the monitor and is deleted with it.
//
// Branding mirrors the hosted Uptimy status pages (upti.my-status): logo in
// place of the title, "Visit website", accent color on section headings.
package statuspage

import (
	"errors"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/uptimy/agent/internal/monitor"
)

// Settings configure the page.
type Settings struct {
	Enabled     bool   `json:"enabled"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ShowEvents  bool   `json:"show_events"`

	// AccentColor is a #rrggbb color for section headings; empty = Uptimy green.
	AccentColor string `json:"accent_color"`
	// WebsiteURL adds a "Visit website" link to the header.
	WebsiteURL string `json:"website_url"`
	// HideUptimyCard drops the Uptimy card from the footer, as white-label
	// does on hosted pages; the "Powered by Uptimy" line stays.
	HideUptimyCard bool `json:"hide_uptimy_card"`
	// Domain is a hostname, e.g. status.example.com, that serves only the
	// status page: at "/", with no sign-in, API or metrics. Pointing it at the
	// agent (DNS, TLS, an Ingress) is up to whoever runs it. Empty = none.
	Domain string `json:"domain"`
	// Sections group monitors, in display order. A monitor on the page with
	// no (known) section goes in the first; with no sections, nothing shows.
	Sections []Section `json:"sections"`
}

// Section is a named group of monitors.
type Section struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Defaults are the settings of a page nobody has edited: on, with one
// section, so a new agent has a working status page right away.
func Defaults() Settings {
	return Settings{
		Enabled:    true,
		Title:      "Service Status",
		ShowEvents: true,
		Sections:   []Section{{ID: "services", Name: "Services"}},
	}
}

var (
	hexColor  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	sectionID = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	hostname  = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

// Validate cleans up and checks the settings.
func (p *Settings) Validate() error {
	p.Title = strings.TrimSpace(p.Title)
	p.Description = strings.TrimSpace(p.Description)
	p.AccentColor = strings.ToLower(strings.TrimSpace(p.AccentColor))
	p.WebsiteURL = strings.TrimSpace(p.WebsiteURL)
	p.Domain = NormalizeHost(p.Domain)
	switch {
	case p.Title == "":
		return errors.New("the status page needs a title")
	case len(p.Title) > 100 || len(p.Description) > 500:
		return errors.New("keep the title under 100 characters and the description under 500")
	case p.AccentColor != "" && !hexColor.MatchString(p.AccentColor):
		return errors.New("the accent color must look like #33a36b")
	case p.Domain != "" && (len(p.Domain) > 253 || !hostname.MatchString(p.Domain)):
		return errors.New("the domain must be a hostname like status.example.com, without https:// or a path")
	case len(p.Sections) > 20:
		return errors.New("use at most 20 sections")
	}
	if p.WebsiteURL != "" {
		u, err := url.Parse(p.WebsiteURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(p.WebsiteURL) > 300 {
			return errors.New("the website link must be an http(s) address, e.g. https://example.com")
		}
	}
	seen := map[string]bool{}
	for i := range p.Sections {
		sec := &p.Sections[i]
		sec.Name = strings.TrimSpace(sec.Name)
		if sec.Name == "" || len(sec.Name) > 60 {
			return errors.New("give every section a name of up to 60 characters")
		}
		if !sectionID.MatchString(sec.ID) || seen[sec.ID] {
			return errors.New("invalid section; reload and try again")
		}
		seen[sec.ID] = true
	}
	return nil
}

// NormalizeHost turns a pasted address or a request's Host into a bare
// lowercase hostname: "https://Status.Example.com:443/" is
// "status.example.com".
func NormalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	if i := strings.LastIndexByte(h, ':'); i >= 0 && !strings.Contains(h[i:], "]") {
		h = h[:i]
	}
	return strings.TrimSuffix(h, ".")
}

// HasSection reports whether id is one of the page's sections.
func (p Settings) HasSection(id string) bool {
	for _, sec := range p.Sections {
		if sec.ID == id {
			return true
		}
	}
	return false
}

// SectionOf returns the ID of the section m is shown in, or "" if the page
// has no sections.
func (p Settings) SectionOf(m monitor.Monitor) string {
	switch {
	case p.HasSection(m.StatusSection):
		return m.StatusSection
	case len(p.Sections) > 0:
		return p.Sections[0].ID
	default:
		return ""
	}
}

// Ordered sorts monitors the way the page shows them: by the editor's order,
// with never-arranged monitors (order 0) last, by name.
func Ordered(monitors []monitor.Monitor) []monitor.Monitor {
	out := append([]monitor.Monitor(nil), monitors...)
	rank := func(m monitor.Monitor) int {
		if m.StatusOrder == 0 {
			return math.MaxInt
		}
		return m.StatusOrder
	}
	sort.SliceStable(out, func(i, j int) bool {
		if rank(out[i]) != rank(out[j]) {
			return rank(out[i]) < rank(out[j])
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}
