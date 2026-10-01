package checks

import (
	"testing"

	"github.com/uptimy/agent/internal/monitor"
)

// Every check type needs a probe, and every probe a registered check type.
func TestEveryTypeHasAProbe(t *testing.T) {
	for _, ct := range monitor.CheckTypes() {
		if _, ok := probes[ct.Type]; !ok {
			t.Errorf("%s has no probe: register one in the checks package", ct.Type)
		}
	}
	for typ := range probes {
		if _, ok := monitor.LookupCheckType(typ); !ok {
			t.Errorf("probe for %s has no monitor.RegisterCheckType", typ)
		}
	}
}
