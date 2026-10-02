package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/uptimy/agent/internal/kube"
)

// probe is the HTTP readinessProbe behind a Service port. Zero: none found.
type probe struct{ path, scheme string }

// intOrString is a port given as a number or a name, kept as text.
type intOrString string

func (v *intOrString) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*v = intOrString(s)
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*v = intOrString(strconv.Itoa(n))
	return nil
}

type container struct {
	Ports []struct {
		Name          string `json:"name"`
		ContainerPort int    `json:"containerPort"`
	} `json:"ports"`
	ReadinessProbe *struct {
		HTTPGet *struct {
			Path   string      `json:"path"`
			Port   intOrString `json:"port"`
			Scheme string      `json:"scheme"`
		} `json:"httpGet"`
	} `json:"readinessProbe"`
}

// portNumber resolves a port number or one of the container's port names.
func (c container) portNumber(v intOrString) (int, bool) {
	if n, err := strconv.Atoi(string(v)); err == nil {
		return n, true
	}
	for _, p := range c.Ports {
		if p.Name == string(v) {
			return p.ContainerPort, true
		}
	}
	return 0, false
}

// readinessProbe finds the HTTP readinessProbe that the pods behind a
// Service port use, so the check asks the app the question Kubernetes asks
// it (/healthz, /ready) instead of loading its home page. Only a probe on
// the port the Service sends traffic to counts. With no pods right now it
// returns what it found last time.
func (d *Discoverer) readinessProbe(ctx context.Context, o object, selector map[string]string, p servicePort) (probe, error) {
	key := o.Metadata.Namespace + "/" + o.Metadata.Name
	if len(selector) == 0 {
		return probe{}, nil // endpoints managed by hand: no pods to ask
	}
	terms := make([]string, 0, len(selector))
	for k, v := range selector {
		terms = append(terms, k+"="+v)
	}
	slices.Sort(terms)
	var l struct {
		Items []struct {
			Spec struct {
				Containers []container `json:"containers"`
			} `json:"spec"`
		} `json:"items"`
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods?limit=1&labelSelector=%s", o.Metadata.Namespace, url.QueryEscape(strings.Join(terms, ",")))
	err := d.kube.Get(ctx, path, &l)
	switch {
	case kube.IsForbidden(err):
		d.warn("not allowed to list pods, so discovered Services are checked on / instead of their readinessProbe path; give the agent's service account list access to pods")
		return d.probes[key], nil
	case err != nil:
		return probe{}, scanError{fmt.Errorf("listing pods for service %s: %w", key, err)}
	case len(l.Items) == 0:
		return d.probes[key], nil
	}

	target := p.TargetPort
	if target == "" {
		target = intOrString(strconv.Itoa(p.Port))
	}
	var found probe
	for _, c := range l.Items[0].Spec.Containers {
		if c.ReadinessProbe == nil || c.ReadinessProbe.HTTPGet == nil {
			continue
		}
		h := c.ReadinessProbe.HTTPGet
		want, ok := c.portNumber(target)
		got, ok2 := c.portNumber(h.Port)
		if ok && ok2 && want == got {
			found = probe{path: h.Path, scheme: strings.ToLower(h.Scheme)}
			if found.path == "" {
				found.path = "/"
			}
			break
		}
	}
	d.probes[key] = found
	return found, nil
}
