import type { HeartbeatState, Monitor, Status } from "./api";

export type { MonitorKind } from "./api";

// A monitor is a healthcheck (the agent probes a target) or a heartbeat (a
// scheduled job pings the agent). Their pages, forms and history differ; this
// is what they share.

export const kinds = {
  healthcheck: { path: "/healthchecks", noun: "healthcheck", title: "Healthchecks" },
  heartbeat: { path: "/heartbeats", noun: "heartbeat", title: "Heartbeats" },
} as const;

export const monitorPath = (m: Pick<Monitor, "id" | "kind">) => `${kinds[m.kind].path}/${m.id}`;

const statusOrder: Record<Status, number> = { down: 0, pending: 1, up: 2, paused: 3 };

/** Needs-attention first, then by name. */
export function byAttention(a: { status: Status; name: string }, b: { status: Status; name: string }) {
  return statusOrder[a.status] - statusOrder[b.status] || a.name.localeCompare(b.name);
}

export function countByStatus(list: { status: Status }[]) {
  const counts: Record<Status, number> = { up: 0, down: 0, pending: 0, paused: 0 };
  for (const m of list) counts[m.status]++;
  return counts;
}

/** How a heartbeat's state reads, and its color. */
export const heartbeatStates: Record<HeartbeatState, { label: string; tone: "up" | "down" | "late" | "muted" }> = {
  waiting: { label: "Waiting for the first run", tone: "muted" },
  on_time: { label: "On schedule", tone: "up" },
  late: { label: "Late", tone: "late" },
  missed: { label: "Missed a run", tone: "down" },
  failed: { label: "Failed", tone: "down" },
  paused: { label: "Paused", tone: "muted" },
};
