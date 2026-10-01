// Package schema describes the settings of a pluggable type (a monitor type,
// a notification channel) so the UI can render its form without knowing the
// type in advance. Adding a type is then a Go change only.
package schema

// Input kinds a Field can use.
const (
	Text     = "text"
	Number   = "number"
	Select   = "select"
	Switch   = "switch"
	Textarea = "textarea"
	Password = "password" // a secret: masked in the form, hidden from viewers
)

// Field is one setting in a type's form. Key is its name in the config
// (JSON and YAML).
type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Input       string   `json:"input"`
	Placeholder string   `json:"placeholder,omitempty"`
	Hint        string   `json:"hint,omitempty"`
	Options     []string `json:"options,omitempty"` // for Select
	Default     any      `json:"default,omitempty"`
	Required    bool     `json:"required,omitempty"`
	// Wide fields take a full row; others share a row with their neighbors.
	Wide bool `json:"wide,omitempty"`
}

// Keys returns the config keys of fields.
func Keys(fields []Field) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Key
	}
	return out
}
