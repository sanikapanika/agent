package statuspage

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/uptimy/agent/internal/monitor"
)

func TestValidate(t *testing.T) {
	p := Defaults()
	p.Title, p.AccentColor, p.Domain = "  Acme  ", "#33A36B", " https://Status.Acme.com/ "
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.Title != "Acme" || p.AccentColor != "#33a36b" || p.Domain != "status.acme.com" {
		t.Fatalf("not cleaned up: %+v", p)
	}

	for name, change := range map[string]func(*Settings){
		"no title":          func(p *Settings) { p.Title = " " },
		"bad color":         func(p *Settings) { p.AccentColor = "green" },
		"javascript link":   func(p *Settings) { p.WebsiteURL = "javascript:alert(1)" },
		"duplicate section": func(p *Settings) { p.Sections = append(p.Sections, p.Sections[0]) },
		"unnamed section":   func(p *Settings) { p.Sections[0].Name = "" },
		"domain with space": func(p *Settings) { p.Domain = "status acme.com" },
		"bare name domain":  func(p *Settings) { p.Domain = "status" },
	} {
		p := Defaults()
		change(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestNormalizeHost(t *testing.T) {
	for in, want := range map[string]string{
		"status.acme.com":               "status.acme.com",
		"Status.Acme.com:8080":          "status.acme.com",
		"https://status.acme.com/x?y=1": "status.acme.com",
		"status.acme.com.":              "status.acme.com",
		"[::1]:8080":                    "[::1]",
		"":                              "",
	} {
		if got := NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSectionsAndOrder(t *testing.T) {
	p := Defaults()
	p.Sections = append(p.Sections, Section{ID: "db", Name: "Databases"})
	if p.SectionOf(monitor.Monitor{StatusSection: "db"}) != "db" ||
		p.SectionOf(monitor.Monitor{StatusSection: "gone"}) != "services" {
		t.Fatal("a monitor in an unknown section belongs in the first")
	}
	// Like on the platform, a page can have no sections, and shows nothing.
	empty := Defaults()
	empty.Sections = []Section{}
	if err := empty.Validate(); err != nil || empty.SectionOf(monitor.Monitor{StatusSection: "db"}) != "" {
		t.Fatalf("no sections: %v", err)
	}

	got := Ordered([]monitor.Monitor{
		{Name: "zeta"}, {Name: "Alpha"}, {Name: "second", StatusOrder: 2}, {Name: "first", StatusOrder: 1},
	})
	var names []string
	for _, m := range got {
		names = append(names, m.Name)
	}
	if strings.Join(names, ",") != "first,second,Alpha,zeta" {
		t.Fatalf("order: %v", names)
	}
}

func TestNewLogo(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	if l, err := NewLogo(png); err != nil || l.Type != "image/png" || l.Version == 0 {
		t.Fatalf("png: %+v %v", l, err)
	}
	if l, err := NewLogo([]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`)); err != nil || l.Type != "image/svg+xml" {
		t.Fatalf("svg: %+v %v", l, err)
	}
	if _, err := NewLogo([]byte("<html><script>")); err == nil {
		t.Fatal("html accepted as a logo")
	}
	if _, err := NewLogo(append(png, make([]byte, MaxLogoBytes)...)); !errors.Is(err, ErrLogoTooLarge) {
		t.Fatalf("too large: %v", err)
	}
}

func TestAnnouncementShownAt(t *testing.T) {
	now := time.Now()
	until := now.Add(time.Hour)
	var none *Announcement
	if none.ShownAt(now) {
		t.Error("nil is shown")
	}
	a := &Announcement{Title: "x", ShowUntil: &until}
	if !a.ShownAt(now) || a.ShownAt(until) {
		t.Error("show until")
	}
	if !(&Announcement{Title: "x"}).ShownAt(now.Add(24 * 365 * time.Hour)) {
		t.Error("without an end it stays")
	}
}
