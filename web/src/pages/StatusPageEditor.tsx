import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, FileText, Globe, RotateCcw, RotateCw } from "lucide-react";
import { api, type StatusPageConfig, type StatusPageLogos } from "@/lib/api";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, Input, Switch, Textarea } from "@/components/ui/input";
import { toast } from "@/components/ui/toast";
import { ErrorNote, PageHeader } from "@/components/Layout";
import { useCanEdit } from "@/components/AuthGate";
import { LogoField } from "@/components/status-page/LogoField";
import { DEFAULT_ACCENT, SectionsEditor } from "@/components/status-page/SectionsEditor";

type Draft = StatusPageConfig;

export function StatusPageEditor() {
  const qc = useQueryClient();
  const canEdit = useCanEdit();
  const config = useQuery({ queryKey: ["status-page"], queryFn: api.statusPageConfig });
  // Only edits are kept in state; without any, the page shows the server copy,
  // so it follows changes made elsewhere without ever discarding unsaved work.
  const [edits, setEdits] = useState<Draft | null>(null);
  const draft = edits ?? config.data ?? null;
  const [previewKey, setPreviewKey] = useState(0);
  const dirty = useMemo(
    () => edits !== null && JSON.stringify(edits) !== JSON.stringify(config.data),
    [edits, config.data],
  );
  const edit = (update: (d: Draft) => Draft) => setEdits((d) => update(d ?? config.data!));

  const save = useMutation({
    mutationFn: () => {
      const d = draft!;
      return api.saveStatusPageConfig({
        enabled: d.enabled,
        title: d.title,
        description: d.description,
        show_events: d.show_events,
        accent_color: d.accent_color,
        website_url: d.website_url,
        domain: d.domain,
        sections: d.sections,
        monitors: d.monitors.map(({ id, public: pub, label, section }) => ({ id, public: pub, label, section })),
      });
    },
    onSuccess: (saved) => {
      qc.setQueryData(["status-page"], saved);
      setEdits(null);
      qc.invalidateQueries({ queryKey: ["info"] });
      setPreviewKey((k) => k + 1);
      toast.success("Status page saved");
    },
  });

  // Logos save right away, without touching other unsaved edits.
  const applyLogos = (logos: StatusPageLogos) => {
    qc.setQueryData<StatusPageConfig>(["status-page"], (c) => c && { ...c, logos });
    setEdits((d) => d && { ...d, logos });
    setPreviewKey((k) => k + 1);
  };

  const set = <K extends keyof Draft>(key: K, value: Draft[K]) => edit((d) => ({ ...d, [key]: value }));
  const domain = config.data?.domain;
  const publicURL = domain ? `https://${domain}` : "/status";
  const domainClash = !!draft && isDashboardHost(draft.domain);

  return (
    <>
      <PageHeader
        title="Status page"
        icon={FileText}
        description={`What visitors see at ${domain ?? "/status"}. Only names and uptime are shown, never hostnames or URLs.`}
        actions={
          <a href={publicURL} target="_blank" rel="noreferrer" className={buttonVariants({ variant: "outline" })}>
            Open <ExternalLink />
          </a>
        }
      />
      <ErrorNote error={config.error} />
      {draft && (
        <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
            <fieldset disabled={!canEdit} className="flex flex-col gap-6">
              {!canEdit && (
                <p className="rounded-md border bg-muted/50 px-3 py-2 text-sm text-muted-foreground">
                  You have read-only access. Ask an admin to change the status page.
                </p>
              )}
              <Card>
                <CardHeader>
                  <CardTitle>Page</CardTitle>
                </CardHeader>
                <CardContent className="flex flex-col gap-5">
                  <Switch
                    id="sp-enabled"
                    checked={draft.enabled}
                    onChange={(v) => set("enabled", v)}
                    label="Publish the status page"
                  />
                  <Field
                    label="Title"
                    htmlFor="sp-title"
                    hint="Shown when there's no logo, and as the browser tab title."
                  >
                    <Input
                      id="sp-title"
                      required
                      maxLength={100}
                      value={draft.title}
                      onChange={(e) => set("title", e.target.value)}
                    />
                  </Field>
                  <Field
                    label="Description"
                    htmlFor="sp-desc"
                    hint="Optional. Shown under the title, e.g. who to contact during an outage."
                  >
                    <Textarea
                      id="sp-desc"
                      maxLength={500}
                      rows={3}
                      value={draft.description}
                      onChange={(e) => set("description", e.target.value)}
                    />
                  </Field>
                  <Switch
                    id="sp-events"
                    checked={draft.show_events}
                    onChange={(v) => set("show_events", v)}
                    label="Show recent events (outages and recoveries)"
                  />
                </CardContent>
              </Card>

              <DomainCard value={draft.domain} onChange={(v) => set("domain", v)} />

              <Card>
                <CardHeader>
                  <CardTitle>Branding</CardTitle>
                  <CardDescription>Make the page look like yours.</CardDescription>
                </CardHeader>
                <CardContent className="flex flex-col gap-5">
                  <div className="grid gap-4 sm:grid-cols-2">
                    <LogoField
                      variant="light"
                      label="Logo"
                      hint="Replaces the title. PNG, SVG, JPEG or WebP, up to 512 KB."
                      url={draft.logos.light}
                      onChange={applyLogos}
                      disabled={!canEdit}
                    />
                    <LogoField
                      variant="dark"
                      label="Logo for dark mode"
                      hint="Optional. Otherwise the logo above is used."
                      url={draft.logos.dark}
                      onChange={applyLogos}
                      disabled={!canEdit}
                    />
                  </div>
                  <Field label="Accent color" htmlFor="sp-accent" hint="Used for section headings.">
                    <div className="flex items-center gap-2">
                      <input
                        type="color"
                        aria-label="Pick accent color"
                        value={draft.accent_color || DEFAULT_ACCENT}
                        onChange={(e) => set("accent_color", e.target.value)}
                        className="h-9 w-12 shrink-0 cursor-pointer rounded-md border bg-card p-1"
                      />
                      <Input
                        id="sp-accent"
                        className="w-32 font-mono"
                        placeholder={DEFAULT_ACCENT}
                        maxLength={7}
                        value={draft.accent_color}
                        onChange={(e) => set("accent_color", e.target.value)}
                      />
                      {draft.accent_color && (
                        <Button variant="ghost" size="sm" onClick={() => set("accent_color", "")}>
                          <RotateCcw /> Default
                        </Button>
                      )}
                    </div>
                  </Field>
                  <Field label="Website" htmlFor="sp-website" hint='Adds a "Visit website" link to the header.'>
                    <Input
                      id="sp-website"
                      type="url"
                      placeholder="https://example.com"
                      maxLength={300}
                      value={draft.website_url}
                      onChange={(e) => set("website_url", e.target.value)}
                    />
                  </Field>
                </CardContent>
              </Card>

              <SectionsEditor draft={draft} onChange={edit} />

              {canEdit && (
                <div className="sticky bottom-4 z-10 flex items-center justify-end gap-3 rounded-lg border bg-card/95 px-4 py-3 shadow-sm backdrop-blur">
                  <ErrorNote error={save.error} />
                  <span className="mr-auto text-sm text-muted-foreground">
                    {dirty ? "Unsaved changes" : "Up to date"}
                  </span>
                  <Button variant="ghost" disabled={!dirty || save.isPending} onClick={() => setEdits(null)}>
                    Discard
                  </Button>
                  <Button type="submit" disabled={!dirty || save.isPending || domainClash}>
                    {save.isPending ? "Saving…" : "Save changes"}
                  </Button>
                </div>
              )}
            </fieldset>
          </form>

          <Card className="overflow-hidden xl:sticky xl:top-20">
            <div className="flex items-center justify-between border-b px-4 py-2.5">
              <span className="text-sm font-medium">Preview</span>
              <span className="flex items-center gap-2 text-xs text-muted-foreground">
                {dirty ? "Updates when you save" : "Live"}
                <button
                  type="button"
                  aria-label="Reload preview"
                  className="rounded p-1 hover:bg-muted hover:text-foreground"
                  onClick={() => setPreviewKey((k) => k + 1)}
                >
                  <RotateCw className="size-3.5" />
                </button>
              </span>
            </div>
            {config.data?.enabled ? (
              <iframe
                key={previewKey}
                src="/status?preview"
                title="Status page preview"
                className="h-[70vh] w-full bg-background"
              />
            ) : (
              <p className="px-4 py-16 text-center text-sm text-muted-foreground">
                The status page is turned off. Visitors to /status get a “not found” page.
              </p>
            )}
          </Card>
        </div>
      )}
    </>
  );
}

// hostOf turns a pasted address into its hostname, as the server does.
function hostOf(v: string) {
  return v
    .trim()
    .toLowerCase()
    .replace(/^[a-z]+:\/\//, "")
    .replace(/[/?#].*$/, "")
    .replace(/:\d+$/, "")
    .replace(/\.$/, "");
}

// isDashboardHost reports whether the domain is the address in use, which
// would then show only the status page.
function isDashboardHost(domain: string) {
  const host = hostOf(domain);
  return host !== "" && host === window.location.hostname;
}

function DomainCard({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const isDashboard = isDashboardHost(value);
  return (
    <Card>
      <CardHeader>
        <CardTitle>Custom domain</CardTitle>
        <CardDescription>
          Optional. A separate address, like status.example.com, that shows only the status page, at its root. Sign-in,
          the dashboard and the API aren't served there, so they can stay private.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <Field
          label="Domain"
          htmlFor="sp-domain"
          hint="Point its DNS at the agent and add TLS where you expose it: an Ingress, Railway's custom domains, or a proxy like Caddy. Heartbeat ping URLs work on it too."
        >
          <div className="relative">
            <Globe className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              id="sp-domain"
              className="pl-9 font-mono"
              placeholder="status.example.com"
              maxLength={253}
              autoComplete="off"
              spellCheck={false}
              value={value}
              onChange={(e) => onChange(e.target.value)}
              aria-invalid={isDashboard || undefined}
            />
          </div>
        </Field>
        {isDashboard && (
          <p className="rounded-md border border-down/30 bg-down/10 px-3 py-2 text-sm text-down">
            That's the address you're using right now. Use a separate one for the status page, or this address would
            show only the status page and you'd lose the dashboard here.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
