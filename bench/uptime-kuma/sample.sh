#!/bin/sh
# sample.sh <out.csv> <seconds>: docker stats for the three apps every 5s.
out=$1; end=$(( $(date +%s) + $2 ))
: > "$out"
while [ "$(date +%s)" -lt "$end" ]; do
  docker stats --no-stream --format '{{.Name}},{{.MemUsage}},{{.CPUPerc}}' bench-agent bench-kuma bench-kuma-slim >> "$out"
  sleep 5
done
