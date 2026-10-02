import { useEffect, useState } from "react";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "./button";
import { toast } from "./toast";

/** An icon button that copies text and shows a check mark for a moment. */
export function CopyButton({ text, label = "Copy", className }: { text: string; label?: string; className?: string }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const t = window.setTimeout(() => setCopied(false), 1500);
    return () => window.clearTimeout(t);
  }, [copied]);

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={copied ? "Copied" : label}
      title={label}
      className={cn("size-8 shrink-0", className)}
      onClick={() =>
        navigator.clipboard.writeText(text).then(
          () => setCopied(true),
          () =>
            toast.error("Couldn't copy", "Your browser blocked the clipboard. Select the text and copy it instead."),
        )
      }
    >
      {copied ? <Check className="text-up" /> : <Copy />}
    </Button>
  );
}

/** Code (a URL, a command) with a copy button. */
export function CopyField({ text, children, label }: { text: string; children?: React.ReactNode; label?: string }) {
  return (
    <div className="flex items-start gap-1 rounded-md border bg-muted/50 py-1 pr-1 pl-3">
      <code className="min-w-0 flex-1 py-1 text-sm whitespace-pre-wrap [overflow-wrap:anywhere]">
        {children ?? text}
      </code>
      <CopyButton text={text} label={label} />
    </div>
  );
}
