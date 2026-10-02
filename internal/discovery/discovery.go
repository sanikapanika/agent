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
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/uptimy/agent/internal/kube"
	"github.com/uptimy/agent/internal/managed"
	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/store"
)

// Label opts a resource in: "true" (or "yes", "1", "on"). "false" (or "no",
// "0", "off") opts it out explicitly; any other value is reported. A label
// rather than an annotation, so the API server does the filtering and large
// clusters stay cheap to scan.
const Label = "upti.my/monitor"

// optIn says what a value of Label means: in, out, or a mistake.
func optIn(value string) (in, valid bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1", "on":
		return true, true
	case "false", "no", "0", "off":
		return false, true
	}
	return false, false
}

// Interval is how often the cluster is scanned.
const Interval = 30 * time.Second

// resource is a kind of object discovery lists.
type resource struct {
	kind  string // as in monitor names and logs
	group string // API path prefix
	name  string // plural, as in the API path
	build func(d *Discoverer, ctx context.Context, o object) ([]monitor.Monitor, error)
}

var resources = []resource{
	{"service", "/api/v1", "services", (*Discoverer).fromService},
	{"ingress", "/apis/networking.k8s.io/v1", "ingresses", fromIngress},
	{"httproute", "/apis/gateway.networking.k8s.io/v1", "httproutes", fromHTTPRoute},
	{"deployment", "/apis/apps/v1", "deployments", fromWorkload("deployment")},
	{"statefulset", "/apis/apps/v1", "statefulsets", fromWorkload("statefulset")},
	{"daemonset", "/apis/apps/v1", "daemonsets", fromWorkload("daemonset")},
	{"cronjob", "/apis/batch/v1", "cronjobs", fromCronJob},
}

// Discoverer scans the cluster for labeled resources.
type Discoverer struct {
	kube *kube.Client
	log  *slog.Logger

	// mu guards what the API reads while scans run: status and namespaced.
	mu     sync.Mutex
	status Status
	// warnings found by the scan in progress, and by the last Job read.
	scanWarnings, jobWarnings []string

	// probes remembers each Service's readinessProbe (namespace/name), so
	// its check keeps the same path while it has no pods (scaled to zero,
	// mid-rollout).
	probes map[string]probe
	// built holds the monitors each object last produced ("kind ns/name").
	// When an object's annotations stop being valid, its monitors stay as
	// they were instead of being deleted with their history.
	built map[string][]monitor.Monitor
	// cronJobs are the discovered CronJobs, whose Jobs report runs.
	cronJobs []cronJobRef
	jobs     jobTracker

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
	return &Discoverer{
		kube: k, log: log,
		probes: map[string]probe{}, built: map[string][]monitor.Monitor{},
		jobs:       jobTracker{seen: map[string]*seenJob{}, since: map[int64]time.Time{}},
		status:     Status{Scope: "cluster"},
		namespaced: map[string]bool{}, warned: map[string]bool{},
	}
}

// scanError is an API failure while building monitors. Unlike a bad
// annotation, it fails the whole scan.
type scanError struct{ error }

// Discover returns the monitors the cluster asks for. Resource types the
// cluster doesn't have (no Gateway API) or RBAC doesn't allow are skipped.
// Any other error fails the whole scan, so a flaky API never deletes
// monitors.
func (d *Discoverer) Discover(ctx context.Context) ([]managed.Desired, error) {
	var out []managed.Desired
	built := map[string][]monitor.Monitor{} // replaces d.built once the scan completes
	var cronJobs []cronJobRef
	d.scanWarnings = nil
	for _, r := range resources {
		items, err := d.list(ctx, r)
		if err != nil {
			d.finishScan(0, err)
			return nil, err
		}
		for _, o := range items {
			from := fmt.Sprintf("%s %s/%s", r.kind, o.Metadata.Namespace, o.Metadata.Name)
			in, valid := optIn(o.Metadata.Labels[Label])
			if !valid {
				d.warn(fmt.Sprintf("%s: %s is %q; set it to \"true\" to monitor it", from, Label, o.Metadata.Labels[Label]))
			}
			if !in {
				continue
			}
			ms, err := r.build(d, ctx, o)
			var se scanError
			if errors.As(err, &se) {
				d.finishScan(0, se.error)
				return nil, se.error
			}
			if err == nil {
				for i := range ms {
					ms[i].Source = monitor.SourceKubernetes
					// Which object it came from, plus the hostname for
					// Ingresses and HTTPRoutes ("#shop.example.com").
					ms[i].SourceRef = fmt.Sprintf("%s/%s/%s%s", r.kind, o.Metadata.Namespace, o.Metadata.Name, ms[i].SourceRef)
					if err = ms[i].Normalize(); err != nil {
						break
					}
				}
			}
			if err != nil {
				prev, ok := d.built[from]
				if !ok {
					d.warn(from + ": " + err.Error())
					continue
				}
				d.warn(fmt.Sprintf("%s: %s; keeping its monitor as it was", from, err))
				ms = prev
			}
			prev := d.built[from]
			built[from] = ms
			for i, m := range ms {
				ds := managed.Desired{Monitor: m, GeneratedToken: m.Kind == monitor.KindHeartbeat}
				// A suspended CronJob pauses its heartbeat, and resuming it
				// resumes the heartbeat. Otherwise a pause set in the UI stays.
				switch {
				case m.Paused:
					ds.SetPaused = &m.Paused
				case i < len(prev) && prev[i].Paused:
					ds.SetPaused = &m.Paused
				}
				out = append(out, ds)
				if r.kind == "cronjob" {
					cronJobs = append(cronJobs, cronJobRef{namespace: o.Metadata.Namespace, name: o.Metadata.Name, uid: o.Metadata.UID, ref: m.SourceRef})
				}
			}
		}
	}
	d.built, d.cronJobs = built, cronJobs
	d.finishScan(len(out), nil)
	return out, nil
}

// errForbidden: RBAC doesn't allow listing this resource, even in the
// agent's namespace.
var errForbidden = errors.New("forbidden")

// get lists r with query, cluster-wide when clusterWide is set, falling back
// to the agent's namespace when that's forbidden (the chart's
// rbac.clusterWide=false). It reports whether it fell back. Resource types
// the cluster doesn't have (no Gateway API) list as empty.
func (d *Discoverer) get(ctx context.Context, r resource, query string, clusterWide bool) (items []object, fellBack bool, err error) {
	var l objectList
	if clusterWide {
		err := d.kube.Get(ctx, r.group+"/"+r.name+query, &l)
		switch {
		case err == nil:
			return l.Items, false, nil
		case kube.IsNotFound(err):
			return nil, false, nil
		case !kube.IsForbidden(err):
			return nil, false, fmt.Errorf("listing %s: %w", r.name, err)
		}
		fellBack = true
	}
	if d.kube.Namespace() == "" {
		return nil, fellBack, nil
	}
	err = d.kube.Get(ctx, r.group+"/namespaces/"+d.kube.Namespace()+"/"+r.name+query, &l)
	switch {
	case err == nil:
		return l.Items, fellBack, nil
	case kube.IsNotFound(err):
		return nil, fellBack, nil
	case kube.IsForbidden(err):
		return nil, fellBack, errForbidden
	}
	return nil, fellBack, fmt.Errorf("listing %s: %w", r.name, err)
}

// list returns r's objects that have the label, whatever its value.
func (d *Discoverer) list(ctx context.Context, r resource) ([]object, error) {
	d.mu.Lock()
	clusterWide := !d.namespaced[r.name]
	d.mu.Unlock()
	items, fellBack, err := d.get(ctx, r, "?labelSelector="+url.QueryEscape(Label), clusterWide)
	if fellBack {
		d.mu.Lock()
		d.namespaced[r.name] = true
		d.status.Scope = "namespace " + d.kube.Namespace()
		d.mu.Unlock()
		if !d.warned["namespaced"] {
			d.warned["namespaced"] = true
			d.log.Info("kubernetes discovery: not allowed to list cluster-wide, so only namespace " + d.kube.Namespace() + " is scanned")
		}
	}
	if errors.Is(err, errForbidden) {
		d.warn(fmt.Sprintf("not allowed to list %s; give the agent's service account list access to discover them", r.name))
		return nil, nil
	}
	return items, err
}

// warn reports a problem: in the log once, and in the status for as long
// as it lasts.
func (d *Discoverer) warn(msg string) {
	if !slices.Contains(d.scanWarnings, msg) {
		d.scanWarnings = append(d.scanWarnings, msg)
	}
	if !d.warned[msg] {
		d.warned[msg] = true
		d.log.Warn("kubernetes discovery: " + msg)
	}
}

// warnJobs is warn for reading Jobs, which runs between scans.
func (d *Discoverer) warnJobs(msg string) {
	if !slices.Contains(d.jobWarnings, msg) {
		d.jobWarnings = append(d.jobWarnings, msg)
	}
	if !d.warned[msg] {
		d.warned[msg] = true
		d.log.Warn("kubernetes discovery: " + msg)
	}
}

// Run scans the cluster every Interval until ctx ends, syncs the
// discovered monitors into the store and passes what changed to apply (to
// restart checks and refresh the UI). Every JobInterval it reads the
// discovered CronJobs' Jobs and reports their runs.
func (d *Discoverer) Run(ctx context.Context, st *store.Store, apply func(managed.Changes), report Report) {
	failing := false
	scanEvery := int(Interval / JobInterval)
	for tick := 0; ; tick++ {
		if tick%scanEvery == 0 {
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
		}
		if err := d.trackJobs(ctx, st, report); err != nil && ctx.Err() == nil {
			d.log.Warn("kubernetes discovery: reading CronJob runs", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(JobInterval):
		}
	}
}
