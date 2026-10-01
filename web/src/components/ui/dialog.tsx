import { useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";
import { X, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * A modal dialog: backdrop, title, a close button, Escape to close, focus
 * kept inside while open and returned to the opener after. The caller lays
 * out the body and the buttons.
 */
export function Dialog({
  title,
  subtitle,
  icon: Icon,
  tone = "default",
  role = "dialog",
  onClose,
  busy = false,
  initialFocus,
  describedBy,
  className,
  children,
}: {
  title: React.ReactNode;
  subtitle?: React.ReactNode;
  icon?: LucideIcon;
  tone?: "default" | "danger";
  role?: "dialog" | "alertdialog";
  onClose: () => void;
  /** While busy, the dialog can't be closed. */
  busy?: boolean;
  /** What to focus on open; the first focusable element otherwise. */
  initialFocus?: React.RefObject<HTMLElement | null>;
  describedBy?: string;
  className?: string;
  children: React.ReactNode;
}) {
  const panel = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const close = () => {
    if (!busy) onClose();
  };

  useEffect(() => {
    const opener = document.activeElement as HTMLElement | null;
    (initialFocus?.current ?? focusables(panel.current)[1] ?? panel.current)?.focus();
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = overflow;
      opener?.focus?.();
    };
  }, [initialFocus]);

  // Escape closes; Tab stays inside the dialog.
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.stopPropagation();
      close();
    } else if (e.key === "Tab") {
      const els = focusables(panel.current);
      const first = els[0];
      const last = els[els.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last?.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first?.focus();
      }
    }
  };

  return createPortal(
    <div className="fixed inset-0 z-[10000] flex items-center justify-center p-4" onKeyDown={onKeyDown}>
      <div className="animate-fade-in absolute inset-0 bg-black/60" onMouseDown={close} />
      <div
        ref={panel}
        role={role}
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={describedBy}
        tabIndex={-1}
        className={cn(
          "animate-dialog-in relative flex max-h-[calc(100vh-2rem)] w-full max-w-md flex-col rounded-xl border bg-card shadow-xl outline-none",
          className,
        )}
      >
        <button
          type="button"
          aria-label="Close"
          onClick={close}
          disabled={busy}
          className="absolute top-4 right-4 grid size-8 place-items-center rounded-md text-muted-foreground hover:bg-nav-hover hover:text-foreground disabled:opacity-50"
        >
          <X className="size-4" />
        </button>
        <div className="flex items-center gap-3 p-6 pr-14">
          {Icon && (
            <span
              className={cn(
                "grid size-9 shrink-0 place-items-center rounded-lg",
                tone === "danger" ? "bg-down/15 text-down" : "bg-brand/15 text-brand",
              )}
            >
              <Icon className="size-5" />
            </span>
          )}
          <div className="min-w-0">
            <h2 id={titleId} className="text-lg font-semibold">
              {title}
            </h2>
            {subtitle && <p className="text-sm text-muted-foreground">{subtitle}</p>}
          </div>
        </div>
        {children}
      </div>
    </div>,
    document.body,
  );
}

function focusables(root: HTMLElement | null) {
  if (!root) return [];
  return Array.from(
    root.querySelectorAll<HTMLElement>(
      'button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
    ),
  );
}
