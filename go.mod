module github.com/uptimy/agent

go 1.26.0

// The UI's npm packages aren't part of the module (one ships Go code).
ignore ./web/node_modules

require (
	github.com/go-sql-driver/mysql v1.10.1
	github.com/jackc/pgx/v5 v5.11.0
	github.com/robfig/cron/v3 v3.0.1
	golang.org/x/crypto v0.57.0
	golang.org/x/net v0.59.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.60.1
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	modernc.org/libc v1.77.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
