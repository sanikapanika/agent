// How posted incidents and notices look, on the status page and in the
// dashboard. Colors and labels mirror the hosted Uptimy status pages.
import type { ReactNode } from "react";
import { AlertCircle, AlertTriangle, CheckCircle2, Eye, Megaphone } from "lucide-react";
import type { IncidentSeverity, IncidentStatus, PublicIncident } from "@/lib/api";
import { cn, formatDateTime } from "@/lib/utils";

export const statusTheme: Record<
  IncidentStatus,
  { label: string; icon: typeof AlertTriangle; rail: string; dot: string; chip: string; text: string }
> = {
  investigating: {
    label: "Investigating",
    icon: AlertTriangle,
    rail: "bg-down",
    dot: "bg-down",
    chip: "bg-down/10 text-red-700 ring-down/20 dark:text-red-300",
    text: "text-red-600 dark:text-red-400",
  },
  identified: {
    label: "Identified",
    icon: AlertCircle,
    rail: "bg-degraded",
    dot: "bg-degraded",
    chip: "bg-degraded/10 text-orange-700 ring-degraded/20 dark:text-orange-300",
    text: "text-orange-600 dark:text-orange-400",
  },
  monitoring: {
    label: "Monitoring",
    icon: Eye,
    rail: "bg-sky-500",
    dot: "bg-sky-500",
    chip: "bg-sky-500/10 text-sky-700 ring-sky-500/20 dark:text-sky-300",
    text: "text-sky-600 dark:text-sky-400",
  },
  resolved: {
    label: "Resolved",
    icon: CheckCircle2,
    rail: "bg-up",
    dot: "bg-up",
    chip: "bg-up/10 text-emerald-700 ring-up/20 dark:text-emerald-300",
    text: "text-emerald-600 dark:text-emerald-400",
  },
};

export const severityTheme: Record<IncidentSeverity, { label: string; dot: string }> = {
  critical: { label: "Critical", dot: "bg-down" },
  high: { label: "High", dot: "bg-orange-500" },
  medium: { label: "Medium", dot: "bg-degraded" },
  low: { label: "Low", dot: "bg-sky-500" },
};

export const STATUSES: IncidentStatus[] = ["investigating", "identified", "monitoring", "resolved"];
export const SEVERITIES: IncidentSeverity[] = ["low", "medium", "high", "critical"];

/** "2h 15m": how long it lasted, or has lasted so far. */
export function incidentDuration(from: string, to?: string | null) {
  const mins = Math.max(0, Math.floor(((to ? new Date(to) : new Date()).getTime() - new Date(from).getTime()) / 60000));
  const d = Math.floor(mins / 1440);
  const h = Math.floor((mins % 1440) / 60);
  const m = mins % 60;
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export function StatusChip({ status }: { status: IncidentStatus }) {
  const t = statusTheme[status];
  const Icon = t.icon;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset",
        t.chip,
      )}
    >
      <Icon className="size-3" />
      {t.label}
    </span>
  );
}

export function SeverityChip({ severity }: { severity: IncidentSeverity }) {
  const t = severityTheme[severity];
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
      <span className={cn("size-1.5 rounded-full", t.dot)} />
      {t.label}
    </span>
  );
}

/** The timeline, newest first: each update with its status and time. */
export function Timeline({
  updates,
  actions,
}: {
  updates: { status?: IncidentStatus | ""; message: string; time: string }[];
  /** Per-update controls, for the dashboard. */
  actions?: (index: number) => ReactNode;
}) {
  return (
    <ol className="relative flex flex-col gap-4 border-l pl-5">
      {updates.map((u, i) => (
        <li key={i} className="relative">
          <span
            className={cn(
              "absolute top-1.5 -left-[25px] size-2.5 rounded-full ring-4 ring-card",
              u.status ? statusTheme[u.status].dot : "bg-primary",
            )}
          />
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
            {u.status && (
              <span className={cn("text-sm font-semibold", statusTheme[u.status].text)}>
                {statusTheme[u.status].label}
              </span>
            )}
            <time className="text-xs text-muted-foreground tabular-nums" dateTime={u.time}>
              {formatDateTime(u.time)}
            </time>
            {actions && <span className="ml-auto">{actions(i)}</span>}
          </div>
          <p className="mt-1 text-sm leading-relaxed whitespace-pre-line break-words">{u.message}</p>
        </li>
      ))}
    </ol>
  );
}

/** An incident on the public page. */
export function IncidentCard({ incident }: { incident: PublicIncident }) {
  const status = (incident.status || "investigating") as IncidentStatus;
  const resolved = !!incident.resolved_at;
  return (
    <div className="relative overflow-hidden rounded-xl border bg-card">
      <span className={cn("absolute inset-y-0 left-0 w-[3px]", statusTheme[status].rail)} />
      <div className="p-4 sm:p-5">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="relative flex size-2.5 shrink-0">
                {!resolved && (
                  <span
                    className={cn(
                      "absolute inline-flex size-full animate-ping rounded-full opacity-60 motion-reduce:hidden",
                      statusTheme[status].dot,
                    )}
                  />
                )}
                <span className={cn("relative inline-flex size-2.5 rounded-full", statusTheme[status].dot)} />
              </span>
              <StatusChip status={status} />
              {incident.severity && <SeverityChip severity={incident.severity} />}
            </div>
            <h3 className="mt-2.5 leading-snug font-semibold">{incident.title}</h3>
            {/* Phones: the duration goes under the title instead of beside it. */}
            <p className="mt-1 text-xs text-muted-foreground sm:hidden">
              <span className="font-semibold text-foreground tabular-nums">
                {incidentDuration(incident.created_at, incident.resolved_at)}
              </span>{" "}
              ·{" "}
              {resolved
                ? `Resolved ${formatDateTime(incident.resolved_at!)}`
                : `Since ${formatDateTime(incident.created_at)}`}
            </p>
            {incident.monitors.length > 0 && (
              <p className="mt-1 text-xs text-muted-foreground">Affects: {incident.monitors.join(", ")}</p>
            )}
          </div>
          <div className="hidden shrink-0 text-right sm:block">
            <div className="text-sm font-semibold tabular-nums sm:text-base">
              {incidentDuration(incident.created_at, incident.resolved_at)}
            </div>
            <div className="text-[11px] text-muted-foreground">
              {resolved
                ? `Resolved ${formatDateTime(incident.resolved_at!)}`
                : `Since ${formatDateTime(incident.created_at)}`}
            </div>
          </div>
        </div>
        <div className="mt-4 border-t pt-4">
          <Timeline updates={incident.updates} />
        </div>
      </div>
    </div>
  );
}

/** A notice on the public page: an announcement shown until it's ended. */
export function NoticeCard({ notice }: { notice: PublicIncident }) {
  const latest = notice.updates[0];
  return (
    <div className="rounded-xl border border-primary/30 bg-primary/5 p-4 sm:p-5">
      <div className="flex gap-3">
        <Megaphone className="mt-0.5 size-5 shrink-0 text-primary" />
        <div className="min-w-0">
          <h3 className="leading-snug font-semibold">{notice.title}</h3>
          {latest && <p className="mt-1 text-sm whitespace-pre-line break-words">{latest.message}</p>}
          <p className="mt-2 text-xs text-muted-foreground">
            Posted {formatDateTime(notice.created_at)}
            {notice.monitors.length > 0 && <> · About: {notice.monitors.join(", ")}</>}
          </p>
        </div>
      </div>
    </div>
  );
}
