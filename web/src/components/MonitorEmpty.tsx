import { Link } from "react-router";
import { FileCode2, Plus, ShipWheel, Wrench } from "lucide-react";
import { cn } from "@/lib/utils";
import { kinds, type MonitorKind } from "@/lib/monitors";
import type { MonitorSource } from "@/lib/api";
import { buttonVariants } from "@/components/ui/button";
import { useCanEdit } from "@/components/AuthGate";

const emptyCopy: Record<MonitorKind, { title: string; body: string }> = {
  healthcheck: {
    title: "No healthchecks yet",
    body: "Check a website, API, database, port, DNS record, TLS certificate or Kubernetes workload.",
  },
  heartbeat: {
    title: "No heartbeats yet",
    body: "Cron jobs, backups and workers ping a URL each time they run. You're alerted when a run is missed or fails.",
  },
};

/** The empty state of a healthcheck or heartbeat list. */
export function MonitorEmpty({ kind, compact }: { kind: MonitorKind; compact?: boolean }) {
  const canEdit = useCanEdit();
  const copy = emptyCopy[kind];
  return (
    <div
      className={cn(
        "flex flex-col items-center gap-2 rounded-lg border border-dashed px-6 text-center",
        compact ? "py-8" : "py-16",
      )}
    >
      <h3 className="font-semibold">{copy.title}</h3>
      <p className="max-w-md text-sm text-muted-foreground">{copy.body}</p>
      {!compact && (
        <p className="max-w-md text-sm text-muted-foreground">
          You can also define them in YAML with <code className="rounded bg-muted px-1">MONITORS_FILE</code>.
        </p>
      )}
      {canEdit && (
        <Link
          to={`${kinds[kind].path}/new`}
          className={cn(buttonVariants({ size: compact ? "sm" : "default" }), "mt-2")}
        >
          <Plus /> Add {kinds[kind].noun}
        </Link>
      )}
    </div>
  );
}

/** Marks a monitor defined in the monitors file. */
/** How a monitor defined outside the UI is labeled; edit it at its source. */
export const managedLabel: Record<Exclude<MonitorSource, "ui">, string> = {
  file: "Managed by monitors file",
  kubernetes: "Discovered in Kubernetes",
};

/** Marks a monitor defined outside the UI, in lists. */
export function ManagedIcon({ source }: { source: MonitorSource }) {
  if (source === "ui") return null;
  const Icon = source === "kubernetes" ? ShipWheel : FileCode2;
  return (
    <span title={managedLabel[source]} className="text-muted-foreground">
      <Icon className="size-3.5" />
    </span>
  );
}

/** Marks a monitor in a maintenance window: it won't alert. */
export function MaintenanceBadge() {
  return (
    <span
      title="In maintenance: it won't alert"
      className="inline-flex items-center gap-1 rounded-full bg-sky-500/15 px-1.5 py-0.5 text-[11px] font-medium text-sky-600 dark:text-sky-400"
    >
      <Wrench className="size-3" /> Maintenance
    </span>
  );
}
