import { useQuery } from "@tanstack/react-query";
import { api, type MonitorKind } from "@/lib/api";
import { kinds } from "@/lib/monitors";
import { cn } from "@/lib/utils";

/** Every monitor, healthchecks first, as { id, name, kind }. */
export function useAllMonitors() {
  const healthchecks = useQuery({ queryKey: ["healthchecks"], queryFn: api.healthchecks });
  const heartbeats = useQuery({ queryKey: ["heartbeats"], queryFn: api.heartbeats });
  const byName = (a: { name: string }, b: { name: string }) => a.name.localeCompare(b.name);
  return {
    monitors: [...(healthchecks.data ?? []).slice().sort(byName), ...(heartbeats.data ?? []).slice().sort(byName)].map(
      (m) => ({ id: m.id, name: m.name, kind: m.kind }),
    ),
    error: healthchecks.error ?? heartbeats.error,
    isLoading: healthchecks.isLoading || heartbeats.isLoading,
  };
}

/**
 * "All monitors" or a chosen few: the scope of an alert channel or a
 * maintenance window.
 */
export function MonitorScope({
  all,
  ids,
  onChange,
  allLabel,
  someLabel,
}: {
  all: boolean;
  ids: number[];
  onChange: (all: boolean, ids: number[]) => void;
  allLabel: string;
  someLabel: string;
}) {
  const { monitors, isLoading } = useAllMonitors();
  const toggle = (id: number) => onChange(false, ids.includes(id) ? ids.filter((i) => i !== id) : [...ids, id]);
  const groups = (["healthcheck", "heartbeat"] as MonitorKind[])
    .map((kind) => ({ kind, items: monitors.filter((m) => m.kind === kind) }))
    .filter((g) => g.items.length > 0);

  return (
    <fieldset className="flex flex-col gap-3">
      <div className="flex flex-col gap-2 text-sm">
        <label className="inline-flex cursor-pointer items-center gap-2">
          <input type="radio" checked={all} onChange={() => onChange(true, [])} className="accent-primary" />
          {allLabel}
        </label>
        <label className="inline-flex cursor-pointer items-center gap-2">
          <input type="radio" checked={!all} onChange={() => onChange(false, ids)} className="accent-primary" />
          {someLabel}
        </label>
      </div>
      {!all && (
        <div className="max-h-64 overflow-y-auto rounded-lg border p-2">
          {isLoading && <p className="p-2 text-sm text-muted-foreground">Loading monitors…</p>}
          {!isLoading && monitors.length === 0 && <p className="p-2 text-sm text-muted-foreground">No monitors yet.</p>}
          {groups.map((g) => (
            <div key={g.kind} className="mb-1 last:mb-0">
              <div className="px-2 pt-1 pb-1 text-xs font-medium text-muted-foreground">{kinds[g.kind].title}</div>
              {g.items.map((m) => (
                <label
                  key={m.id}
                  className={cn(
                    "flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-nav-hover",
                    ids.includes(m.id) && "font-medium",
                  )}
                >
                  <input
                    type="checkbox"
                    checked={ids.includes(m.id)}
                    onChange={() => toggle(m.id)}
                    className="accent-primary"
                  />
                  <span className="truncate">{m.name}</span>
                </label>
              ))}
            </div>
          ))}
        </div>
      )}
    </fieldset>
  );
}

/**
 * The alert channels for one monitor. Channels for every monitor are shown
 * as always on; the others can be ticked.
 */
export function ChannelPicker({ ids, onChange }: { ids: number[]; onChange: (ids: number[]) => void }) {
  const notifiers = useQuery({ queryKey: ["notifiers"], queryFn: api.notifiers });
  if (!notifiers.data) return null;
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-1 text-sm font-medium">Alerts</legend>
      {notifiers.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No alert channels yet. Add one under Notifications to get alerted.
        </p>
      ) : (
        <div className="flex flex-col gap-1">
          {notifiers.data.map((n) => {
            const checked = n.all_monitors || ids.includes(n.id);
            return (
              <label
                key={n.id}
                className={cn(
                  "flex items-center gap-2 text-sm",
                  n.all_monitors ? "cursor-default" : "cursor-pointer",
                  !n.enabled && "opacity-60",
                )}
              >
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={n.all_monitors}
                  onChange={() => onChange(ids.includes(n.id) ? ids.filter((i) => i !== n.id) : [...ids, n.id])}
                  className="accent-primary"
                />
                <span>{n.name}</span>
                {n.all_monitors && <span className="text-xs text-muted-foreground">· every monitor</span>}
                {!n.enabled && <span className="text-xs text-muted-foreground">· disabled</span>}
              </label>
            );
          })}
        </div>
      )}
    </fieldset>
  );
}
