# Uptimy Agent

Open-source, self-hosted uptime monitoring that runs inside your cluster. It checks Services, databases and workloads by their internal names, alerts you when something goes down, and serves a public status page. One small Go binary, Apache-2.0.

- **Healthchecks:** HTTP(S) with status and keyword checks, TCP, ping, DNS, TLS certificate expiry, Postgres, MySQL and Redis (a real login and query), and Kubernetes Deployments, StatefulSets and DaemonSets with fewer ready replicas than they should have.
- **Heartbeats** for CronJobs, backups and workers: an interval or a cron schedule in any time zone, with missed and failed runs, exit codes and durations.
- **Alerts** by email, Slack, Microsoft Teams, Discord, Telegram, ntfy, PagerDuty, webhooks and Uptimy.
- **Status page** at `/status` with your logo, sections and public names. It never shows internal hostnames.
- **Auto-discovery:** label a Service, Ingress, HTTPRoute or workload with `upti.my/monitor: "true"` and it's monitored within 30 seconds.
- **Monitors in Git:** or put them in `values.yaml` and they're rendered into a ConfigMap.

[Source and docs](https://github.com/uptimy/agent) · [Live demo](https://uptimy-agent-production-b9a3.up.railway.app/status)

## Install

```bash
helm install uptimy-agent oci://ghcr.io/uptimy/charts/uptimy-agent -n monitoring --create-namespace
kubectl -n monitoring logs deploy/uptimy-agent | grep -i password
kubectl -n monitoring port-forward svc/uptimy-agent 8080:80
```

Open http://localhost:8080 and sign in as `admin` with the password from the log. You'll choose your own straight away. Set `adminPassword` or `existingSecret` to skip that.

## Auto-discovery

Label what you want monitored. A Service gets an HTTP check on its cluster DNS name, on its pods' readinessProbe path (or a TCP connect for ports that aren't HTTP), an Ingress or HTTPRoute an HTTP check per hostname, and a Deployment, StatefulSet or DaemonSet a readiness check.

Set `discovery.serviceCheck: kubernetes` (or `upti.my/type: kubernetes` on one Service) to reuse the pods' readinessProbes instead: the Service is up while it has a ready endpoint, with no traffic to the app. The default, `probe`, tests the real path through DNS, the Service and the app. Annotations adjust the check:

```yaml
metadata:
  labels:
    upti.my/monitor: "true"
  annotations:
    upti.my/name: Checkout API   # default: <namespace>/<name>
    upti.my/path: /health        # default: the readinessProbe path, else /
    upti.my/interval: 30s        # default: 1m
```

All annotations (`port`, `type`, `scheme`, `expected-status`, `keyword`) are in the [agent README](https://github.com/uptimy/agent#auto-discovery). Removing the label deletes the monitor. Discovery lists only labeled objects; turn it off with `discovery.enabled: false`.

## Monitors in values

Same schema as [`examples/monitors.yaml`](https://github.com/uptimy/agent/blob/main/examples/monitors.yaml). Monitors defined here are read-only in the UI, and changing them is a `helm upgrade`.

```yaml
healthchecks:
  - name: Checkout API
    type: http
    target: http://checkout.shop.svc.cluster.local:8080/health
    interval: 30s
  - name: Checkout deployment
    type: kubernetes
    target: shop/deployment/checkout
  - name: Postgres
    type: postgres
    target: postgres://monitor:${PG_PASSWORD}@postgres.shop.svc:5432/shop
heartbeats:
  - name: Nightly backup
    cron: "0 3 * * *"
    timezone: Europe/Berlin
    grace: 30m

# ${PG_PASSWORD} comes from a Secret, so it never lands in the ConfigMap.
extraEnv:
  - name: PG_PASSWORD
    valueFrom:
      secretKeyRef: { name: postgres-monitor, key: password }
```

## Public status page

```yaml
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: status.example.com
      paths: ["/"]
  tls:
    - secretName: status-tls
      hosts: [status.example.com]
```

## Values

| Key | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/uptimy/agent` | Image |
| `image.tag` | chart `appVersion` | Image tag |
| `adminPassword` | `""` | Password for `admin`; empty prints a random one to the log |
| `existingSecret` | `""` | Secret with key `ADMIN_PASSWORD` instead of `adminPassword` |
| `agentName` | release name | How the agent is labeled in Uptimy |
| `uptimy.heartbeatUrl` | `""` | Pin the Uptimy check-in URL (usually done with "Connect to Uptimy" in the UI) |
| `uptimy.existingSecret` | `""` | Secret with key `UPTIMY_HEARTBEAT_URL` |
| `healthchecks` / `heartbeats` | `[]` | Monitors defined in values |
| `retentionDays` | `30` | How long check results are kept |
| `discovery.enabled` | `true` | Monitor resources labeled `upti.my/monitor: "true"` |
| `discovery.serviceCheck` | `probe` | Discovered Services: `probe` (the agent's own request) or `kubernetes` (ready endpoints, from their readinessProbes) |
| `rbac.create` | `true` | Read-only access for checks (Deployments, StatefulSets, DaemonSets, Services, EndpointSlices) and discovery (listing Services, Ingresses, HTTPRoutes and pods) |
| `rbac.clusterWide` | `true` | `false` limits that access to the release namespace |
| `serviceAccount.create` / `.name` | `true` / `""` | Service account for Kubernetes checks |
| `persistence.enabled` | `true` | Keep data on a PVC (SQLite) |
| `persistence.size` / `.storageClass` / `.existingClaim` | `1Gi` / `""` / `""` | PVC settings |
| `service.type` / `.port` | `ClusterIP` / `80` | Service |
| `ingress.*` | disabled | Ingress for the UI and status page |
| `resources` | 20m CPU, 32Mi request; 128Mi limit | Container resources |
| `extraEnv` | `[]` | Extra environment variables, e.g. passwords for `${VAR}` in targets |
| `nodeSelector` / `tolerations` / `affinity` / `podAnnotations` | empty | Scheduling and pod metadata |

The agent runs as a single replica (SQLite has one writer), as non-root with a read-only root filesystem and no capabilities. Ping checks use unprivileged ICMP sockets through the `net.ipv4.ping_group_range` sysctl.

## Watch the watcher

If the agent or the cluster goes down, nothing inside is left to tell you. **Settings → Connect to Uptimy** makes the agent check in with an [Uptimy](https://www.upti.my/?utm_source=artifacthub&utm_medium=chart&utm_campaign=agent) heartbeat every minute, and Uptimy alerts you from outside when it goes quiet.
