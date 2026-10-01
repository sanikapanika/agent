import { useId, useRef, useState, useSyncExternalStore } from "react";
import { AlertTriangle, Check, Loader2, Trash2, type LucideIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

// A confirmation dialog in place of window.confirm, matching the Uptimy app's
// DeleteItem / DeleteItemConfirmation dialogs. The dialog runs the action
// itself: it stays open with a spinner while the action runs, closes on
// success, and shows the error inside the dialog on failure, so a failed
// delete is never silently lost.
//
//   confirm({ title: "Delete channel?", message: ..., confirmLabel: "Delete",
//             action: () => remove.mutateAsync() })
//
// Render <ConfirmDialog /> once, near the root.

export interface ConfirmOptions {
  title: string;
  /** Under the title; defaults to "This action cannot be undone" for danger. */
  subtitle?: string;
  message: React.ReactNode;
  confirmLabel: string;
  /** "danger" (red, the default) or "default" (brand green). */
  tone?: "danger" | "default";
  icon?: LucideIcon;
  /** Make people type this (e.g. the monitor's name) before confirming. */
  typeToConfirm?: string;
  action: () => Promise<unknown>;
}

let current: (ConfirmOptions & { id: number }) | null = null;
let nextId = 1;
const listeners = new Set<() => void>();
const set = (next: ConfirmOptions | null) => {
  current = next && { ...next, id: nextId++ };
  listeners.forEach((l) => l());
};

export function confirm(options: ConfirmOptions) {
  set(options);
}

export function ConfirmDialog() {
  const options = useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => current,
  );
  if (!options) return null;
  // Keyed so every confirmation starts with fresh state.
  return <ConfirmPanel key={options.id} options={options} />;
}

function ConfirmPanel({ options }: { options: ConfirmOptions }) {
  const { title, message, confirmLabel, typeToConfirm, action } = options;
  const tone = options.tone ?? "danger";
  const Icon = options.icon ?? (tone === "danger" ? AlertTriangle : undefined);
  const subtitle = options.subtitle ?? (tone === "danger" ? "This action cannot be undone" : undefined);

  const [typed, setTyped] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const descId = useId();
  // Stray spaces from a paste don't count against the match.
  const matches = !typeToConfirm || typed.trim() === typeToConfirm.trim();

  const submit = async () => {
    if (!matches || pending) return;
    setPending(true);
    setError(null);
    try {
      await action();
      set(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setPending(false);
    }
  };

  // Focus the input when typing is required, otherwise Cancel (the safe choice).
  return (
    <Dialog
      role="alertdialog"
      title={title}
      subtitle={subtitle}
      icon={Icon}
      tone={tone}
      onClose={() => set(null)}
      busy={pending}
      initialFocus={typeToConfirm ? inputRef : cancelRef}
      describedBy={descId}
    >
      <div className="flex flex-col gap-4 border-y p-6">
        <div id={descId} className="rounded-lg border bg-nav-hover p-4 text-sm leading-relaxed text-muted-foreground">
          {message}
        </div>
        {typeToConfirm && (
          <label className="flex flex-col gap-2">
            <span className="text-sm font-semibold">Type the name to confirm</span>
            {/* On its own line so long names wrap cleanly; one click selects it. */}
            <code className="rounded-md bg-muted px-2 py-1.5 font-mono text-sm break-all select-all">
              {typeToConfirm}
            </code>
            <span className="relative">
              <Input
                ref={inputRef}
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && submit()}
                aria-label={`Type ${typeToConfirm} to confirm`}
                autoComplete="off"
                spellCheck={false}
                className={cn("pr-9 font-mono", typed && matches && "border-up focus-visible:ring-up/40")}
              />
              {typed && matches && (
                <Check
                  aria-hidden
                  className="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-up"
                />
              )}
            </span>
          </label>
        )}
        {error && (
          <p role="alert" className="rounded-md border border-down/30 bg-down/10 px-3 py-2 text-sm text-down">
            {error}
          </p>
        )}
      </div>

      <div className="flex gap-3 p-6 pt-4">
        <Button ref={cancelRef} variant="outline" className="flex-1" onClick={() => set(null)} disabled={pending}>
          Cancel
        </Button>
        <Button
          variant={tone === "danger" ? "destructive" : "default"}
          className="flex-1"
          onClick={submit}
          disabled={!matches || pending}
        >
          {pending ? (
            <Loader2 className="animate-spin" />
          ) : options.icon ? (
            <options.icon />
          ) : tone === "danger" ? (
            <Trash2 />
          ) : null}
          {confirmLabel}
        </Button>
      </div>
    </Dialog>
  );
}
