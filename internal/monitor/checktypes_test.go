package monitor

import (
	"net/http"
	"strings"
	"testing"
)

func TestCheckTypesAreComplete(t *testing.T) {
	seen := map[int]Type{}
	for _, k := range CheckTypes() {
		if k.Summary == "" || k.Target.Label == "" {
			t.Errorf("%s: needs a summary and a target label for the UI", k.Type)
		}
		if other, dup := seen[k.Order]; dup {
			t.Errorf("%s and %s share order %d", k.Type, other, k.Order)
		}
		seen[k.Order] = k.Type
		for _, f := range k.Fields {
			if f.Key == "" || f.Label == "" || f.Input == "" {
				t.Errorf("%s: incomplete field %+v", k.Type, f)
			}
		}
	}
}

func TestNormalizeDropsOtherTypesSettings(t *testing.T) {
	// A monitor switched from DNS to HTTP keeps no DNS settings.
	m := healthcheck("a", Check{Type: TypeHTTP, Target: "https://x.test", Config: Config{
		RecordType: "MX", Resolver: "1.1.1.1", Keyword: "ok", Headers: map[string]string{"Authorization": "x"},
	}})
	if err := m.Normalize(); err != nil {
		t.Fatal(err)
	}
	c := m.Check.Config
	if c.RecordType != "" || c.Resolver != "" {
		t.Fatalf("DNS settings kept on an HTTP check: %+v", c)
	}
	if c.Keyword != "ok" || c.Headers["Authorization"] != "x" || c.Method != http.MethodGet {
		t.Fatalf("HTTP settings lost: %+v", c)
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "twice") {
			t.Fatalf("expected a panic for a duplicate type, got %v", r)
		}
	}()
	RegisterCheckType(CheckType{Type: TypeHTTP, Label: "x", Normalize: func(*Check) error { return nil }})
}
