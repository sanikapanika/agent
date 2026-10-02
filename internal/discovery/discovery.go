// Package discovery creates monitors for Kubernetes resources that opt in
// with the label upti.my/monitor=true, so a new app gets monitored when
// it's deployed, with no one opening the UI:
//
//   - a Service becomes an HTTP check (or TCP, for ports that aren't HTTP)
//     on its cluster DNS name
//   - an Ingress or Gateway API HTTPRoute becomes an HTTP check per hostname
//   - a Deployment, StatefulSet or DaemonSet becomes a readiness check
//
// Annotations (upti.my/name, /path, /port, ...) adjust what's checked; see
// the README. Discovered monitors are read-only in the UI and are deleted
// when the label or the resource goes away.
package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/uptimy/agent/internal/kube"
	"github.com/uptimy/agent/internal/managed"
	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/store"
)

// Label opts a resource in. A label rather than an annotation, so the API
// server does the filtering and large clusters stay cheap to scan.
const Label = "upti.my/monitor"

// Interval is how often the cluster is scanned.
const Interval = 30 * time.Second

// resource is a kind of object discovery lists.
type resource struct {
	kind  string // as in monitor names and logs
	group string // API path prefix
	name  string // plural, as in the API path
	build func(o object) ([]monitor.Monitor, error)
}

var resources = []resource{
	{"service", "/api/v1", "services", fromService},
	{"ingress", "/apis/networking.k8s.io/v1", "ingresses", fromIngress},
	{"httproute", "/apis/gateway.networking.k8s.io/v1", "httproutes", fromHTTPRoute},
	{"deployment", "/apis/apps/v1", "deployments", fromWorkload("deployment")},
	{"statefulset", "/apis/apps/v1", "statefulsets", fromWorkload("statefulset")},
	{"daemonset", "/apis/apps/v1", "daemonsets", fromWorkload("daemonset")},
}

// Discoverer scans the cluster for labeled resources.
type Discoverer struct {
	kube *kube.Client
	log  *slog.Logger

	// namespaced: listing this resource cluster-wide was forbidden (the
	// chart's rbac.clusterWide=false), so only the agent's namespace is
	// scanned.
	namespaced map[string]bool
	// warned holds messages already logged, so a bad annotation is reported
	// once, not every scan.
	warned map[string]bool
}

// New returns a Discoverer.
func New(k *kube.Client, log *slog.Logger) *Discoverer {
	return &Discoverer{kube: k, log: log, namespaced: map[string]bool{}, warned: map[string]bool{}}
}

// Discover returns the monitors the cluster asks for. Resource types the
// cluster doesn't have (no Gateway API) or RBAC doesn't allow are skipped.
// Any other error fails the whole scan, so a flaky API never deletes
// monitors.
func (d *Discoverer) Discover(ctx context.Context) ([]managed.Desired, error) {
	var out []managed.Desired
	seen := map[string]string{} // monitor name → resource it came from
	for _, r := range resources {
		items, err := d.list(ctx, r)
		if err != nil {
			return nil, err
		}
		for _, o := range items {
			from := fmt.Sprintf("%s %s/%s", r.kind, o.Metadata.Namespace, o.Metadata.Name)
			ms, err := r.build(o)
			if err != nil {
				d.warn(from + ": " + err.Error())
				continue
			}
			for _, m := range ms {
				m.Source = monitor.SourceKubernetes
				if err := m.Normalize(); err != nil {
					d.warn(fmt.Sprintf("%s: %s", from, err))
					continue
				}
				if other, dup := seen[m.Name]; dup {
					d.warn(fmt.Sprintf("%s: %s also wants the name %q; set %s/name on one of them", from, other, m.Name, prefix))
					continue
				}
				seen[m.Name] = from
				out = append(out, managed.Desired{Monitor: m})
			}
		}
	}
	return out, nil
}

func (d *Discoverer) list(ctx context.Context, r resource) ([]object, error) {
	selector := "?labelSelector=" + url.QueryEscape(Label+"=true")
	var l objectList
	if !d.namespaced[r.name] {
		err := d.kube.Get(ctx, r.group+"/"+r.name+selector, &l)
		switch {
		case err == nil:
			return l.Items, nil
		case kube.IsNotFound(err):
			return nil, nil // e.g. Gateway API isn't installed
		case !kube.IsForbidden(err):
			return nil, fmt.Errorf("listing %s: %w", r.name, err)
		}
		d.namespaced[r.name] = true
	}
	if d.kube.Namespace() == "" {
		return nil, nil
	}
	err := d.kube.Get(ctx, r.group+"/namespaces/"+d.kube.Namespace()+"/"+r.name+selector, &l)
	switch {
	case err == nil:
		return l.Items, nil
	case kube.IsNotFound(err):
		return nil, nil
	case kube.IsForbidden(err):
		d.warn(fmt.Sprintf("not allowed to list %s; give the agent's service account list access to discover them", r.name))
		return nil, nil
	}
	return nil, fmt.Errorf("listing %s: %w", r.name, err)
}

func (d *Discoverer) warn(msg string) {
	if !d.warned[msg] {
		d.warned[msg] = true
		d.log.Warn("kubernetes discovery: " + msg)
	}
}

// Run scans every Interval until ctx ends, syncs the discovered monitors
// into the store and passes what changed to apply (to restart checks and
// refresh the UI).
func (d *Discoverer) Run(ctx context.Context, st *store.Store, apply func(managed.Changes)) {
	failing := false
	for {
		desired, err := d.Discover(ctx)
		if err == nil {
			var ch managed.Changes
			ch, err = managed.Sync(ctx, st, monitor.SourceKubernetes, desired, managed.Options{KeepPaused: true})
			if !ch.Empty() {
				d.log.Info("kubernetes discovery applied", "created", ch.Created, "updated", ch.Updated, "deleted", len(ch.Deleted))
				apply(ch)
			}
		}
		switch {
		case err != nil && ctx.Err() == nil && !failing:
			d.log.Warn("kubernetes discovery failed; keeping the monitors it found before", "err", err)
			failing = true
		case err == nil && failing:
			d.log.Info("kubernetes discovery recovered")
			failing = false
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(Interval):
		}
	}
}
