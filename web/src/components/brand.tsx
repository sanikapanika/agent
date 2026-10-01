import { cn } from "@/lib/utils";

// Official Uptimy artwork, copied from upti.my-landing/public/brand. Served
// from the agent itself so a self-hosted install never calls home to load it.
// The dark variants swap with the theme (the `dark` class on <html>), like the rest of the UI.

export function UptimyLogo({ className }: { className?: string }) {
  return (
    <>
      <img src="/brand/uptimy-logo.svg" alt="Uptimy" className={cn("h-6 w-auto dark:hidden", className)} />
      <img src="/brand/uptimy-logo-white.svg" alt="Uptimy" className={cn("hidden h-6 w-auto dark:block", className)} />
    </>
  );
}

export function UptimyMark({ className }: { className?: string }) {
  return (
    <>
      <img src="/brand/uptimy-mark.svg" alt="" className={cn("h-3.5 w-auto dark:hidden", className)} />
      <img src="/brand/uptimy-mark-white.svg" alt="" className={cn("hidden h-3.5 w-auto dark:block", className)} />
    </>
  );
}

/** "Powered by Uptimy" credit, matching the hosted status pages. */
export function PoweredBy({ medium }: { medium: string }) {
  return (
    <a
      href={`https://www.upti.my/?utm_source=uptimy-agent&utm_medium=${medium}&utm_campaign=powered_by`}
      target="_blank"
      rel="noopener"
      className="inline-flex items-center gap-1.5 text-muted-foreground transition-colors hover:text-foreground"
    >
      <span>Powered by</span>
      <UptimyMark />
      <span className="font-semibold">Uptimy</span>
    </a>
  );
}
