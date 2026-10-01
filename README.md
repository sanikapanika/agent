<p align="center">
  <a href="https://www.upti.my/?utm_source=github&utm_medium=readme&utm_campaign=agent">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://www.upti.my/brand/uptimy-logo-white.svg">
      <img src="https://www.upti.my/brand/uptimy-logo.svg" alt="Uptimy" height="48">
    </picture>
  </a>
</p>

<h3 align="center">Uptimy Agent</h3>

<p align="center">
  Self-hosted uptime monitoring with a status page, built to run <b>inside</b> your cluster or project.<br>
  One small Go binary. Web UI included. Apache-2.0.
</p>

---

Uptimy Agent watches your websites, APIs, databases, DNS, TLS certificates, cron jobs and Kubernetes workloads, alerts you by email, Slack, Microsoft Teams, Discord, Telegram, ntfy, PagerDuty or webhooks, and publishes a public status page.

Because it runs next to your services, it can check things that external monitors can't reach: `postgres.default.svc:5432`, `api.railway.internal`, a Deployment's ready replicas.

- **Healthchecks:** HTTP(S) with status and keyword assertions, Postgres, MySQL and Redis (real logins and queries, not just an open port), ping, TCP, DNS, TLS expiry and Kubernetes workloads
- **Heartbeats:** cron jobs, backups and workers ping a URL when they run. Schedules are an interval or a cron expression in any time zone; you see on-time rate, missed and failed runs, exit codes and how long each run took, and you're alerted when a run is missed or fails
- **Dashboard:** both at a glance, split into Healthchecks and Heartbeats like in Uptimy
- **Alerts:** email, Slack, Microsoft Teams, Discord, Telegram, ntfy, PagerDuty, webhooks and Uptimy (alerts become incidents there, which can run workflows), fired on down and on recovery, with a consecutive-failure threshold to avoid flapping. Each channel alerts for every monitor or only the ones you choose
- **Maintenance windows:** planned work doesn't page anyone, and is announced on the status page. A monitor that's still down when the window ends alerts then
- **Status page:** public, at `/status`. Admins set a logo (with an optional dark-mode version), accent color and website link, create sections and drag monitors into order with public names, with a live preview, much like the Uptimy app. New monitors stay off the page until you add them. It shows names with uptime (healthchecks) or on-time runs (heartbeats) only, never internal hostnames
- **GitOps-friendly:** define monitors in YAML (a file, a ConfigMap or an env var), or click them together in the UI. Script everything else with [API tokens](#api)
- **Switching from Uptime Kuma:** import its monitors and notifications in a few clicks; see [below](#switching-from-uptime-kuma)
- **Light:** a single ~12 MB static binary (14 MB image) with an embedded SQLite database (pure Go, no CGO) and a distroless image. It uses about 25 MB of RAM with a handful of monitors

## Quick start

### Docker

```bash
docker run -d --name uptimy-agent -p 8080:8080 -v uptimy-agent:/data ghcr.io/uptimy/agent
```

Open http://localhost:8080 and sign in as `admin` with the password printed in the logs (`docker logs uptimy-agent`). You'll choose your own password straight away. Or pass `-e ADMIN_PASSWORD=…` to set it up front.

### Docker Compose

See [`docker-compose.yml`](docker-compose.yml).

### Kubernetes (Helm)

```bash
helm install uptimy-agent ./deploy/helm/uptimy-agent -n monitoring --create-namespace
kubectl -n monitoring port-forward svc/uptimy-agent 8080:80
```

Inside a cluster the agent uses its service account to check Deployments, StatefulSets and DaemonSets. The chart creates a read-only role for that. Monitors can live in `values.yaml`:

```yaml
healthchecks:
  - name: Checkout API
    type: http
    target: http://checkout.shop.svc.cluster.local:8080/health
    interval: 30s
  - name: Checkout deployment
    type: kubernetes
    target: shop/deployment/checkout
heartbeats:
  - name: Nightly backup
    cron: "0 3 * * *"
    timezone: Europe/Berlin
```

### Railway

Deploy from this repo (it includes [`railway.json`](railway.json)) and add a volume mounted at `/data`. See [deploy/railway.md](deploy/railway.md) for the template settings. Services in the same project are reachable over private networking, so you can monitor `*.railway.internal` hosts that aren't exposed publicly.

## Configuration

Environment variables set how the agent runs, its secrets, and (optionally) monitors as code. Everything else, including the status page, alert channels and Uptimy alerting, is set in the UI and kept in the agent's database.

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` | `8080` | HTTP port |
| `DATA_DIR` | `./data` (`/data` in the image) | Where the SQLite database lives |
| `ADMIN_USERNAME` | `admin` | Username of the default admin created on first start |
| `ADMIN_PASSWORD` | – | That admin's password (min 8 chars). If unset, a random one is printed in the log on first start and must be changed at first sign-in. Setting it later resets the password, which is also how you recover a lost admin login |
| `MONITORS_FILE` | – | Path to a YAML file of monitors |
| `MONITORS_YAML` | – | The YAML itself, for platforms where mounting files is awkward |
| `RETENTION_DAYS` | `30` | How long check results, heartbeat runs and events are kept |
| `UPTIMY_HEARTBEAT_URL` | – | Optional. Pins the [Watch the watcher](#watch-the-watcher) heartbeat, for agents without a persistent volume; otherwise connect it in the UI |
| `AGENT_NAME` | hostname | How this agent is labelled in Uptimy |

### Monitors in YAML

The file has two lists, `healthchecks` and `heartbeats`; see [`examples/monitors.yaml`](examples/monitors.yaml) for every check type and both kinds of schedule. Monitors from YAML are matched by name: changing one keeps its history, and removing one deletes it on the next start. They're shown read-only in the UI, though you can still pause them during maintenance and add them to the status page. Monitors created in the UI are never touched by the file. Unknown settings are an error, so a typo doesn't go unnoticed.

### Users and roles

The agent starts with one admin account (see `ADMIN_USERNAME` / `ADMIN_PASSWORD`). Admins can add people under **Users**:

- **Admin:** can change everything, including users
- **Viewer:** can see monitors, history and settings, but not change them. Credentials such as webhook URLs, auth headers, database passwords and heartbeat ping URLs are hidden from viewers

New users get a temporary password to pass on and choose their own at first sign-in. Admins can reset a password or change a role at any time; the agent won't let you delete or demote yourself or remove the last admin.

### Database monitors

Postgres, MySQL and Redis monitors open a fresh connection on every check, log in, run a query (`SELECT 1` by default; `PING` for Redis) and disconnect. They catch what a TCP check can't: rejected logins, a full connection limit, a server that accepts connections but can't answer.

| Type | Target |
| --- | --- |
| `postgres` | `postgres://user:password@host:5432/db?sslmode=require` |
| `mysql` | `mysql://user:password@host:3306/db?tls=true` (`tls`: `true`, `false`, `skip-verify` or `preferred`, the default) |
| `redis` | `redis://:password@host:6379/0`, or `rediss://` for TLS |

- **Keep passwords out of the agent:** write `${VAR}` for the whole URL or just the password, and the agent reads that environment variable when the check runs, e.g. from a Kubernetes Secret or a Railway reference variable. The agent's own settings (`ADMIN_PASSWORD`, `UPTIMY_*`, ...) can't be referenced.
- **Custom query:** for Postgres and MySQL, set `query` and optionally `expected`, compared with the first column of the first row. For example, `SELECT pg_is_in_recovery()` with `expected: "false"` alerts when the database stops being the primary.
- **Read-only:** queries run in a read-only session, one statement at a time, and connections show up as `uptimy-agent`. That's a guard against mistakes, not a permission system: create a dedicated read-only user for monitoring.
- Passwords are masked everywhere the target is shown (lists, alerts) and hidden from viewers. The status page never shows targets.
- Postgres checks work through PgBouncer in transaction mode.

### Heartbeats

Healthchecks ask "is it up?". Heartbeats ask "did the job run, on time, and did it succeed?", for cron jobs, backups, queue workers and anything else that runs on a schedule. Create one under **Heartbeats** and have the job call its ping URL:

```bash
0 3 * * *  /usr/local/bin/backup.sh && curl -fsS -m 10 --retry 3 https://agent.example.com/ping/<token>
```

| URL | Meaning |
| --- | --- |
| `/ping/<token>` | The job finished |
| `/ping/<token>/start` | The job started. Its finish then records how long it took |
| `/ping/<token>/fail` | The job failed |
| `/ping/<token>/<exit code>` | 0 is success; 1 to 255 is a failure, e.g. `curl …/ping/<token>/$?` |

GET and POST both work. A POST body (say, the end of the job's log) is kept with the run, up to 500 characters.

- **Schedule:** a fixed interval (`every: 6h`) or a cron expression with a time zone (`cron: "0 3 * * *"`, `timezone: Europe/Berlin`), so daylight saving time is handled.
- **Grace period:** how late a run may be before it counts as missed (5 minutes by default). You're alerted when a run is missed or a job reports a failure, and again when it's back on schedule.
- **History:** each run is kept with its result, how long it took and its message; the heartbeat's page shows the on-time rate over 24 hours, 7 days and 30 days.
- **Ping URLs** are generated and stay the same when you edit the heartbeat. Replace one from its page if it leaks. In YAML, set `token` to pin it (16 to 64 letters, digits, `-` or `_`); otherwise one is generated and kept.

### Ping

Ping monitors send three ICMP echo requests and are up if any reply comes back. The agent uses unprivileged ICMP sockets, so it needs no root or `NET_RAW`: Docker allows them by default, and the Helm chart sets the `net.ipv4.ping_group_range` sysctl. Elsewhere on Linux, run `sysctl -w net.ipv4.ping_group_range="0 2147483647"`.

### Alert routing

A channel alerts for every monitor (the default, including monitors added later) or only for the monitors you choose. Set it on the channel under **Notifications**, or tick channels under **Alerts** when editing a monitor.

### Maintenance

Schedule maintenance under **Maintenance** before deploys, upgrades or migrations: for all monitors or some, starting now or later. During the window the monitors keep being checked, but nobody is alerted. If one is still down when the window ends, its alert goes out then; if it recovered, nothing is sent. Public windows are announced on the status page up to a week ahead, and the affected monitors are marked as under maintenance rather than down.

## API

Everything in the UI goes through a JSON API under `/api`. For scripts and CI, create a token under **Account → API tokens** and send it as a bearer token:

```bash
curl -H "Authorization: Bearer upa_…" https://agent.example.com/api/healthchecks
```

A token acts as the user who made it. Choose read-only for dashboards and exporters; a viewer's tokens are always read-only. Tokens can't manage accounts, users or other tokens; that takes signing in.

## Switching from Uptime Kuma

Under **Settings → Import from Uptime Kuma**, upload Kuma's database (`kuma.db`, from its data folder: `/app/data` in Docker). Stop Kuma first so the copy is complete:

```bash
docker stop uptime-kuma && docker cp uptime-kuma:/app/data/kuma.db .
```

You'll see what each monitor and notification becomes, and choose what to import. HTTP, keyword, TCP port, ping, DNS, TLS, Postgres, MySQL, Redis and push monitors carry over, with their intervals, retries, accepted status codes, headers and basic or bearer auth. So do email, Slack, Teams, Discord, Telegram, ntfy, PagerDuty and webhook notifications, along with which monitors alert where. Monitors that were on a Kuma status page are added to yours. Push monitors become heartbeats with new ping URLs, so update your jobs. Anything that can't be carried over (Docker, gRPC, inverted checks, ...) is listed with the reason. Kuma installs that use MariaDB aren't supported yet.

## Watch the watcher

A self-hosted monitor has one blind spot: itself. If the agent, its node, or the whole cluster goes down, nothing is left to tell you.

Open **Settings → Watch the watcher** and click **Connect to Uptimy**. Sign in to [Uptimy](https://www.upti.my/?utm_source=github&utm_medium=readme&utm_campaign=agent-heartbeat) (free), pick a workspace, and approve. The agent creates its own heartbeat and starts checking in every minute. If it stops, Uptimy alerts you from outside.

- The agent receives an **agent-scoped key**, which can only manage this agent's own heartbeat, not the rest of your workspace. It stays on the agent's server and is revoked when you click Disconnect, which also deletes the heartbeat.
- Reconnecting (say, after reinstalling) reuses the same heartbeat, so you keep its history and don't end up with orphaned heartbeats.
- **When you're alerted:** by default after 5 minutes of silence, enough to ride out a restart or redeploy. Choose 2, 5, 15 or 30 minutes in the same card. The setting lives on the heartbeat in Uptimy, so a change made there (or with `uptimyctl heartbeats update`) shows up here too.
- **Planned work:** **Allow downtime** for 30 minutes, 1 hour or 4 hours before working on the agent or its server. This doesn't pause the heartbeat: it pushes its alert deadline back, so the heartbeat stays active in Uptimy and still alerts if the agent isn't back when the window ends, and the agent restores normal alerting afterwards. **Pause until I resume** is there too, but nothing alerts until you resume.
- Set `AGENT_NAME` to label the agent in Uptimy; the default is the hostname, which in Kubernetes is the pod name.
- Prefer to set it up by hand? Choose **Or paste a heartbeat URL instead**, or set `UPTIMY_HEARTBEAT_URL`, which pins the connection read-only for GitOps setups.

## Development

You'll need Go 1.26+ and Node 22+.

```bash
make run    # http://localhost:8080, sign in as admin / uptimy-dev
make test   # Go and UI tests
make lint   # golangci-lint, ESLint and Prettier, as in CI
```

`make run` gives you the same single URL as production: the Go agent serves the API and the UI, proxying the UI to an internal Vite server so edits hot-reload, and saving a `.go` file rebuilds and restarts the agent.

Adding a monitor type or a notification channel is a small, self-contained change: see [CONTRIBUTING.md](CONTRIBUTING.md) for a walk-through and [ARCHITECTURE.md](ARCHITECTURE.md) for how the pieces fit together. Please report security issues privately, as described in [SECURITY.md](SECURITY.md).

## License

[Apache-2.0](LICENSE)
