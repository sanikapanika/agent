# Changelog

Notable changes to Uptimy Agent. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Settings → Kubernetes: discovery's status (last scan, scope, monitors found) and every problem it found, such as a mistyped label value or an invalid annotation, plus a browser of the cluster's resources, monitored or not, with the `kubectl label` command for one or for all shown. Empty healthcheck and heartbeat pages point to it when the agent runs in a cluster.
- `upti.my/monitor` also accepts `yes`, `1` and `on`; `false`, `no`, `0` and `off` opt a resource out; other values are reported instead of ignored.
- Discovered monitors show the object they came from ("Discovered in Kubernetes · service shop/checkout").

### Changed
- Discovered monitors are tied to their Kubernetes object instead of their name: renaming one with `upti.my/name` keeps its history, and two objects may share a name. Monitors discovered by earlier versions are matched up on the first scan.
- CronJobs without pings: label a CronJob `upti.my/monitor: "true"` and the agent creates a heartbeat on its schedule and time zone, and records each run from the CronJob's Jobs at the times Kubernetes saw them: start, finish, duration, and for a failure the reason with the container's exit code. Missed runs are detected from the schedule. Suspending the CronJob pauses the heartbeat. `upti.my/grace` sets how long after its scheduled time a run may finish (default: the Job's `activeDeadlineSeconds` plus a minute, or else the time between runs, at most an hour).
- A "More services (Shoutrrr)" alert channel: one URL reaches any of 30+ services, including Pushover, Gotify, Matrix, Google Chat, Mattermost, Rocket.Chat, Opsgenie, Signal, Zulip and Home Assistant. Errors never repeat the URL or its credentials.
- Discovered Services are checked on their pods' readinessProbe path (and scheme) instead of `/`, and ports with an HTTP readinessProbe get an HTTP check even when they don't look like HTTP.
- Kubernetes Service checks (`<namespace>/service/<name>`): up while the Service has a ready endpoint, reusing the pods' readinessProbes with no traffic to the app. Discovery uses them for Services annotated `upti.my/type: kubernetes`.
- The chart lets the agent read Services and EndpointSlices, and list pods when discovery is on.

## [0.1.3] - 2026-10-02

### Added
- Kubernetes auto-discovery: label a Service, Ingress, Gateway API HTTPRoute, Deployment, StatefulSet or DaemonSet with `upti.my/monitor: "true"` and the agent monitors it within 30 seconds. `upti.my/` annotations set the name, path, port, interval and more. Discovered monitors are read-only in the UI and are removed with the label. On by default in the chart (`discovery.enabled`), which now lets the agent list those resources.
- The Helm chart is published with each release: `helm install uptimy-agent oci://ghcr.io/uptimy/charts/uptimy-agent`, at the same version as the agent, and listed on Artifact Hub.
- Images and charts are signed with cosign (keyless, from CI).

## [0.1.2] - 2026-10-02

Not published: the release build failed before the image and chart were pushed. Use 0.1.3.

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

[Unreleased]: https://github.com/uptimy/agent/compare/v0.1.3...HEAD
[0.1.3]: https://github.com/uptimy/agent/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/uptimy/agent/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/uptimy/agent/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/uptimy/agent/releases/tag/v0.1.0
