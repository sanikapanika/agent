#!/bin/sh
# agent.sh <base URL> <from> <to>: adds HTTP monitors FROM..TO to an Uptimy
# Agent (admin / bench-pass-123), the same as kuma.cjs does for Kuma.
set -e
# The session cookie is passed as a header: curl won't send cookies it
# stored for a host without a dot, like a Docker container name.
session=$(curl -fsS -D - -o /dev/null -H 'content-type: application/json' \
  -d '{"username":"admin","password":"bench-pass-123"}' "$1/api/auth/login" |
  sed -n 's/^[Ss]et-[Cc]ookie: \([^;]*\).*/\1/p')
for i in $(seq "$2" "$3"); do
  curl -fsS -o /dev/null -H "Cookie: $session" -H 'content-type: application/json' \
    -d "{\"name\":\"Check $i\",\"check\":{\"type\":\"http\",\"target\":\"http://bench-target/?m=$i\",\"interval_seconds\":60,\"timeout_seconds\":10,\"failure_threshold\":1}}" \
    "$1/api/healthchecks"
done
echo "$1: monitors $2-$3 added"
