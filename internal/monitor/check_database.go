package monitor

import "github.com/uptimy/agent/internal/schema"

// The database types share their target rules (a connection URL, ${VAR}
// references, masked passwords); see database.go.

const databaseTargetHint = "Use ${VAR} to read it, or just the password, from the agent's environment. Prefer a read-only user."

var sqlFields = []schema.Field{
	{Key: "query", Label: "Query", Input: schema.Text, Placeholder: "SELECT 1", Hint: "Optional, runs read-only. Default SELECT 1"},
	{Key: "expected", Label: "Expected result", Input: schema.Text, Hint: "Optional, exact match of the first column of the first row"},
}

func init() {
	RegisterCheckType(CheckType{
		Type:      TypePostgres,
		Label:     "PostgreSQL",
		Summary:   "Logs in and runs a query",
		Order:     60,
		Target:    TargetSpec{Label: "Connection URL", Placeholder: "postgres://monitor:password@db:5432/app?sslmode=require", Hint: databaseTargetHint}, //nolint:gosec // G101: a placeholder
		Fields:    sqlFields,
		Normalize: (*Check).normalizeDatabase,
	})
	RegisterCheckType(CheckType{
		Type:      TypeMySQL,
		Label:     "MySQL",
		Summary:   "Logs in and runs a query",
		Order:     70,
		Target:    TargetSpec{Label: "Connection URL", Placeholder: "mysql://monitor:password@db:3306/app", Hint: databaseTargetHint}, //nolint:gosec // G101: a placeholder
		Fields:    sqlFields,
		Normalize: (*Check).normalizeDatabase,
	})
	RegisterCheckType(CheckType{
		Type:    TypeRedis,
		Label:   "Redis",
		Summary: "Authenticates and PINGs",
		Order:   80,
		Target:  TargetSpec{Label: "Connection URL", Placeholder: "redis://:password@cache:6379/0", Hint: databaseTargetHint},
		Fields: []schema.Field{
			{Key: "ignore_tls", Label: "Ignore TLS certificate errors", Input: schema.Switch, Hint: "For rediss:// with a self-signed certificate", Wide: true},
		},
		Normalize: (*Check).normalizeDatabase,
	})
}
