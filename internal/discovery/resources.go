package discovery

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/uptimy/agent/internal/monitor"
)

// prefix of the annotations that adjust a discovered monitor.
const prefix = "upti.my"

type objectList struct {
	Items []object `json:"items"`
}

type object struct {
	Metadata struct {
		Name        string            `json:"name"`
		Namespace   string            `json:"namespace"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Spec json.RawMessage `json:"spec"`
}

// options are the annotations, parsed.
type options struct {
	name, path, port, typ, scheme string
	interval                      int
	expectedStatus, keyword       string
}

func parseOptions(o object) (options, error) {
	a := func(key string) string { return strings.TrimSpace(o.Metadata.Annotations[prefix+"/"+key]) }
	opts := options{
		name: a("name"), path: a("path"), port: a("port"), typ: a("type"), scheme: a("scheme"),
		expectedStatus: a("expected-status"), keyword: a("keyword"),
	}
	if v := a("interval"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return opts, fmt.Errorf("%s/interval: %w", prefix, err)
		}
		opts.interval = int(d.Seconds())
	}
	if opts.path != "" && !strings.HasPrefix(opts.path, "/") {
		opts.path = "/" + opts.path
	}
	switch opts.scheme {
	case "", "http", "https":
	default:
		return opts, fmt.Errorf("%s/scheme must be http or https", prefix)
	}
	return opts, nil
}

func (opts options) httpCheck(target string) *monitor.Check {
	return &monitor.Check{
		Type: monitor.TypeHTTP, Target: target, IntervalSeconds: opts.interval,
		Config: monitor.Config{ExpectedStatus: opts.expectedStatus, Keyword: opts.keyword},
	}
}

func healthcheck(name string, c *monitor.Check) monitor.Monitor {
	return monitor.Monitor{Kind: monitor.KindHealthcheck, Name: name, Check: c}
}

type servicePort struct {
	Name        string      `json:"name"`
	Port        int         `json:"port"`
	TargetPort  intOrString `json:"targetPort"`
	AppProtocol string      `json:"appProtocol"`
}

// fromService checks a Service. In probe mode the agent sends its own
// request to the Service's cluster DNS name: HTTP on the pods'
// readinessProbe path when they have one, or when the port looks like HTTP,
// otherwise a TCP connect. In kubernetes mode it reuses the kubelet's
// readinessProbes: up while the Service has a ready endpoint.
func (d *Discoverer) fromService(ctx context.Context, o object) ([]monitor.Monitor, error) {
	opts, err := parseOptions(o)
	if err != nil {
		return nil, err
	}
	var spec struct {
		Selector map[string]string `json:"selector"`
		Ports    []servicePort     `json:"ports"`
	}
	if err := json.Unmarshal(o.Spec, &spec); err != nil {
		return nil, err
	}
	name := opts.name
	if name == "" {
		name = o.Metadata.Namespace + "/" + o.Metadata.Name
	}

	typ := opts.typ
	if typ == "" && opts.path == "" && opts.scheme == "" && d.serviceCheck == ServiceKubernetes {
		typ = "kubernetes"
	}
	if typ == "kubernetes" {
		target := o.Metadata.Namespace + "/service/" + o.Metadata.Name
		return []monitor.Monitor{healthcheck(name, &monitor.Check{Type: monitor.TypeKubernetes, Target: target, IntervalSeconds: opts.interval})}, nil
	}

	if len(spec.Ports) == 0 {
		return nil, errors.New("has no ports")
	}
	p := spec.Ports[0]
	if opts.port != "" {
		i := slices.IndexFunc(spec.Ports, func(p servicePort) bool {
			return p.Name == opts.port || strconv.Itoa(p.Port) == opts.port
		})
		if i < 0 {
			return nil, fmt.Errorf("%s/port %q isn't one of its ports", prefix, opts.port)
		}
		p = spec.Ports[i]
	}
	host := net.JoinHostPort(o.Metadata.Name+"."+o.Metadata.Namespace+".svc", strconv.Itoa(p.Port))

	var rp probe
	if (typ == "" || typ == "http") && (opts.path == "" || opts.scheme == "") {
		if rp, err = d.readinessProbe(ctx, o, spec.Selector, p); err != nil {
			return nil, err
		}
	}
	if typ == "" {
		typ = "tcp"
		if opts.path != "" || opts.scheme != "" || rp.path != "" || isHTTP(p) {
			typ = "http"
		}
	}
	switch typ {
	case "tcp":
		return []monitor.Monitor{healthcheck(name, &monitor.Check{Type: monitor.TypeTCP, Target: host, IntervalSeconds: opts.interval})}, nil
	case "http":
		scheme := cmp.Or(opts.scheme, rp.scheme)
		if scheme == "" {
			scheme = "http"
			if isHTTPS(p) {
				scheme = "https"
			}
		}
		path := cmp.Or(opts.path, rp.path, "/")
		return []monitor.Monitor{healthcheck(name, opts.httpCheck(scheme+"://"+host+path))}, nil
	}
	return nil, fmt.Errorf("%s/type must be http, tcp or kubernetes", prefix)
}

// isHTTP: the port's appProtocol or name says HTTP (Istio-style names like
// http-web count), or it's a usual HTTP port.
func isHTTP(p servicePort) bool {
	switch strings.ToLower(p.AppProtocol) {
	case "http", "https", "kubernetes.io/h2c", "kubernetes.io/ws", "kubernetes.io/wss":
		return true
	}
	n := strings.ToLower(p.Name)
	if n == "http" || n == "https" || n == "web" || strings.HasPrefix(n, "http-") || strings.HasPrefix(n, "https-") {
		return true
	}
	return p.Port == 80 || p.Port == 443 || p.Port == 8080
}

func isHTTPS(p servicePort) bool {
	n := strings.ToLower(p.Name)
	return strings.EqualFold(p.AppProtocol, "https") || strings.EqualFold(p.AppProtocol, "kubernetes.io/wss") ||
		n == "https" || strings.HasPrefix(n, "https-") || p.Port == 443
}

// fromIngress checks each of an Ingress's hostnames from the outside, over
// HTTPS when the Ingress has TLS for it.
func fromIngress(_ *Discoverer, _ context.Context, o object) ([]monitor.Monitor, error) {
	var spec struct {
		Rules []struct {
			Host string `json:"host"`
		} `json:"rules"`
		TLS []struct {
			Hosts []string `json:"hosts"`
		} `json:"tls"`
	}
	if err := json.Unmarshal(o.Spec, &spec); err != nil {
		return nil, err
	}
	var hosts []string
	for _, r := range spec.Rules {
		hosts = append(hosts, r.Host)
	}
	tls := map[string]bool{}
	for _, t := range spec.TLS {
		for _, h := range t.Hosts {
			tls[h] = true
		}
	}
	return fromHosts(o, hosts, func(h string) string {
		if tls[h] {
			return "https"
		}
		return "http"
	})
}

// fromHTTPRoute checks each of an HTTPRoute's hostnames. Whether the
// Gateway terminates TLS isn't on the route, so it assumes HTTPS; set
// upti.my/scheme: http otherwise.
func fromHTTPRoute(_ *Discoverer, _ context.Context, o object) ([]monitor.Monitor, error) {
	var spec struct {
		Hostnames []string `json:"hostnames"`
	}
	if err := json.Unmarshal(o.Spec, &spec); err != nil {
		return nil, err
	}
	return fromHosts(o, spec.Hostnames, func(string) string { return "https" })
}

func fromHosts(o object, hosts []string, scheme func(host string) string) ([]monitor.Monitor, error) {
	opts, err := parseOptions(o)
	if err != nil {
		return nil, err
	}
	if opts.typ != "" && opts.typ != "http" {
		return nil, fmt.Errorf("%s/type can only be http here", prefix)
	}
	path := opts.path
	if path == "" {
		path = "/"
	}
	var usable []string
	for _, h := range hosts {
		// No host is a catch-all and a wildcard isn't one address: neither
		// can be checked.
		if h != "" && !strings.Contains(h, "*") && !slices.Contains(usable, h) {
			usable = append(usable, h)
		}
	}
	if len(usable) == 0 {
		return nil, errors.New("has no hostname to check (catch-all and wildcard hosts are skipped)")
	}
	var out []monitor.Monitor
	for _, h := range usable {
		s := opts.scheme
		if s == "" {
			s = scheme(h)
		}
		name := h
		if path != "/" {
			name += path
		}
		if opts.name != "" {
			name = opts.name
			if len(usable) > 1 {
				name += " (" + h + ")"
			}
		}
		out = append(out, healthcheck(name, opts.httpCheck(s+"://"+h+path)))
	}
	return out, nil
}

// fromWorkload checks that a workload has all its replicas ready.
func fromWorkload(kind string) func(*Discoverer, context.Context, object) ([]monitor.Monitor, error) {
	return func(_ *Discoverer, _ context.Context, o object) ([]monitor.Monitor, error) {
		opts, err := parseOptions(o)
		if err != nil {
			return nil, err
		}
		if opts.typ != "" && opts.typ != "kubernetes" {
			return nil, fmt.Errorf("%s/type can only be kubernetes here", prefix)
		}
		name := opts.name
		if name == "" {
			name = fmt.Sprintf("%s/%s (%s)", o.Metadata.Namespace, o.Metadata.Name, kind)
		}
		target := o.Metadata.Namespace + "/" + kind + "/" + o.Metadata.Name
		return []monitor.Monitor{healthcheck(name, &monitor.Check{Type: monitor.TypeKubernetes, Target: target, IntervalSeconds: opts.interval})}, nil
	}
}
