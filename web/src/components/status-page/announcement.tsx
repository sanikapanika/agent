import { Megaphone } from "lucide-react";
import type { Announcement } from "@/lib/api";
import { formatDateTime } from "@/lib/utils";

/**
 * The status page's announcement, under the overall status. Hosted Uptimy
 * status pages show theirs the same way.
 */
export function AnnouncementCard({ announcement: a }: { announcement: Announcement }) {
  const edited = a.updated_at !== a.posted_at;
  return (
    <div className="rounded-xl border border-primary/30 bg-primary/5 p-4 sm:p-5">
      <div className="flex gap-3">
        <Megaphone className="mt-0.5 size-5 shrink-0 text-primary" />
        <div className="min-w-0">
          <h3 className="leading-snug font-semibold">{a.title}</h3>
          {a.message && <p className="mt-1 text-sm break-words whitespace-pre-line">{a.message}</p>}
          <p className="mt-2 text-xs text-muted-foreground">
            Posted {formatDateTime(a.posted_at)}
            {edited && ` · Updated ${formatDateTime(a.updated_at)}`}
          </p>
        </div>
      </div>
    </div>
  );
}
