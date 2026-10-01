import { useEffect, useRef, useSyncExternalStore } from "react";
import { CheckCircle2, Info, X, XCircle } from "lucide-react";
import { cn } from "@/lib/utils";

// Toasts, like the Uptimy app's toaster: top-right (placement "top-end"),
// auto-dismiss, paused while hovered. Call toast.success(...) from anywhere
// and render <Toaster /> once.

type Kind = "success" | "error" | "info";
interface Toast {
  id: number;
  kind: Kind;
  title: string;
  description?: string;
}

const DURATION = 4000;
let toasts: Toast[] = [];
let nextId = 1;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((l) => l());

function push(kind: Kind, title: string, description?: string) {
  toasts = [...toasts, { id: nextId++, kind, title, description }].slice(-4);
  emit();
}

function dismiss(id: number) {
  toasts = toasts.filter((t) => t.id !== id);
  emit();
}

export const toast = {
  success: (title: string, description?: string) => push("success", title, description),
  error: (title: string, description?: string) => push("error", title, description),
  info: (title: string, description?: string) => push("info", title, description),
};

const icons = {
  success: { icon: CheckCircle2, className: "text-up" },
  error: { icon: XCircle, className: "text-down" },
  info: { icon: Info, className: "text-sky-500" },
};

export function Toaster() {
  const list = useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => toasts,
  );
  return (
    <div
      aria-live="polite"
      className="pointer-events-none fixed top-4 right-4 left-4 z-[10000] flex flex-col items-end gap-2 md:left-auto"
    >
      {list.map((t) => (
        <ToastItem key={t.id} t={t} />
      ))}
    </div>
  );
}

function ToastItem({ t }: { t: Toast }) {
  const timer = useRef<number | undefined>(undefined);
  const start = () => {
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => dismiss(t.id), DURATION);
  };
  const pause = () => window.clearTimeout(timer.current);

  useEffect(() => {
    start();
    return pause;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const { icon: Icon, className } = icons[t.kind];
  return (
    <div
      role={t.kind === "error" ? "alert" : "status"}
      onPointerEnter={pause}
      onPointerLeave={start}
      className="animate-toast-in pointer-events-auto flex w-full items-start gap-3 rounded-xl border bg-card p-4 shadow-lg md:w-sm"
    >
      <Icon className={cn("mt-0.5 size-5 shrink-0", className)} />
      <div className="min-w-0 flex-1">
        <div className="text-sm font-semibold">{t.title}</div>
        {t.description && <div className="mt-0.5 text-sm text-muted-foreground">{t.description}</div>}
      </div>
      <button
        type="button"
        aria-label="Dismiss"
        onClick={() => dismiss(t.id)}
        className="-m-1 grid size-6 shrink-0 place-items-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
      >
        <X className="size-4" />
      </button>
    </div>
  );
}
