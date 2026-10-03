// Package incident models what people post on the status page by hand:
// incidents, with a timeline of updates from "investigating" to "resolved",
// and notices, a message shown until it's ended. The statuses and
// severities match the hosted Uptimy status pages.
package incident

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// Kind is an incident or a notice.
type Kind string

const (
	KindIncident Kind = "incident"
	KindNotice   Kind = "notice"
)

// Status is where an incident is. A notice's updates have no status until
// it's ended (Resolved).
type Status string

const (
	Investigating Status = "investigating"
	Identified    Status = "identified"
	Monitoring    Status = "monitoring"
	Resolved      Status = "resolved"
)

// Severity is how bad an incident is; notices have none.
type Severity string

const (
	Low      Severity = "low"
	Medium   Severity = "medium"
	High     Severity = "high"
	Critical Severity = "critical"
)

// Incident is an incident or a notice.
type Incident struct {
	ID       int64    `json:"id"`
	Kind     Kind     `json:"kind"`
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

// Open reports whether it's still shown as current.
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
	case i.Kind != KindIncident && i.Kind != KindNotice:
		return errors.New("post an incident or a notice")
	case i.Title == "" || len(i.Title) > 150:
		return errors.New("give it a title of up to 150 characters")
	case i.Kind == KindNotice:
		i.Severity = ""
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

// NormalizeUpdate validates an update to an incident of kind k.
func NormalizeUpdate(k Kind, u *Update) error {
	var err error
	if u.Message, err = NormalizeMessage(u.Message); err != nil {
		return err
	}
	switch {
	case k == KindNotice && u.Status != "" && u.Status != Resolved:
		return errors.New("a notice is either shown or ended")
	case k == KindIncident && !slices.Contains([]Status{Investigating, Identified, Monitoring, Resolved}, u.Status):
		return errors.New("choose a status: investigating, identified, monitoring or resolved")
	}
	return nil
}
