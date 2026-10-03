// Public status page. Layout, copy and colours mirror the hosted Uptimy
// status pages (upti.my-status) so every agent status page reads as Uptimy.
import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertCircle, AlertTriangle, CheckCircle, ExternalLink, XCircle, Wrench } from "lucide-react";
import { api, type PublicMonitor, type PublicStatus, type Status, type PublicMaintenance } from "@/lib/api";
import { cn, formatDateTime, timeAgo } from "@/lib/utils";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DailyBars } from "@/components/status";
import { PoweredBy } from "@/components/brand";
import { Moon, Sun } from "lucide-react";
import { useTheme } from "@/lib/theme";
import { ErrorNote } from "@/components/Layout";
import { IncidentCard, NoticeCard } from "@/components/status-page/incidents";

const brandText = "text-[#268256] dark:text-[#65bd91]"; // emerald-600 / 400 in the Uptimy ramp

const banners: Record<
  PublicStatus["overall"],
  {
    icon: typeof CheckCircle;
    iconColor: string;
    iconBg: string;
    tint: string;
    titleColor: string;
    title: string;
    subtitle: string;
  }
> = {
  operational: {
    icon: CheckCircle,
    iconColor: "text-up",
    iconBg: "bg-up/10",
    tint: "bg-up/5",
    titleColor: brandText,
    title: "All Systems Operational",
    subtitle: "All systems are running smoothly",
  },
  maintenance: {
    icon: Wrench,
    iconColor: "text-sky-500",
    iconBg: "bg-sky-500/10",
    tint: "bg-sky-500/5",
    titleColor: "text-sky-600 dark:text-sky-400",
    title: "Maintenance in Progress",
    subtitle: "Some services are under planned maintenance",
  },
  degraded: {
    icon: AlertTriangle,
    iconColor: "text-degraded",
    iconBg: "bg-degraded/10",
    tint: "bg-degraded/5",
    titleColor: "text-orange-600 dark:text-orange-400",
    title: "Some Issues Detected",
    subtitle: "Some systems are experiencing issues",
  },
  outage: {
    icon: XCircle,
    iconColor: "text-down",
    iconBg: "bg-down/10",
    tint: "bg-down/5",
    titleColor: "text-red-600 dark:text-red-400",
    title: "Outage Detected",
    subtitle: "We're working to resolve the issues",
  },
};

const accents: Record<Status, { border: string; dot: string; live?: boolean }> = {
  up: { border: "border-l-up", dot: "bg-up" },
  down: { border: "border-l-down", dot: "bg-down", live: true },
  pending: { border: "border-l-no-data", dot: "bg-pending" },
  paused: { border: "border-l-no-data", dot: "bg-pending" },
};

export function StatusPage() {
  // ?preview is set by the status page editor's preview frame.
  const preview = new URLSearchParams(window.location.search).has("preview");
  const status = useQuery({
    queryKey: ["public-status"],
    queryFn: () => api.publicStatus(preview),
    refetchInterval: 30_000,
  });
  const s = status.data;
  const title = s?.title;
  const notices = s?.incidents.filter((i) => i.kind === "notice") ?? [];
  const active = s?.incidents.filter((i) => i.kind === "incident" && !i.resolved_at) ?? [];
  const past = s?.incidents.filter((i) => i.kind === "incident" && i.resolved_at) ?? [];
  useEffect(() => {
    if (title) document.title = title;
  }, [title]);

  return (
    <div
      className="status-surface min-h-dvh overflow-x-hidden"
      // Section headings use the page's accent color (theme.primaryColor on
      // hosted pages); unset = Uptimy green.
      style={s?.accent_color ? ({ "--status-accent": s.accent_color } as React.CSSProperties) : undefined}
    >
      <div className="mx-auto max-w-4xl px-4 py-6 sm:px-6 sm:py-8 lg:px-8 lg:py-10">
        {status.error && <ErrorNote error={status.error} />}
        {s && (
          <>
            <div className="mb-6 sm:mb-8">
              <div className="flex items-center justify-between gap-4">
                <div className="min-w-0 flex-1">
                  {s.logos.light || s.logos.dark ? (
                    <>
                      <h1 className="sr-only">{s.title}</h1>
                      <StatusLogo logos={s.logos} alt={`${s.title} logo`} />
                    </>
                  ) : (
                    <h1 className="text-xl leading-tight font-semibold break-words sm:text-2xl lg:text-3xl">
                      {s.title}
                    </h1>
                  )}
                </div>
                {s.website_url && (
                  <a
                    href={s.website_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex min-h-11 shrink-0 items-center gap-2 rounded-md px-3 text-sm font-semibold transition-colors hover:bg-muted sm:min-h-9 sm:text-base"
                  >
                    <span className="hidden sm:inline">Visit website</span>
                    <ExternalLink className="size-4" />
                    <span className="sr-only sm:hidden">Visit website</span>
                  </a>
                )}
              </div>
              {s.description && (
                <p className="mt-2 text-sm whitespace-pre-line text-muted-foreground sm:text-base">{s.description}</p>
              )}
            </div>
            <Banner overall={s.overall} />
            {notices.map((n) => (
              <div key={n.created_at} className="mt-4">
                <NoticeCard notice={n} />
              </div>
            ))}
            {active.length > 0 && (
              <section className="mt-8">
                <h2 className="mb-3 text-sm font-semibold tracking-wide text-muted-foreground uppercase">
                  Active incidents
                </h2>
                <div className="flex flex-col gap-4">
                  {active.map((i) => (
                    <IncidentCard key={i.created_at} incident={i} />
                  ))}
                </div>
              </section>
            )}
            {s.maintenance.map((n) => (
              <MaintenanceNotice key={`${n.title}-${n.starts_at}`} n={n} />
            ))}

            {s.sections.length === 0 ? (
              <Card className="mt-8">
                <CardContent className="py-8 text-center">
                  <div className="mx-auto mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
                    <AlertCircle className="size-6 text-muted-foreground" />
                  </div>
                  <p className="text-sm text-muted-foreground">No monitors to show yet.</p>
                </CardContent>
              </Card>
            ) : (
              s.sections.map((section) => (
                <Card key={section.name} className="mt-8">
                  <SectionHeader title={section.name} />
                  <CardContent className="flex flex-col gap-6 p-3 sm:p-6">
                    {section.monitors.map((m) => (
                      <MonitorCard key={m.name} m={m} />
                    ))}
                  </CardContent>
                </Card>
              ))
            )}

            {past.length > 0 && (
              <Card className="mt-8">
                <SectionHeader title="Past incidents" />
                <CardContent className="flex flex-col gap-4 p-3 sm:p-6">
                  {past.map((i) => (
                    <IncidentCard key={i.created_at} incident={i} />
                  ))}
                </CardContent>
              </Card>
            )}

            {s.events.length > 0 && (
              <Card className="mt-8">
                <SectionHeader title="Recent events" />
                <CardContent className="p-3 sm:p-6">
                  <ol className="divide-y">
                    {s.events.map((e, i) => (
                      <li key={i} className="flex items-baseline justify-between gap-4 py-3 text-sm">
                        <span>
                          <span className="font-medium">{e.monitor}</span>{" "}
                          <span className={e.status === "down" ? "text-red-600 dark:text-red-400" : brandText}>
                            {e.status === "down" ? "went down" : "recovered"}
                          </span>
                        </span>
                        <time className="shrink-0 text-xs text-muted-foreground" dateTime={e.time}>
                          {formatDateTime(e.time)}
                        </time>
                      </li>
                    ))}
                  </ol>
                </CardContent>
              </Card>
            )}

            <footer className="mt-10 flex flex-col items-center gap-3 text-xs text-muted-foreground">
              <div className="flex items-center gap-3">
                <PoweredBy medium="status_page" />
                <span className="text-border">·</span>
                <ThemeToggle />
              </div>
              <span>Last updated {timeAgo(s.updated)}</span>
            </footer>
          </>
        )}
      </div>
    </div>
  );
}

function Banner({ overall }: { overall: PublicStatus["overall"] }) {
  const b = banners[overall];
  const Icon = b.icon;
  return (
    <Card className="overflow-hidden shadow-sm">
      <div className={cn("flex justify-center p-4 sm:p-6 lg:p-8", b.tint)}>
        <div className="flex flex-col items-center gap-3 text-center sm:flex-row sm:gap-4 sm:text-left">
          <div className={cn("shrink-0 rounded-full p-2 sm:p-2.5", b.iconBg)}>
            <Icon className={cn("size-5 sm:size-6", b.iconColor)} />
          </div>
          <div>
            <h2 className={cn("mb-0.5 text-lg font-semibold sm:text-xl lg:text-2xl", b.titleColor)}>{b.title}</h2>
            <p className="text-sm text-muted-foreground">{b.subtitle}</p>
          </div>
        </div>
      </div>
    </Card>
  );
}

const maintenanceAccent = { border: "border-l-sky-500", dot: "bg-sky-500", live: false };

/** Planned work, in progress or coming up. */
function MaintenanceNotice({ n }: { n: PublicMaintenance }) {
  const range = `${formatWhen(n.starts_at)} – ${formatWhen(n.ends_at)}`;
  return (
    <Card className="mt-4 border-sky-500/40 bg-sky-500/5 p-4 sm:p-5">
      <div className="flex gap-3">
        <Wrench className="mt-0.5 size-5 shrink-0 text-sky-500" />
        <div className="min-w-0">
          <div className="text-xs font-medium tracking-wide text-sky-600 uppercase dark:text-sky-400">
            {n.active ? "Maintenance in progress" : "Scheduled maintenance"}
          </div>
          <h3 className="mt-1 font-semibold">{n.title}</h3>
          <p className="mt-0.5 text-sm text-muted-foreground">{range}</p>
          {n.description && <p className="mt-2 text-sm whitespace-pre-line">{n.description}</p>}
          <p className="mt-2 text-xs text-muted-foreground">
            Affects: {n.monitors.length ? n.monitors.join(", ") : "all services"}
          </p>
        </div>
      </div>
    </Card>
  );
}

/** "Thu, Oct 2, 03:00" in the viewer's time zone. */
function formatWhen(iso: string) {
  return new Date(iso).toLocaleString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function MonitorCard({ m }: { m: PublicMonitor }) {
  const accent = m.in_maintenance ? maintenanceAccent : accents[m.status];
  const pct = m.ratio == null ? null : m.ratio * 100;
  // A healthcheck's uptime, or the share of a heartbeat's runs that were on time.
  const heartbeat = m.kind === "heartbeat";
  const measure = heartbeat ? "on time" : "uptime";
  const when = heartbeat
    ? { label: "Last run", time: m.last_run }
    : { label: "Last change", time: m.last_change ?? undefined };

  return (
    <div
      className={cn(
        "rounded-xl border border-l-[3px] bg-card p-4 transition-shadow hover:shadow-sm sm:p-5",
        accent.border,
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-3 sm:flex-nowrap">
        <div className="flex min-w-0 flex-1 items-center gap-3 sm:gap-4">
          <span className="relative mt-0.5 flex size-2.5 shrink-0">
            {accent.live && (
              <span
                className={cn(
                  "absolute inline-flex size-full animate-ping rounded-full opacity-60 motion-reduce:hidden",
                  accent.dot,
                )}
              />
            )}
            <span className={cn("relative inline-flex size-2.5 rounded-full", accent.dot)} />
          </span>
          <div className="min-w-0 flex-1">
            <h3 className="mb-1 text-sm font-medium sm:text-base">{m.name}</h3>
            <span className="inline-flex items-center rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium whitespace-nowrap text-muted-foreground">
              {m.type_label}
            </span>
            {m.in_maintenance && (
              <span className="ml-1.5 inline-flex items-center rounded-full bg-sky-500/15 px-2 py-0.5 text-[11px] font-medium whitespace-nowrap text-sky-600 dark:text-sky-400">
                Maintenance
              </span>
            )}
          </div>
        </div>
        <div className="shrink-0 text-right">
          <div
            className={cn(
              "mb-1 text-lg font-semibold tabular-nums sm:text-xl",
              pct == null
                ? "text-muted-foreground"
                : pct >= 99
                  ? brandText
                  : pct >= 95
                    ? "text-orange-600 dark:text-orange-400"
                    : "text-red-600 dark:text-red-400",
            )}
            title={
              heartbeat
                ? `Runs on time over the last ${m.days.length} days.`
                : `Total uptime for the last ${m.days.length} days.`
            }
          >
            {pct == null ? "--" : pct.toFixed(2)}%
          </div>
          <div className="text-xs text-muted-foreground">
            {when.label}: {/* Own line on phones, so the timestamp doesn't squeeze the name. */}
            <span className="block font-medium sm:inline">
              {when.time ? new Date(when.time).toLocaleString() : "Never"}
            </span>
          </div>
        </div>
      </div>

      <div className="mt-4 sm:mt-6">
        <div className="mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <h4 className="text-xs font-medium text-muted-foreground">
            {m.days.length}-day {heartbeat ? "on-time runs" : "uptime"}
          </h4>
          <div className="flex items-center gap-3 text-[11px] text-muted-foreground">
            <Legend className="bg-up" label="99%+" />
            <Legend className="bg-degraded" label="95–99%" />
            <Legend className="bg-down" label="<95%" />
            <Legend className="bg-no-data" label="No data" />
          </div>
        </div>
        <div className="rounded-lg border bg-muted/30 p-2 sm:p-3">
          <DailyBars days={m.days} measure={measure} />
        </div>
      </div>
    </div>
  );
}

function Legend({ className, label }: { className: string; label: string }) {
  return (
    <span className="flex items-center gap-1">
      <span className={cn("size-2 rounded-sm", className)} />
      {label}
    </span>
  );
}

/** Flips light/dark, like the toggle on hosted Uptimy status pages. */
function ThemeToggle() {
  const { resolved, setTheme } = useTheme();
  const next = resolved === "dark" ? "light" : "dark";
  return (
    <button
      type="button"
      onClick={() => setTheme(next)}
      className="grid size-8 place-items-center rounded-md border transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      {resolved === "dark" ? <Moon className="size-4" /> : <Sun className="size-4" />}
      <span className="sr-only">Switch to {next} theme</span>
    </button>
  );
}

function SectionHeader({ title }: { title: string }) {
  return (
    <CardHeader className="border-b bg-muted/50 pb-5">
      <CardTitle className="flex items-center text-base text-(--status-accent,var(--foreground))">
        <span className="mr-3 h-6 w-1 shrink-0 rounded-full bg-(--status-accent,var(--primary))" />
        {title}
      </CardTitle>
    </CardHeader>
  );
}

/** The logo, with the dark variant in dark mode when there is one. */
function StatusLogo({ logos, alt }: { logos: PublicStatus["logos"]; alt: string }) {
  const light = logos.light ?? logos.dark!;
  const dark = logos.dark ?? logos.light!;
  const size = "h-10 w-auto max-w-[200px] object-contain object-left sm:h-12 sm:max-w-[260px] lg:h-14 lg:max-w-[320px]";
  return (
    <>
      <img src={light} alt={alt} className={cn(size, "dark:hidden")} />
      <img src={dark} alt={alt} className={cn(size, "hidden dark:block")} />
    </>
  );
}
