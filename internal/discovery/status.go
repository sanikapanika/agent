package discovery

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/store"
)

// Status is how discovery is doing, for the UI: what the last scan found
// and what it couldn't use.
type Status struct {
	// Scope is "cluster", or "namespace <name>" when RBAC limits it.
	Scope    string    `json:"scope"`
	LastScan time.Time `json:"last_scan,omitzero"`
	// Error is why the last scan failed; it then kept what it had found.
	Error    string   `json:"error,omitempty"`
	Monitors int      `json:"monitors"`
	Warnings []string `json:"warnings"`
}

// Status returns the latest scan's status.
func (d *Discoverer) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.status
	st.Warnings = slices.Clone(st.Warnings)
	if st.Warnings == nil {
		st.Warnings = []string{}
	}
	return st
}

func (d *Discoverer) finishScan(monitors int, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status.LastScan = time.Now().UTC()
	d.status.Error = ""
	if err != nil {
		d.status.Error = err.Error()
	} else {
		d.status.Monitors = monitors
	}
	d.status.Warnings = append(slices.Clone(d.scanWarnings), d.jobWarnings...)
}

func (d *Discoverer) publishWarnings() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status.Warnings = append(slices.Clone(d.scanWarnings), d.jobWarnings...)
}

// Resource is an object discovery could monitor, for the resource browser.
type Resource struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// Label is its upti.my/monitor value, if it has one, and LabelState
	// what that means: "in", "out", "invalid", or "" without the label.
	Label      string            `json:"label,omitempty"`
	LabelState string            `json:"label_state"`
	Monitors   []ResourceMonitor `json:"monitors"`
}

// ResourceMonitor is a monitor discovered from a Resource.
type ResourceMonitor struct {
	ID   int64        `json:"id"`
	Kind monitor.Kind `json:"kind"`
	Name string       `json:"name"`
}

// Resources lists what discovery could monitor.
type Resources struct {
	Items []Resource `json:"items"`
	// Truncated names the kinds with more objects than were listed.
	Truncated []string `json:"truncated"`
}

// maxListed bounds each kind's list in the resource browser.
const maxListed = 500

// Resources lists the cluster's objects of every kind discovery handles,
// labeled or not, with the monitors made from them. It reads the API on
// demand, for the resource browser; scans stay label-filtered.
func (d *Discoverer) Resources(ctx context.Context, st *store.Store) (Resources, error) {
	out := Resources{Items: []Resource{}, Truncated: []string{}}
	all, err := st.ListMonitors(ctx)
	if err != nil {
		return out, err
	}
	byObject := map[string][]ResourceMonitor{} // "kind/namespace/name"
	for _, m := range all {
		if m.Source != monitor.SourceKubernetes || m.SourceRef == "" {
			continue
		}
		key, _, _ := strings.Cut(m.SourceRef, "#")
		byObject[key] = append(byObject[key], ResourceMonitor{ID: m.ID, Kind: m.Kind, Name: m.Name})
	}
	for _, r := range resources {
		d.mu.Lock()
		clusterWide := !d.namespaced[r.name]
		d.mu.Unlock()
		var items []object
		items, _, err = d.get(ctx, r, "?limit="+strconv.Itoa(maxListed), clusterWide)
		if errors.Is(err, errForbidden) {
			continue
		}
		if err != nil {
			return out, err
		}
		if len(items) >= maxListed {
			out.Truncated = append(out.Truncated, r.kind)
		}
		for _, o := range items {
			key := r.kind + "/" + o.Metadata.Namespace + "/" + o.Metadata.Name
			monitors := byObject[key]
			if monitors == nil {
				monitors = []ResourceMonitor{}
			}
			res := Resource{
				Kind: r.kind, Namespace: o.Metadata.Namespace, Name: o.Metadata.Name,
				Monitors: monitors,
			}
			if v, labeled := o.Metadata.Labels[Label]; labeled {
				res.Label = v
				switch in, valid := optIn(v); {
				case !valid:
					res.LabelState = "invalid"
				case in:
					res.LabelState = "in"
				default:
					res.LabelState = "out"
				}
			}
			out.Items = append(out.Items, res)
		}
	}
	return out, nil
}
