// Package incident models incidents people post on the status page by hand,
// with a timeline of updates from "investigating" to "resolved". The
// statuses and severities match the hosted Uptimy status pages.
package incident

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// Status is where an incident is.
type Status string

const (
	Investigating Status = "investigating"
	Identified    Status = "identified"
	Monitoring    Status = "monitoring"
	Resolved      Status = "resolved"
)

// Severity is how bad an incident is.
type Severity string

const (
	Low      Severity = "low"
	Medium   Severity = "medium"
	High     Severity = "high"
	Critical Severity = "critical"
)

// Incident is one incident and its timeline.
type Incident struct {
	ID       int64    `json:"id"`
	Title    string   `json:"title"`
	Severity Severity `json:"severity"`
	// Status is the latest update's.
	Status Status `json:"status"`
	// MonitorIDs are the affected monitors; empty names none in particular.
	MonitorIDs []int64    `json:"monitor_ids"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
	// Updates are newest first.
	Updates []Update `json:"updates"`
}

// Update is one entry in the timeline.
type Update struct {
	ID        int64     `json:"id"`
	Status    Status    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// Open reports whether it isn't resolved.
func (i Incident) Open() bool { return i.ResolvedAt == nil }

// Normalize validates the incident's own fields and cleans them up.
func (i *Incident) Normalize() error {
	i.Title = strings.TrimSpace(i.Title)
	if i.MonitorIDs == nil {
		i.MonitorIDs = []int64{}
	}
	slices.Sort(i.MonitorIDs)
	i.MonitorIDs = slices.Compact(i.MonitorIDs)
	switch {
	case i.Title == "" || len(i.Title) > 150:
		return errors.New("give the incident a title of up to 150 characters")
	case !slices.Contains([]Severity{Low, Medium, High, Critical}, i.Severity):
		return errors.New("choose a severity: low, medium, high or critical")
	}
	return nil
}

// NormalizeMessage validates an update's message and cleans it up.
func NormalizeMessage(m string) (string, error) {
	m = strings.TrimSpace(m)
	switch {
	case m == "":
		return m, errors.New("write a message")
	case len(m) > 5000:
		return m, errors.New("keep the message under 5000 characters")
	}
	return m, nil
}

// NormalizeUpdate validates an update.
func NormalizeUpdate(u *Update) error {
	var err error
	if u.Message, err = NormalizeMessage(u.Message); err != nil {
		return err
	}
	switch {
	case !slices.Contains([]Status{Investigating, Identified, Monitoring, Resolved}, u.Status):
		return errors.New("choose a status: investigating, identified, monitoring or resolved")
	}
	return nil
}
