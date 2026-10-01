import { Settings as SettingsIcon } from "lucide-react";
import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ErrorNote, PageHeader } from "@/components/Layout";
import { WatchTheWatcherCard } from "@/components/WatchTheWatcher";
import { KumaImportCard } from "@/components/KumaImport";
import { useCanEdit } from "@/components/AuthGate";

export function Settings() {
  const info = useQuery({ queryKey: ["info"], queryFn: api.info });
  const i = info.data;
  const canEdit = useCanEdit();

  return (
    <>
      <PageHeader title="Settings" icon={SettingsIcon} description="Settings for this agent." />
      <ErrorNote error={info.error} />
      {i && (
        <div className="flex flex-col gap-6">
          <WatchTheWatcherCard />
          {canEdit && <KumaImportCard />}

          <Card>
            <CardHeader>
              <CardTitle>Agent</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="grid gap-x-8 gap-y-3 text-sm sm:grid-cols-[max-content_1fr]">
                <Row label="Version" value={i.version} />
                <Row
                  label="Kubernetes checks"
                  value={i.kubernetes ? "Available (running in cluster)" : "Unavailable (not in a cluster)"}
                />
                <Row label="Monitors file" value={i.monitors_file ? "Loaded" : "Not configured"} />
                <Row label="Data retention" value={`${i.retention_days} days`} />
                <Row
                  label="Status page"
                  value={
                    i.status_page_enabled ? (
                      <Link to="/status-page" className="underline">
                        {i.status_page_title}
                      </Link>
                    ) : (
                      "Disabled"
                    )
                  }
                />
              </dl>
            </CardContent>
          </Card>
        </div>
      )}
    </>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{value}</dd>
    </>
  );
}
