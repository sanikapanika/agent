package monitor

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/uptimy/agent/internal/schema"
)

// A CheckType describes one kind of healthcheck (HTTP, DNS, ...): how it's
// labeled and configured in the UI, and how its settings are validated. Each
// built-in type registers itself from its own file (check_http.go, ...); its
// probe lives in the checks package. See CONTRIBUTING.md, "Adding a check type".
type CheckType struct {
	Type    Type   `json:"type"`
	Label   string `json:"label"`   // "HTTP", "PostgreSQL"
	Summary string `json:"summary"` // one line for the type picker
	// Order places the type in the picker.
	Order  int        `json:"order"`
	Target TargetSpec `json:"target"`
	// Fields are the type's settings in the form, in order.
	Fields []schema.Field `json:"fields"`
	// Requires names an environment the probe needs ("kubernetes"), so the
	// UI can warn when the agent doesn't run there.
	Requires string `json:"requires,omitempty"`

	// ExtraKeys are config keys the type uses that aren't in the form (set in
	// YAML, or by the agent itself).
	ExtraKeys []string `json:"-"`
	// Normalize fills the type's defaults and validates its target and
	// config. Generic fields (interval, timeout, ...) are already checked.
	Normalize func(c *Check) error `json:"-"`
}

// TargetSpec describes the target field for a type.
type TargetSpec struct {
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Hint        string `json:"hint,omitempty"`
	Optional    bool   `json:"optional,omitempty"`
}

var checkTypes = map[Type]CheckType{}

// RegisterCheckType adds a check type. It panics on a duplicate, so a
// mistake shows up at startup (and in tests) rather than as a silently
// missing type.
func RegisterCheckType(ct CheckType) {
	if ct.Type == "" || ct.Label == "" || ct.Normalize == nil {
		panic("monitor: RegisterCheckType needs a type, a label and Normalize")
	}
	if _, dup := checkTypes[ct.Type]; dup {
		panic(fmt.Sprintf("monitor: check type %q registered twice", ct.Type))
	}
	checkTypes[ct.Type] = ct
}

// LookupCheckType returns the registered check type t.
func LookupCheckType(t Type) (CheckType, bool) {
	ct, ok := checkTypes[t]
	return ct, ok
}

// CheckTypes returns every registered check type, in picker order.
func CheckTypes() []CheckType {
	out := make([]CheckType, 0, len(checkTypes))
	for _, ct := range checkTypes {
		out = append(out, ct)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Type < out[j].Type
	})
	return out
}

// keys returns every config key the type uses.
func (ct CheckType) keys() []string {
	return append(schema.Keys(ct.Fields), ct.ExtraKeys...)
}

// onlyKeys drops config settings that belong to other types, e.g. a DNS
// resolver left over after switching a healthcheck from DNS to HTTP, so its
// config only ever holds settings that mean something for it.
func (c Config) onlyKeys(keys []string) (Config, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return c, err
	}
	for k := range all {
		if !slices.Contains(keys, k) {
			delete(all, k)
		}
	}
	raw, _ = json.Marshal(all)
	var out Config
	return out, json.Unmarshal(raw, &out)
}
