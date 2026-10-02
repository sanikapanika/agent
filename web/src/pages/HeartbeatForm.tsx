import { useEffect, useMemo, useState } from "react";
import { Navigate, useNavigate, useParams } from "react-router";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity } from "lucide-react";
import { api, type HeartbeatInput, type HeartbeatSpec } from "@/lib/api";
import { cn, formatSpan, nearestStep } from "@/lib/utils";
import { monitorPath } from "@/lib/monitors";
import { Field, Input, Select } from "@/components/ui/input";
import { ErrorNote } from "@/components/Layout";
import { FormShell } from "@/components/MonitorPage";
import { ChannelPicker } from "@/components/MonitorPicker";
import { useCanEdit } from "@/components/AuthGate";

// A heartbeat runs on a fixed interval ("every 6 hours") or a cron schedule
// in a time zone ("0 3 * * *" in Europe/Berlin). The agent validates the
// schedule and says when it would run next as it's typed.

type Unit = "minutes" | "hours" | "days";
const unitSeconds: Record<Unit, number> = { minutes: 60, hours: 3600, days: 86400 };

/** 7200 → { value: 2, unit: "hours" }. */
function toUnit(seconds: number, units: Unit[] = ["days", "hours", "minutes"]) {
  const unit = units.find((u) => seconds % unitSeconds[u] === 0) ?? "minutes";
  return { value: Math.max(1, Math.round(seconds / unitSeconds[unit])), unit };
}

// Slider stops, in seconds, within what the agent accepts: an interval of 1
// minute to 366 days, a grace period of 1 minute to 7 days. The exact value
// can still be typed below the slider.
const intervalSteps = [
  60, 120, 300, 600, 900, 1800, 3600, 7200, 21600, 43200, 86400, 172800, 604800, 1209600, 2592000, 7776000, 15552000,
  31536000,
];
const intervalMarks = { 0: "1m", 6: "1h", 10: "1d", 14: "30d", 17: "1y" };
const graceSteps = [60, 120, 300, 600, 900, 1800, 3600, 7200, 21600, 43200, 86400, 172800, 604800];
const graceMarks = { 0: "1m", 6: "1h", 10: "1d", 12: "7d" };

const cronPresets = [
  { label: "Hourly", cron: "0 * * * *" },
  { label: "Daily at 03:00", cron: "0 3 * * *" },
  { label: "Weekdays at 09:00", cron: "0 9 * * 1-5" },
  { label: "Sundays at 03:00", cron: "0 3 * * 0" },
  { label: "Monthly, on the 1st", cron: "0 3 1 * *" },
];

const browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";

interface FormState {
  name: string;
  paused: boolean;
  mode: "every" | "cron";
  every: { value: number; unit: Unit };
  cron: string;
  timezone: string;
  grace: { value: number; unit: Unit };
  notifierIDs: number[];
}

const empty: FormState = {
  name: "",
  paused: false,
  mode: "every",
  every: { value: 1, unit: "hours" },
  cron: "0 3 * * *",
  timezone: browserZone,
  grace: { value: 5, unit: "minutes" },
  notifierIDs: [],
};

function fromSpec(name: string, paused: boolean, h: HeartbeatSpec, notifierIDs: number[]): FormState {
  return {
    notifierIDs,
    name,
    paused,
    mode: h.cron ? "cron" : "every",
    every: h.every_seconds ? toUnit(h.every_seconds) : empty.every,
    cron: h.cron || empty.cron,
    timezone: h.timezone || "UTC",
    grace: toUnit(h.grace_seconds),
  };
}

/** The schedule as the API takes it. */
function toSpec(f: FormState): HeartbeatInput["heartbeat"] {
  const grace_seconds = f.grace.value * unitSeconds[f.grace.unit];
  return f.mode === "cron"
    ? { cron: f.cron.trim(), timezone: f.timezone, grace_seconds }
    : { every_seconds: f.every.value * unitSeconds[f.every.unit], grace_seconds };
}

export function HeartbeatForm() {
  const params = useParams();
  const editing = params.id ? Number(params.id) : null;
  const canEdit = useCanEdit();
  const existing = useQuery({
    queryKey: ["heartbeat", editing],
    queryFn: () => api.heartbeat(editing!),
    enabled: editing != null,
  });

  if (!canEdit) return <Navigate to="/heartbeats" replace />;
  if (editing == null) return <HeartbeatFormBody editing={null} initial={empty} />;
  if (existing.error) return <ErrorNote error={existing.error} />;
  if (!existing.data) return null;
  const h = existing.data.heartbeat;
  if (h.source !== "ui") return <Navigate to={monitorPath(h)} replace />;
  return (
    <HeartbeatFormBody
      key={h.id}
      editing={h.id}
      initial={fromSpec(h.name, h.paused, h.heartbeat, existing.data.notifier_ids)}
    />
  );
}

function HeartbeatFormBody({ editing, initial }: { editing: number | null; initial: FormState }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [form, setForm] = useState(initial);
  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => setForm((f) => ({ ...f, [key]: value }));

  const save = useMutation({
    mutationFn: () => {
      const input: HeartbeatInput = {
        name: form.name,
        paused: form.paused,
        heartbeat: toSpec(form),
        notifier_ids: form.notifierIDs,
      };
      return editing ? api.updateHeartbeat(editing, input) : api.createHeartbeat(input);
    },
    onSuccess: (h) => {
      qc.invalidateQueries({ queryKey: ["heartbeats"], exact: true });
      qc.invalidateQueries({ queryKey: ["heartbeat", h.id] });
      navigate(monitorPath(h));
    },
  });

  return (
    <FormShell
      kind="heartbeat"
      editing={editing}
      icon={Activity}
      description={editing ? undefined : "After you save, you get a URL for your job to call each time it runs."}
      error={save.error}
      submitting={save.isPending}
      onSubmit={() => save.mutate()}
    >
      <Field label="Name" htmlFor="name">
        <Input
          id="name"
          required
          maxLength={100}
          value={form.name}
          onChange={(e) => set("name", e.target.value)}
          placeholder="Nightly backup"
        />
      </Field>

      <fieldset className="flex flex-col gap-4">
        <legend className="mb-2 text-sm font-medium">Schedule</legend>
        <div className="flex w-fit gap-1 rounded-md bg-muted p-0.5" role="radiogroup" aria-label="Schedule type">
          {(
            [
              ["every", "Fixed interval"],
              ["cron", "Cron expression"],
            ] as const
          ).map(([mode, label]) => (
            <button
              key={mode}
              type="button"
              role="radio"
              aria-checked={form.mode === mode}
              onClick={() => set("mode", mode)}
              className={cn(
                "rounded px-3 py-1.5 text-sm font-medium",
                form.mode === mode ? "bg-card shadow-xs" : "text-muted-foreground",
              )}
            >
              {label}
            </button>
          ))}
        </div>

        {form.mode === "every" ? (
          <Field label="Runs every" htmlFor="every" hint="The first run is due one interval after you save.">
            <DurationSlider
              id="every"
              steps={intervalSteps}
              marks={intervalMarks}
              value={form.every}
              onChange={(v) => set("every", v)}
              units={["minutes", "hours", "days"]}
            />
          </Field>
        ) : (
          <>
            <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
              <Field
                label="Cron expression"
                htmlFor="cron"
                hint={
                  <>
                    Minute, hour, day of month, month, day of week. <code>@daily</code> and <code>@hourly</code> work
                    too.
                  </>
                }
              >
                <Input
                  id="cron"
                  required
                  value={form.cron}
                  onChange={(e) => set("cron", e.target.value)}
                  className="font-mono"
                  autoComplete="off"
                  spellCheck={false}
                />
              </Field>
              <Field label="Time zone" htmlFor="timezone">
                <TimezoneSelect id="timezone" value={form.timezone} onChange={(tz) => set("timezone", tz)} />
              </Field>
            </div>
            <div className="flex flex-wrap gap-1.5">
              {cronPresets.map((p) => (
                <button
                  key={p.cron}
                  type="button"
                  onClick={() => set("cron", p.cron)}
                  className={cn(
                    "rounded-full border px-2.5 py-1 text-xs transition-colors",
                    form.cron.trim() === p.cron ? "border-primary bg-primary/5" : "hover:bg-muted",
                  )}
                >
                  {p.label}
                </button>
              ))}
            </div>
          </>
        )}

        <SchedulePreview spec={toSpec(form)} />
      </fieldset>

      <Field
        label="Grace period"
        htmlFor="grace"
        hint="How late a run can be before it counts as missed and you're alerted. Leave room for slow runs."
      >
        <DurationSlider
          id="grace"
          steps={graceSteps}
          marks={graceMarks}
          value={form.grace}
          onChange={(v) => set("grace", v)}
          units={["minutes", "hours", "days"]}
        />
      </Field>

      <ChannelPicker ids={form.notifierIDs} onChange={(ids) => set("notifierIDs", ids)} />
    </FormShell>
  );
}

/**
 * A duration picked on a slider of common values (1m, 10m, 1h, 1d, ...),
 * as on the Uptimy platform, with the exact value editable below it.
 */
function DurationSlider({
  id,
  steps,
  marks,
  value,
  onChange,
  units,
}: {
  id: string;
  steps: number[];
  marks: Record<number, string>;
  value: { value: number; unit: Unit };
  onChange: (v: { value: number; unit: Unit }) => void;
  units: Unit[];
}) {
  const seconds = value.value * unitSeconds[value.unit];
  const last = steps.length - 1;
  // A stop shows in its largest whole unit: 1 day, not 1440 minutes.
  const largestFirst = [...units].sort((a, b) => unitSeconds[b] - unitSeconds[a]);
  // The thumb is 16px wide and stays inside the track, so the stops run
  // from 8px in on each side.
  const at = (i: number) => `calc(${(i / last) * 100}% + ${8 - (i / last) * 16}px)`;
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-start gap-4">
        <div className="min-w-0 flex-1">
          <input
            id={id}
            type="range"
            min={0}
            max={last}
            step={1}
            value={nearestStep(seconds, steps)}
            aria-valuetext={formatSpan(seconds)}
            onChange={(e) => onChange(toUnit(steps[Number(e.target.value)], largestFirst))}
            className="h-4 w-full cursor-pointer rounded-full accent-primary"
          />
          <div className="relative mt-1 h-4 text-xs text-muted-foreground" aria-hidden>
            {Object.entries(marks).map(([i, label]) => (
              <span key={i} className="absolute -translate-x-1/2 tabular-nums" style={{ left: at(Number(i)) }}>
                {label}
              </span>
            ))}
          </div>
        </div>
        {/* A fixed width, so the track doesn't resize as the label changes. */}
        <span className="w-28 shrink-0 rounded-full bg-primary py-0.5 text-center text-sm font-semibold whitespace-nowrap text-primary-foreground tabular-nums">
          {formatSpan(seconds)}
        </span>
      </div>
      <div className="flex items-center gap-3">
        <span className="text-xs text-muted-foreground">Exactly</span>
        <DurationInput id={`${id}-exact`} value={value} onChange={onChange} units={units} />
      </div>
    </div>
  );
}

function DurationInput({
  id,
  value,
  onChange,
  units,
}: {
  id: string;
  value: { value: number; unit: Unit };
  onChange: (v: { value: number; unit: Unit }) => void;
  units: Unit[];
}) {
  return (
    <div className="flex max-w-xs gap-2">
      <Input
        id={id}
        type="number"
        min={1}
        required
        value={value.value}
        onChange={(e) => onChange({ ...value, value: Number(e.target.value) })}
        className="w-24"
      />
      <Select
        aria-label="Unit"
        value={value.unit}
        onChange={(e) => onChange({ ...value, unit: e.target.value as Unit })}
        className="w-auto"
      >
        {units.map((u) => (
          <option key={u} value={u}>
            {u}
          </option>
        ))}
      </Select>
    </div>
  );
}

function TimezoneSelect({ id, value, onChange }: { id: string; value: string; onChange: (tz: string) => void }) {
  const zones = useMemo(() => {
    const all = new Set(["UTC", ...Intl.supportedValuesOf("timeZone")]);
    all.add(value); // keep a saved zone the browser doesn't list
    return [...all];
  }, [value]);
  return (
    <Select id={id} value={value} onChange={(e) => onChange(e.target.value)}>
      {zones.map((z) => (
        <option key={z} value={z}>
          {z.replaceAll("_", " ")}
        </option>
      ))}
    </Select>
  );
}

/** Checks the schedule with the agent as it's typed and lists the next runs. */
function SchedulePreview({ spec }: { spec: HeartbeatInput["heartbeat"] }) {
  const debounced = useDebounced(spec, 300);
  const preview = useQuery({
    queryKey: ["schedule-preview", debounced],
    queryFn: () => api.previewSchedule(debounced),
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  });
  const data = preview.data;
  if (!data) return null;
  if (data.error !== undefined) {
    return <p className="rounded-md bg-down/10 px-3 py-2 text-sm text-down">{data.error}</p>;
  }
  const zone = spec.timezone || browserZone;
  return (
    <div className="rounded-md border bg-muted/40 px-3 py-2.5 text-sm" aria-live="polite">
      <span className="text-muted-foreground">Next runs: </span>
      {data.upcoming
        .map((t) =>
          new Date(t).toLocaleString(undefined, {
            weekday: "short",
            month: "short",
            day: "numeric",
            hour: "2-digit",
            minute: "2-digit",
            timeZone: zone,
          }),
        )
        .join(" · ")}
      {spec.cron && zone !== browserZone && <span className="text-muted-foreground"> ({zone})</span>}
    </div>
  );
}

function useDebounced<T>(value: T, ms: number) {
  const key = JSON.stringify(value);
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setDebounced(JSON.parse(key)), ms);
    return () => window.clearTimeout(t);
  }, [key, ms]);
  return debounced;
}
