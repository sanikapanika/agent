package managed

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/uptimy/agent/internal/monitor"
	"github.com/uptimy/agent/internal/store"
)

func check(name, ref, target string) Desired {
	m := monitor.Monitor{Kind: monitor.KindHealthcheck, Name: name, SourceRef: ref,
		Check: &monitor.Check{Type: monitor.TypeTCP, Target: target}}
	if err := m.Normalize(); err != nil {
		panic(err)
	}
	return Desired{Monitor: m}
}

func TestSyncByRef(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sync := func(ds ...Desired) Changes {
		t.Helper()
		ch, err := Sync(ctx, st, monitor.SourceKubernetes, ds, Options{KeepPaused: true})
		if err != nil {
			t.Fatal(err)
		}
		return ch
	}

	// A monitor saved before refs existed is matched by name once, and
	// takes the ref.
	legacy := check("shop/checkout", "", "checkout.shop.svc:80")
	legacy.Source = monitor.SourceKubernetes
	old, err := st.CreateMonitor(ctx, legacy.Monitor)
	if err != nil {
		t.Fatal(err)
	}
	if ch := sync(check("shop/checkout", "service/shop/checkout", "checkout.shop.svc:80")); ch.Created != 0 || ch.Updated != 1 {
		t.Fatalf("adopt: %+v", ch)
	}
	if m, _ := st.GetMonitor(ctx, old.ID); m.SourceRef != "service/shop/checkout" {
		t.Fatalf("ref not saved: %+v", m)
	}

	// Renamed: the same monitor.
	if ch := sync(check("Checkout API", "service/shop/checkout", "checkout.shop.svc:80")); ch.Created != 0 || ch.Updated != 1 || len(ch.Deleted) != 0 {
		t.Fatalf("rename: %+v", ch)
	}
	if m, _ := st.GetMonitor(ctx, old.ID); m.Name != "Checkout API" {
		t.Fatalf("not renamed: %+v", m)
	}

	// Two objects may share a name now; each keeps its own monitor.
	ch := sync(
		check("Checkout API", "service/shop/checkout", "checkout.shop.svc:80"),
		check("Checkout API", "service/staging/checkout", "checkout.staging.svc:80"),
	)
	if ch.Created != 1 || ch.Updated != 0 || len(ch.Deleted) != 0 {
		t.Fatalf("same name, two objects: %+v", ch)
	}

	// Gone from the source: deleted.
	if ch := sync(check("Checkout API", "service/staging/checkout", "checkout.staging.svc:80")); len(ch.Deleted) != 1 || ch.Deleted[0] != old.ID {
		t.Fatalf("delete: %+v", ch)
	}
}
