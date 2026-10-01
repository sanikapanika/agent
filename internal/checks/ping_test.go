package checks

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out := ping(ctx, "127.0.0.1")
	if strings.HasPrefix(out.Message, "ICMP isn't allowed") {
		t.Skip("this machine doesn't allow ICMP for the test's user")
	}
	if !out.OK || out.Latency <= 0 || !strings.Contains(out.Message, "3/3 replies") {
		t.Fatalf("localhost: %+v", out)
	}

	// TEST-NET-1 is never routed: every ping times out.
	ctx, cancel = context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if out := ping(ctx, "192.0.2.1"); out.OK || !strings.Contains(out.Message, "no reply") {
		t.Fatalf("unroutable: %+v", out)
	}
}
