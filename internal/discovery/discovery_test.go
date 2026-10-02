package discovery

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/uptimy/agent/internal/kube"
	"github.com/uptimy/agent/internal/managed"
	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/store"
)

const services = `{"items":[
 {"metadata":{"name":"checkout","namespace":"shop"},
  "spec":{"ports":[{"name":"http","port":8080}]}},
 {"metadata":{"name":"postgres","namespace":"shop","annotations":{"upti.my/name":"Shop database"}},
  "spec":{"ports":[{"name":"pg","port":5432}]}},
 {"metadata":{"name":"api","namespace":"shop","annotations":{
   "upti.my/port":"admin","upti.my/path":"healthz","upti.my/interval":"30s","upti.my/keyword":"ok"}},
  "spec":{"ports":[{"name":"grpc","port":9090},{"name":"admin","port":9091}]}},
 {"metadata":{"name":"broken","namespace":"shop","annotations":{"upti.my/interval":"often"}},
  "spec":{"ports":[{"port":80}]}},
 {"metadata":{"name":"web","namespace":"shop"},
  "spec":{"selector":{"app":"web"},"ports":[{"name":"main","port":80,"targetPort":"http"}]}},
 {"metadata":{"name":"grpcish","namespace":"shop"},
  "spec":{"selector":{"app":"grpcish"},"ports":[{"name":"api","port":9000,"targetPort":9000}]}},
 {"metadata":{"name":"admin","namespace":"shop"},
  "spec":{"selector":{"app":"admin"},"ports":[{"name":"http","port":80,"targetPort":8080}]}},
 {"metadata":{"name":"queue","namespace":"shop","annotations":{"upti.my/type":"kubernetes"}},
  "spec":{"selector":{"app":"queue"},"ports":[{"port":5672}]}}
]}`

// Pods behind the Services, by label selector.
const (
	webPods = `{"items":[{"spec":{"containers":[{"ports":[{"name":"http","containerPort":3000}],
  "readinessProbe":{"httpGet":{"path":"/healthz","port":"http"}}}]}}]}`
	grpcishPods = `{"items":[{"spec":{"containers":[{"readinessProbe":{"httpGet":{"path":"/ready","port":9000,"scheme":"HTTPS"}}}]}}]}`
	// The probe is on another port than the one the Service sends traffic to.
	adminPods = `{"items":[{"spec":{"containers":[{"ports":[{"containerPort":8080},{"containerPort":8081}],
  "readinessProbe":{"httpGet":{"path":"/ready","port":8081}}}]}}]}`
)

const ingresses = `{"items":[
 {"metadata":{"name":"shop","namespace":"shop"},
  "spec":{"rules":[{"host":"shop.example.com"},{"host":"admin.example.com"},{"host":"*.example.com"},{}],
          "tls":[{"hosts":["shop.example.com"]}]}}
]}`

const deployments = `{"items":[
 {"metadata":{"name":"checkout","namespace":"shop"}},
 {"metadata":{"name":"worker","namespace":"shop","annotations":{"upti.my/name":"shop/checkout"}}}
]}`

// fakeAPI serves list responses by path. Paths not in the map are 404.
type fakeAPI struct {
	mu        sync.Mutex
	responses map[string]string
	status    map[string]int
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := r.URL.Path
	if strings.HasSuffix(key, "/pods") {
		key += "?" + r.URL.Query().Get("labelSelector")
	} else if !strings.HasSuffix(key, "/jobs") && r.URL.Query().Get("labelSelector") != "upti.my/monitor=true" {
		http.Error(w, "missing label selector", http.StatusBadRequest)
		return
	}
	if code := f.status[key]; code != 0 {
		http.Error(w, "nope", code)
		return
	}
	body, ok := f.responses[key]
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, _ = io.WriteString(w, body)
}

func (f *fakeAPI) set(path, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[path] = body
}

func newFake(t *testing.T) (*fakeAPI, *Discoverer) {
	f := &fakeAPI{responses: map[string]string{
		"/api/v1/services":                         services,
		"/apis/networking.k8s.io/v1/ingresses":     ingresses,
		"/apis/apps/v1/deployments":                deployments,
		"/apis/apps/v1/statefulsets":               `{"items":[]}`,
		"/apis/apps/v1/daemonsets":                 `{"items":[]}`,
		"/api/v1/namespaces/shop/pods?app=web":     webPods,
		"/api/v1/namespaces/shop/pods?app=grpcish": grpcishPods,
		"/api/v1/namespaces/shop/pods?app=admin":   adminPods,
		// No Gateway API: httproutes are a 404.
	}, status: map[string]int{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, New(kube.New(srv.URL, "monitoring"), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func byName(ds []managed.Desired) map[string]monitor.Check {
	out := map[string]monitor.Check{}
	for _, d := range ds {
		out[d.Name] = *d.Check
	}
	return out
}

func TestDiscover(t *testing.T) {
	_, d := newFake(t)
	ds, err := d.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := byName(ds)
	want := map[string]struct {
		typ      monitor.Type
		target   string
		interval int
	}{
		"shop/checkout":              {monitor.TypeHTTP, "http://checkout.shop.svc:8080/", 60},
		"Shop database":              {monitor.TypeTCP, "postgres.shop.svc:5432", 60},
		"shop/api":                   {monitor.TypeHTTP, "http://api.shop.svc:9091/healthz", 30},
		"shop.example.com":           {monitor.TypeHTTP, "https://shop.example.com/", 60},
		"admin.example.com":          {monitor.TypeHTTP, "http://admin.example.com/", 60},
		"shop/checkout (deployment)": {monitor.TypeKubernetes, "shop/deployment/checkout", 60},
		// The pods' readinessProbe path and scheme, on the Service's port.
		"shop/web":     {monitor.TypeHTTP, "http://web.shop.svc:80/healthz", 60},
		"shop/grpcish": {monitor.TypeHTTP, "https://grpcish.shop.svc:9000/ready", 60},
		"shop/admin":   {monitor.TypeHTTP, "http://admin.shop.svc:80/", 60},
		// upti.my/type: kubernetes reuses the readinessProbes instead.
		"shop/queue": {monitor.TypeKubernetes, "shop/service/queue", 60},
	}
	for name, w := range want {
		c, ok := got[name]
		if !ok {
			t.Errorf("missing %q", name)
			continue
		}
		if c.Type != w.typ || c.Target != w.target || c.IntervalSeconds != w.interval {
			t.Errorf("%s: got %s %s every %ds", name, c.Type, c.Target, c.IntervalSeconds)
		}
	}
	if got["shop/api"].Config.Keyword != "ok" {
		t.Error("keyword annotation ignored")
	}
	// The broken interval is skipped, the wildcard and catch-all hosts too,
	// and the worker's name clashes with the checkout Service.
	if len(got) != len(want) {
		t.Errorf("got %d monitors, want %d: %v", len(got), len(want), got)
	}
	for _, d := range ds {
		if d.Source != monitor.SourceKubernetes || d.Public {
			t.Errorf("%s: source %q public %v", d.Name, d.Source, d.Public)
		}
	}
}

// With rbac.clusterWide=false, cluster-wide lists are forbidden and the
// agent's own namespace is scanned instead.
func TestDiscoverNamespaced(t *testing.T) {
	f, d := newFake(t)
	for _, p := range []string{"/api/v1/services", "/apis/networking.k8s.io/v1/ingresses", "/apis/apps/v1/deployments", "/apis/apps/v1/statefulsets", "/apis/apps/v1/daemonsets"} {
		f.status[p] = http.StatusForbidden
	}
	f.set("/api/v1/namespaces/monitoring/services", `{"items":[{"metadata":{"name":"agent","namespace":"monitoring"},"spec":{"ports":[{"name":"http","port":80}]}}]}`)
	ds, err := d.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := byName(ds); len(got) != 1 || got["monitoring/agent"].Target != "http://agent.monitoring.svc:80/" {
		t.Fatalf("got %v", got)
	}
}

// A failing API fails the scan, so Run keeps what it found before.
func TestDiscoverError(t *testing.T) {
	f, d := newFake(t)
	f.status["/apis/apps/v1/deployments"] = http.StatusInternalServerError
	if _, err := d.Discover(context.Background()); err == nil || !strings.Contains(err.Error(), "deployments") {
		t.Fatalf("got %v", err)
	}
}

func TestSyncKeepsPause(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	f, d := newFake(t)
	rescan := func() managed.Changes {
		t.Helper()
		ds, err := d.Discover(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ch, err := managed.Sync(ctx, st, monitor.SourceKubernetes, ds, managed.Options{KeepPaused: true})
		if err != nil {
			t.Fatal(err)
		}
		return ch
	}
	if ch := rescan(); ch.Created != 10 {
		t.Fatalf("created %d", ch.Created)
	}
	all, _ := st.ListMonitors(ctx)
	paused := all[0]
	paused.Paused = true
	if _, err := st.UpdateMonitor(ctx, paused); err != nil {
		t.Fatal(err)
	}
	if ch := rescan(); !ch.Empty() {
		t.Fatalf("a rescan changed %+v", ch)
	}
	if m, _ := st.GetMonitor(ctx, paused.ID); !m.Paused {
		t.Fatal("pausing in the UI didn't stick")
	}

	// Unlabeling the Ingress deletes its two monitors.
	f.set("/apis/networking.k8s.io/v1/ingresses", `{"items":[]}`)
	if ch := rescan(); len(ch.Deleted) != 2 || len(ch.Saved) != 0 {
		t.Fatalf("got %+v", ch)
	}
}

// Without pods (scaled to zero) a Service keeps the path found before; if
// pods can't be listed, it falls back to /.
func TestReadinessProbeFallbacks(t *testing.T) {
	f, d := newFake(t)
	target := func() string {
		t.Helper()
		ds, err := d.Discover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return byName(ds)["shop/web"].Target
	}
	if got := target(); got != "http://web.shop.svc:80/healthz" {
		t.Fatalf("got %s", got)
	}
	f.set("/api/v1/namespaces/shop/pods?app=web", `{"items":[]}`)
	if got := target(); got != "http://web.shop.svc:80/healthz" {
		t.Fatalf("scaled to zero: got %s", got)
	}

	f2, d2 := newFake(t)
	f2.status["/api/v1/namespaces/shop/pods?app=web"] = http.StatusForbidden
	d = d2
	if got := target(); got != "http://web.shop.svc:80/" {
		t.Fatalf("pods forbidden: got %s", got)
	}

	// Any other failure fails the scan rather than changing the target.
	f2.status["/api/v1/namespaces/shop/pods?app=web"] = http.StatusInternalServerError
	if _, err := d2.Discover(context.Background()); err == nil {
		t.Fatal("a pod listing failure didn't fail the scan")
	}
}

// A typo in an annotation keeps the monitor it had, rather than deleting it
// and its history.
func TestInvalidAnnotationKeepsMonitor(t *testing.T) {
	f, d := newFake(t)
	if _, err := d.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.set("/apis/apps/v1/deployments", `{"items":[{"metadata":{"name":"checkout","namespace":"shop","annotations":{"upti.my/interval":"soon"}}}]}`)
	ds, err := d.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := byName(ds)["shop/checkout (deployment)"]; !ok || c.IntervalSeconds != 60 {
		t.Fatalf("the monitor wasn't kept as it was: %v %+v", ok, c)
	}
	// Removing the label still deletes it.
	f.set("/apis/apps/v1/deployments", `{"items":[]}`)
	ds, _ = d.Discover(context.Background())
	if _, ok := byName(ds)["shop/checkout (deployment)"]; ok {
		t.Fatal("kept after the label was removed")
	}
}
