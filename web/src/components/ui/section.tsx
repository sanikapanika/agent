import type * as React from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

/** A titled card, like SectionCard in the Uptimy app. */
export function Section({
  title,
  icon: Icon,
  description,
  action,
  children,
  className,
  contentClassName,
}: {
  title: React.ReactNode;
  icon?: LucideIcon;
  description?: React.ReactNode;
  action?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
  contentClassName?: string;
}) {
  return (
    <section className={cn("rounded-xl border bg-card shadow-xs", className)}>
      <div className="flex flex-col gap-3 px-4 pt-4 sm:flex-row sm:items-center sm:justify-between md:px-5 md:pt-5">
        <div className="min-w-0">
          <h2 className="flex items-center gap-2 text-base font-bold md:text-lg">
            {Icon && <Icon className="size-4 shrink-0" />}
            {title}
          </h2>
          {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
        </div>
        {action && <div className="flex shrink-0 items-center gap-2">{action}</div>}
      </div>
      <div className={cn("p-4 md:p-5", contentClassName)}>{children}</div>
    </section>
  );
}
