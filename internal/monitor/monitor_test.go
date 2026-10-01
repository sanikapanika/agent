package monitor

import (
	"net/http"
	"testing"
)

// healthcheck returns a healthcheck monitor named name with the given check.
func healthcheck(name string, c Check) Monitor {
	return Monitor{Kind: KindHealthcheck, Name: name, Check: &c}
}

func TestNormalizeDefaults(t *testing.T) {
	m := healthcheck(" API ", Check{Type: TypeHTTP, Target: "https://example.com/health"})
	if err := m.Normalize(); err != nil {
		t.Fatal(err)
	}
	c := m.Check
	if m.Name != "API" || c.IntervalSeconds != 60 || c.TimeoutSeconds != 10 || c.FailureThreshold != 2 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.Config.Method != http.MethodGet || c.Config.ExpectedStatus != "200-399" {
		t.Fatalf("unexpected http defaults: %+v", c.Config)
	}
}

func TestNormalizeRejects(t *testing.T) {
	cases := map[string]Monitor{
		"no name":         healthcheck("", Check{Type: TypeHTTP, Target: "https://x"}),
		"bad url":         healthcheck("a", Check{Type: TypeHTTP, Target: "ftp://x"}),
		"tcp no port":     healthcheck("a", Check{Type: TypeTCP, Target: "db"}),
		"short interval":  healthcheck("a", Check{Type: TypeTCP, Target: "db:5432", IntervalSeconds: 5}),
		"timeout > int":   healthcheck("a", Check{Type: TypeTCP, Target: "db:5432", IntervalSeconds: 10, TimeoutSeconds: 20}),
		"bad kube kind":   healthcheck("a", Check{Type: TypeKubernetes, Target: "default/pod/api"}),
		"unknown type":    healthcheck("a", Check{Type: "icmp", Target: "x"}),
		"bad status code": healthcheck("a", Check{Type: TypeHTTP, Target: "https://x", Config: Config{ExpectedStatus: "abc"}}),
		"no kind":         {Name: "a", Check: &Check{Type: TypeTCP, Target: "db:5432"}},
		"both settings":   {Name: "a", Kind: KindHealthcheck, Check: &Check{Type: TypeTCP, Target: "db:1"}, Heartbeat: &Heartbeat{}},
	}
	for name, m := range cases {
		if err := m.Normalize(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseStatusRanges(t *testing.T) {
	r, err := ParseStatusRanges("200-299, 301,404")
	if err != nil {
		t.Fatal(err)
	}
	want := []StatusRange{{200, 299}, {301, 301}, {404, 404}}
	if len(r) != len(want) {
		t.Fatalf("got %v", r)
	}
	for i := range want {
		if r[i] != want[i] {
			t.Fatalf("got %v, want %v", r, want)
		}
	}
	for _, bad := range []string{"", "600", "300-200", "2xx"} {
		if _, err := ParseStatusRanges(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
