import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle, RotateCw } from "lucide-react";
import { Button, buttonVariants } from "@/components/ui/button";

const ISSUE_URL = "https://github.com/uptimy/agent/issues/new?template=bug_report.yml";

/**
 * Catches a crash while rendering so one broken page doesn't blank the whole
 * app. Wrap each page (keyed by path, so navigating away resets it).
 */
export class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Uncaught error while rendering", error, info.componentStack);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <div role="alert" className="mx-auto flex max-w-lg flex-col items-center gap-3 py-16 text-center">
        <span className="grid size-12 place-items-center rounded-full bg-down/15 text-down">
          <AlertTriangle className="size-6" />
        </span>
        <h2 className="text-lg font-semibold">This page ran into a problem</h2>
        <p className="text-sm text-muted-foreground">
          Your monitors keep running; only this page failed to display. Reloading usually helps.
        </p>
        <code className="max-w-full truncate rounded bg-muted px-2 py-1 text-xs text-muted-foreground">
          {error.message}
        </code>
        <div className="mt-2 flex gap-2">
          <Button onClick={() => window.location.reload()}>
            <RotateCw /> Reload
          </Button>
          <a href={ISSUE_URL} target="_blank" rel="noopener" className={buttonVariants({ variant: "outline" })}>
            Report a bug
          </a>
        </div>
      </div>
    );
  }
}
