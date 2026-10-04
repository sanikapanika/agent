# Uptimy Agent vs Uptime Kuma: resource use

Measured 2026-10-04: Uptimy Agent 0.1.7 against Uptime Kuma 2.5.5 (the default image and `-slim`), both on SQLite, all three running side by side with the same HTTP monitors against the same local nginx. (The first run, on 2026-10-02 with agent 0.1.4, gave the same picture.)

## Results

| | Uptimy Agent 0.1.7 | Uptime Kuma 2.5.5 | Uptime Kuma 2.5.5-slim |
|---|---|---|---|
| Image download (arm64 / amd64) | **9.2 / 9.9 MB** | 603 / 602 MB | 180 / 182 MB |
| Image on disk | **25 MB** | 1,747 MB | 560 MB |
| Start to serving HTTP | **under 1 s** | about 3 s | about 3 s |
| Memory, no monitors | **8.4 MiB** | 126.0 MiB | 126.8 MiB |
| Memory, 50 monitors | **12.3 MiB** | 132.0 MiB | 134.8 MiB |
| Memory, 350 monitors | **25.3 MiB** | 153.5 MiB | 157.3 MiB |
| CPU average, no monitors | **0.02%** | 0.65% | 0.61% |
| CPU average / peak, 50 monitors | **0.18% / 2.8%** | 1.34% / 22.8% | 1.16% / 19.8% |
| CPU average / peak, 350 monitors | **0.93% / 9.6%** | 5.64% / 57.4% | 3.53% / 41.6% |
| Railway cost per month, 50 monitors | **$0.16** | $1.56 | $1.55 |
| Railway cost per month, 350 monitors | **$0.43** | $2.63 | $2.24 |

CPU is a percentage of one core, as `docker stats` reports it. Railway cost is average memory × $10 per GB-month plus average CPU × $20 per vCPU-month ([Railway pricing](https://railway.com/pricing), October 2026), before any plan's included usage.

In short: about **15× less memory** idle and **6× less** with 350 monitors, **4–6× less CPU** on average with 350 monitors and a far lower peak, and an image **66× smaller** to download than Kuma's default (20× smaller than `-slim`).

Per monitor, the two are closer than the totals suggest: going from 50 to 350 monitors added 13 MiB to the agent and 22–23 MiB to Kuma. Most of the difference is the baseline: a Go binary against a Node.js runtime.

## Method

- **Host:** Docker Desktop on Apple silicon (arm64), 10 CPUs and 8 GB for the VM, Docker 29.1.
- **Containers:** on one Docker network: `nginx:alpine` as the target, `ghcr.io/uptimy/agent:0.1.7`, `louislam/uptime-kuma:2.5.5` and `louislam/uptime-kuma:2.5.5-slim` with `UPTIME_KUMA_DB_TYPE=sqlite`. No limits, default settings.
- **Monitors:** HTTP GET every 60 seconds with a 10-second timeout to `http://bench-target/?m=<n>`, added at runtime, the same way for all three: through the agent's REST API (`agent.sh`) and through Kuma's socket.io API, as its UI does (`kuma.cjs`).
- **Verified load:** before each measurement, nginx's access log showed each app sending exactly as many requests per minute as it had monitors (50 and 350).
- **Sampling:** `docker stats` every 5 seconds (`sample.sh`): 2 minutes idle, and 5 minutes for each monitor count after a 2.5-minute warm-up. Averages and peaks by `summary.py`. The raw samples are in `idle.csv`, `monitors-50.csv` and `monitors-350.csv`.

## Limits

- Other containers on the same machine add noise: an earlier attempt of this run, with a busy Kubernetes cluster and another agent alongside, recorded a single 143% CPU sample for the agent (and higher averages for all three). The published run had those stopped; keep the machine quiet when reproducing.
- One machine and short windows. Memory over days (history growth, retention) wasn't measured, and the database sizes after the run (about 4 MB each) say nothing yet.
- A local, fast target. Slow or failing targets hold connections open longer, which costs both more.
- Docker Desktop runs containers in a VM; on a Linux host or in Kubernetes the absolute numbers differ, but all three ran under the same conditions.
- The apps don't do the same things. Kuma ships features the agent doesn't (more monitor types and notification services built in, a browser engine in the default image); the agent has some Kuma doesn't (Kubernetes discovery, database queries, multiple users). This compares what each costs to run, not what it offers.

## Reproduce

```bash
docker network create bench
docker run -d --name bench-target --network bench nginx:alpine
docker run -d --name bench-agent --network bench -e ADMIN_PASSWORD=bench-pass-123 ghcr.io/uptimy/agent:0.1.7
docker run -d --name bench-kuma --network bench -e UPTIME_KUMA_DB_TYPE=sqlite louislam/uptime-kuma:2.5.5
docker run -d --name bench-kuma-slim --network bench -e UPTIME_KUMA_DB_TYPE=sqlite louislam/uptime-kuma:2.5.5-slim

./sample.sh idle.csv 120 && python3 summary.py idle.csv

npm i socket.io-client@4
docker run --rm --network bench -v "$PWD:/w" -w /w alpine:3 sh -c 'apk add -q curl && sh agent.sh http://bench-agent:8080 1 50'
for k in bench-kuma bench-kuma-slim; do docker run --rm --network bench -v "$PWD:/w" -w /w node:22-alpine node kuma.cjs http://$k:3001 1 50; done
# wait 2-3 minutes, then check every app sends 50 requests a minute:
docker logs --since 60s bench-target | awk '{print $1}' | sort | uniq -c
./sample.sh monitors-50.csv 300 && python3 summary.py monitors-50.csv
```

Run the scripts from inside the Docker network, as above: some VPN and proxy clients on a host intercept connections to published ports.
