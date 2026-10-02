# Changelog

Notable changes to Uptimy Agent. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.2] - 2026-10-02

### Added
- The Helm chart is published with each release: `helm install uptimy-agent oci://ghcr.io/uptimy/charts/uptimy-agent`, at the same version as the agent, and listed on Artifact Hub.
- Images and charts are signed with cosign (keyless, from CI).

## [0.1.1] - 2026-10-01

### Fixed
- Status page on phones: a monitor's type badge no longer runs into its "Last change" time, and badges stay on one line.

### Added
- A one-click Railway template ("Deploy on Railway" in the README) and a live demo.

## [0.1.0] - 2026-10-01

First public release.

### Monitoring
- Healthchecks: HTTP(S) with status and keyword checks, ping (unprivileged ICMP), TCP, DNS, TLS certificate expiry, Kubernetes workloads, and PostgreSQL, MySQL and Redis (real logins and queries).
- Heartbeats for cron jobs, backups and workers: an interval or a cron schedule in any time zone with a grace period; start, fail and exit-code pings; run history with duration and the job's output; on-time rate, missed and failed runs.
- Alerts by email (SMTP), Slack, Microsoft Teams, Discord, Telegram, ntfy, PagerDuty (incidents open and resolve) and webhooks, on down and on recovery. Each channel alerts for every monitor or only chosen ones.
- Maintenance windows: no alerts during planned work, a held alert when a monitor is still down afterwards, and a notice on the status page.
- Healthchecks and heartbeats in YAML (`MONITORS_FILE` / `MONITORS_YAML`) alongside the UI.

### Status page
- Public page at `/status` with a logo (and dark-mode logo), accent color, website link and sections. Shows names with uptime or on-time rate only, never targets. Sections and their monitors are managed in the status page editor (drag to reorder, rename inline), as on the Uptimy platform. Announces maintenance, in progress and up to a week ahead.

### Uptimy
- "Watch the watcher": connect to Uptimy in one click, and Uptimy alerts from outside when the agent stops checking in, with a configurable delay and maintenance windows.
- An Uptimy alert channel: paste the webhook URL of an Uptimy Agent integration, and down and recovery alerts open and resolve incidents in Uptimy, which can trigger workflows.

### Running it
- One binary with the UI built in; Docker images, a Helm chart and a Railway template.
- Users with admin and viewer roles; light and dark themes.
- API tokens (full or read-only) for scripts and CI.
- Import monitors and notifications from Uptime Kuma (`kuma.db`), with a review step.

[Unreleased]: https://github.com/uptimy/agent/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/uptimy/agent/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/uptimy/agent/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/uptimy/agent/releases/tag/v0.1.0
